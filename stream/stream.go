package stream

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/goccy/go-json"

	"github.com/benitogf/ooo/meta"
	"github.com/benitogf/ooo/monotonic"

	"github.com/benitogf/jsonpatch"

	"github.com/benitogf/coat"
	"github.com/gorilla/websocket"
)

var (
	ErrPoolNotFound   = errors.New("stream: pool not found")
	ErrPoolCacheEmpty = errors.New("stream: pool cache empty")
)

// Subscribe : monitoring or filtering of subscriptions
type Subscribe func(key string) error

// Unsubscribe : function callback on subscription closing
type Unsubscribe func(key string)

// FilterObjectFn is a function type for filtering single objects.
type FilterObjectFn func(key string, obj meta.Object) (meta.Object, error)

// FilterListFn is a function type for filtering object lists.
type FilterListFn func(key string, objs []meta.Object) ([]meta.Object, error)

// Conn extends the websocket connection with a mutex
// https://godoc.org/github.com/gorilla/websocket#hdr-Concurrency
type Conn struct {
	mutex sync.Mutex
	conn  *websocket.Conn
}

// Pool of key filtered connections
type Pool struct {
	mutex       sync.RWMutex
	Key         string
	cache       Cache
	connections []*Conn
}

// DefaultWriteTimeout is the default timeout for WebSocket write operations.
const DefaultWriteTimeout = 15 * time.Second

// DefaultParallelThreshold is the minimum number of connections before parallel broadcast is used.
const DefaultParallelThreshold = 6

// Stream a group of pools
type Stream struct {
	mutex             sync.RWMutex
	OnSubscribe       Subscribe
	OnUnsubscribe     Unsubscribe
	ForcePatch        bool
	NoPatch           bool
	WriteTimeout      time.Duration    // timeout for WebSocket writes, defaults to DefaultWriteTimeout
	ParallelThreshold int              // minimum connections for parallel broadcast, defaults to DefaultParallelThreshold
	clockPool         *Pool            // dedicated clock pool (was index 0)
	pools             map[string]*Pool // O(1) lookup by key
	poolIndex         *poolTrie        // trie for O(k) path matching in Broadcast
	Console           *coat.Console
}

// BroadcastOpt options for broadcasting storage events
type BroadcastOpt struct {
	Key          string
	Operation    string       // "set" or "del"
	Object       *meta.Object // The object that was set/deleted
	FilterObject FilterObjectFn
	FilterList   FilterListFn
	Static       bool
}

// Cache holds version and decoded objects for efficient broadcast
type Cache struct {
	Version int64
	Objects []meta.Object // For list subscriptions (glob paths)
	Object  *meta.Object  // For single object subscriptions (non-glob)
}

// insertSorted inserts obj into list maintaining ascending Created order (oldest first).
// Returns new list and position where inserted.
func insertSorted(list []meta.Object, obj meta.Object) ([]meta.Object, int) {
	// Find position for ascending order (oldest first, newest at end)
	pos := len(list)
	for i, item := range list {
		if obj.Created < item.Created {
			pos = i
			break
		}
	}
	// Insert at pos
	list = append(list, meta.Object{})
	copy(list[pos+1:], list[pos:])
	list[pos] = obj
	return list, pos
}

// updateInList finds and updates obj in list by matching Path.
// Returns new list, position, and whether it was found.
func updateInList(list []meta.Object, obj meta.Object) ([]meta.Object, int, bool) {
	for i, item := range list {
		if item.Path == obj.Path {
			list[i] = obj
			return list, i, true
		}
	}
	return list, -1, false
}

// removeFromList removes obj from list by matching Path.
// Returns new list, position where removed, and whether it was found.
func removeFromList(list []meta.Object, obj meta.Object) ([]meta.Object, int, bool) {
	for i, item := range list {
		if item.Path == obj.Path {
			return append(list[:i], list[i+1:]...), i, true
		}
	}
	return list, -1, false
}

// generateListPatch creates a JSON patch for a single list operation.
func generateListPatch(op string, pos int, obj *meta.Object) ([]byte, error) {
	switch op {
	case "add":
		objBytes, err := meta.Encode(*obj)
		if err != nil {
			return nil, err
		}
		// Use json.Marshal for proper escaping
		patch := []jsonpatch.Operation{{Operation: "add", Path: "/" + strconv.Itoa(pos), Value: json.RawMessage(objBytes)}}
		return json.Marshal(patch)
	case "replace":
		objBytes, err := meta.Encode(*obj)
		if err != nil {
			return nil, err
		}
		patch := []jsonpatch.Operation{{Operation: "replace", Path: "/" + strconv.Itoa(pos), Value: json.RawMessage(objBytes)}}
		return json.Marshal(patch)
	case "remove":
		patch := []jsonpatch.Operation{{Operation: "remove", Path: "/" + strconv.Itoa(pos)}}
		return json.Marshal(patch)
	}
	return nil, errors.New("stream: unknown patch operation")
}

// generateAddRemovePatch creates a JSON patch that adds an item and removes another.
// The remove is applied first (at removePos), then add (at addPos).
// Since remove happens first, if addPos > removePos, the actual add position is addPos.
// If addPos <= removePos, the add position is still addPos since remove shifts items down.
func generateAddRemovePatch(addPos int, obj *meta.Object, removePos int) ([]byte, error) {
	objBytes, err := meta.Encode(*obj)
	if err != nil {
		return nil, err
	}
	// Order matters: remove first, then add
	// After remove at removePos, indices >= removePos shift down by 1
	// So if addPos > removePos, the effective add position is addPos (since we're adding to the new shorter list)
	patch := []jsonpatch.Operation{
		{Operation: "remove", Path: "/" + strconv.Itoa(removePos)},
		{Operation: "add", Path: "/" + strconv.Itoa(addPos), Value: json.RawMessage(objBytes)},
	}
	return json.Marshal(patch)
}

// generateObjectPatch creates a JSON patch for a single object change.
func generateObjectPatch(oldObj, newObj *meta.Object) ([]byte, bool, error) {
	if oldObj == nil || oldObj.Created == 0 {
		// No previous object - send as snapshot
		return nil, true, nil
	}
	oldBytes, err := meta.Encode(*oldObj)
	if err != nil {
		return nil, true, nil
	}
	newBytes, err := meta.Encode(*newObj)
	if err != nil {
		return nil, true, nil
	}
	patch, err := jsonpatch.CreatePatch(oldBytes, newBytes)
	if err != nil {
		return nil, true, nil
	}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return nil, true, nil
	}
	return patchBytes, false, nil
}

var StreamUpgrader = websocket.Upgrader{
	// define the upgrade success
	CheckOrigin: func(r *http.Request) bool {
		return r.Header.Get("Upgrade") == "websocket"
	},
	Subprotocols: []string{"bearer"},
}

func (sm *Stream) getPool(key string) *Pool {
	// Clock pool uses empty key
	if key == "" {
		return sm.clockPool
	}
	return sm.pools[key]
}

func (sm *Stream) InitClock() {
	if sm.pools == nil {
		sm.pools = make(map[string]*Pool)
	}
	if sm.poolIndex == nil {
		sm.poolIndex = newPoolTrie()
	}
	if sm.clockPool == nil {
		sm.clockPool = &Pool{Key: ""}
	}
	if sm.WriteTimeout == 0 {
		sm.WriteTimeout = DefaultWriteTimeout
	}
}

// PreallocatePools pre-allocates Pool structs for known paths.
// This avoids allocation overhead when the first connection arrives.
// Useful when you know the set of paths that will be used upfront.
func (sm *Stream) PreallocatePools(paths []string) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if sm.pools == nil {
		sm.pools = make(map[string]*Pool, len(paths))
	}
	if sm.poolIndex == nil {
		sm.poolIndex = newPoolTrie()
	}

	for _, path := range paths {
		if _, exists := sm.pools[path]; !exists {
			pool := &Pool{
				Key:         path,
				connections: make([]*Conn, 0, 4), // Pre-allocate small capacity
			}
			sm.pools[path] = pool
			sm.poolIndex.insert(path, pool)
		}
	}
}

// New stream on a key
func (sm *Stream) New(key string, w http.ResponseWriter, r *http.Request) (*Conn, error) {
	err := sm.OnSubscribe(key)
	if err != nil {
		return nil, err
	}

	wsClient, err := StreamUpgrader.Upgrade(w, r, nil)
	if err != nil {
		sm.Console.Err("socketUpgradeError["+key+"]", err)
		return nil, err
	}

	return sm.new(key, wsClient), nil
}

// Open a connection for a key
func (sm *Stream) new(key string, wsClient *websocket.Conn) *Conn {
	client := &Conn{
		conn:  wsClient,
		mutex: sync.Mutex{},
	}

	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	// Clock connections (empty key) go to dedicated clockPool
	if key == "" {
		if sm.clockPool == nil {
			sm.clockPool = &Pool{Key: ""}
		}
		sm.clockPool.mutex.Lock()
		sm.clockPool.connections = append(sm.clockPool.connections, client)
		sm.Console.Log("clock connections: ", len(sm.clockPool.connections))
		sm.clockPool.mutex.Unlock()
		return client
	}

	pool := sm.getPool(key)
	if pool == nil {
		// create a pool
		pool = &Pool{
			Key:         key,
			connections: []*Conn{client},
		}
		sm.pools[key] = pool
		sm.poolIndex.insert(key, pool)
		sm.Console.Log("connections["+key+"]: ", len(pool.connections))
		return client
	}

	// use existing pool - need pool.mutex to avoid race with broadcastPool
	pool.mutex.Lock()
	pool.connections = append(pool.connections, client)
	sm.Console.Log("connections["+key+"]: ", len(pool.connections))
	pool.mutex.Unlock()
	return client
}

// removeConn removes a connection from a slice using swap-and-truncate.
// This is O(1) instead of O(n) and doesn't allocate a new slice.
func removeConn(conns []*Conn, client *Conn) []*Conn {
	for i, v := range conns {
		if v == client {
			// Swap with last element and truncate
			last := len(conns) - 1
			conns[i] = conns[last]
			conns[last] = nil // Allow GC
			return conns[:last]
		}
	}
	return conns
}

// Close client connection
func (sm *Stream) Close(key string, client *Conn) {
	sm.mutex.Lock()
	// Handle clock pool (empty key) specially
	if key == "" {
		if sm.clockPool != nil {
			sm.clockPool.mutex.Lock()
			sm.clockPool.connections = removeConn(sm.clockPool.connections, client)
			sm.clockPool.mutex.Unlock()
		}
	} else {
		pool := sm.getPool(key)
		if pool != nil {
			pool.mutex.Lock()
			pool.connections = removeConn(pool.connections, client)
			pool.mutex.Unlock()
		}
	}
	sm.mutex.Unlock()
	go sm.OnUnsubscribe(key)
	client.conn.Close()
}

// Broadcast will look for pools that match a path and broadcast updates.
// Uses a trie for O(k) path matching where k is the number of path segments,
// instead of O(n) where n is the total number of pools.
// Lock contention is minimized by only holding stream-level RLock during trie lookup,
// then using per-pool locks for the actual broadcast.
func (sm *Stream) Broadcast(path string, opt BroadcastOpt) {
	// Get matching pools under stream RLock (brief)
	sm.mutex.RLock()
	matchingPools := sm.poolIndex.findMatching(path)
	sm.mutex.RUnlock()

	// Process each pool with only per-pool locking
	for _, pool := range matchingPools {
		pool.mutex.Lock()
		if isGlobKey(pool.Key) {
			sm.broadcastList(pool, opt)
		} else {
			sm.broadcastObject(pool, opt)
		}
		pool.mutex.Unlock()
	}
}

// isGlobKey checks if a pool key contains a glob pattern
func isGlobKey(key string) bool {
	return strings.Contains(key, "*")
}

// broadcastList handles broadcasting to list subscriptions (glob paths)
func (sm *Stream) broadcastList(pool *Pool, opt BroadcastOpt) {
	obj := opt.Object
	if obj == nil {
		return
	}

	// Apply object filter first to see if this object passes
	filtered, filterErr := opt.FilterObject(pool.Key, *obj)

	switch opt.Operation {
	case "set":
		if filterErr != nil {
			// Object filtered out - remove from list if it exists
			newList, pos, found := removeFromList(pool.cache.Objects, *obj)
			if found {
				pool.cache.Objects = newList
				// Apply list filter after removal
				finalList, _ := opt.FilterList(pool.Key, pool.cache.Objects)
				pool.cache.Objects = finalList
				sm.sendListBroadcast(pool, "remove", pos, nil)
			}
			return
		}

		// Check if this is an update or insert
		newList, _, found := updateInList(pool.cache.Objects, filtered)
		if found {
			// Update existing item
			pool.cache.Objects = newList
			// Apply list filter (may re-sort)
			finalList, _ := opt.FilterList(pool.Key, pool.cache.Objects)
			pool.cache.Objects = finalList
			// Find actual position after filter (may have re-sorted)
			actualPos := 0
			for i, item := range finalList {
				if item.Path == filtered.Path {
					actualPos = i
					break
				}
			}
			sm.sendListBroadcast(pool, "replace", actualPos, &filtered)
		} else {
			// Insert new item
			oldLen := len(pool.cache.Objects)
			newList, _ := insertSorted(pool.cache.Objects, filtered)
			// Apply list filter (may limit size and re-sort)
			finalList, _ := opt.FilterList(pool.Key, newList)
			pool.cache.Objects = finalList

			// Find actual position of our item in the filtered list
			// (FilterList may have re-sorted the list)
			actualPos := -1
			for i, item := range finalList {
				if item.Path == filtered.Path {
					actualPos = i
					break
				}
			}

			// Check if an item was pushed out due to limit
			itemPushedOut := len(finalList) < len(newList)

			if actualPos >= 0 && itemPushedOut {
				// Item is in the filtered list AND another was pushed out
				// Send combined add+remove patch
				// The pushed-out item was at the last position in the old list (oldLen - 1)
				// since the filter keeps the newest items and removes the oldest
				sm.sendAddRemoveBroadcast(pool, actualPos, &filtered, oldLen-1)
			} else if actualPos >= 0 {
				// Item is in the filtered list, no item pushed out
				sm.sendListBroadcast(pool, "add", actualPos, &filtered)
			} else if itemPushedOut && len(finalList) == oldLen {
				// Item was added but filtered out, and another was pushed out due to limit
				sm.sendListBroadcast(pool, "remove", len(finalList), nil)
			}
		}

	case "del":
		newList, pos, found := removeFromList(pool.cache.Objects, *obj)
		if found {
			pool.cache.Objects = newList
			sm.sendListBroadcast(pool, "remove", pos, nil)
		}
	}
}

// sendListBroadcast sends a list broadcast, using snapshot if NoPatch is enabled
func (sm *Stream) sendListBroadcast(pool *Pool, op string, pos int, obj *meta.Object) {
	version := sm.nextVersion(pool)

	if sm.NoPatch {
		// Send full snapshot
		data, err := meta.Encode(pool.cache.Objects)
		if err != nil {
			return
		}
		sm.Console.Log("SEND broadcast[" + pool.Key + "]: snapshot (NoPatch)")
		sm.broadcastPool(pool, data, true, version)
		return
	}

	patch, err := generateListPatch(op, pos, obj)
	if err != nil {
		sm.Console.Err("generateListPatch failed", err)
		return
	}
	sm.Console.Log("SEND broadcast[" + pool.Key + "]: " + op + " at " + strconv.Itoa(pos))
	sm.broadcastPool(pool, patch, false, version)
}

// sendAddRemoveBroadcast sends a combined add+remove broadcast for when an item is added
// and another is pushed out due to a limit filter.
func (sm *Stream) sendAddRemoveBroadcast(pool *Pool, addPos int, obj *meta.Object, removePos int) {
	version := sm.nextVersion(pool)

	if sm.NoPatch {
		// Send full snapshot
		data, err := meta.Encode(pool.cache.Objects)
		if err != nil {
			return
		}
		sm.Console.Log("SEND broadcast[" + pool.Key + "]: snapshot (NoPatch)")
		sm.broadcastPool(pool, data, true, version)
		return
	}

	patch, err := generateAddRemovePatch(addPos, obj, removePos)
	if err != nil {
		sm.Console.Err("generateAddRemovePatch failed", err)
		return
	}
	sm.Console.Log("SEND broadcast[" + pool.Key + "]: add at " + strconv.Itoa(addPos) + " + remove at " + strconv.Itoa(removePos))
	sm.broadcastPool(pool, patch, false, version)
}

// broadcastObject handles broadcasting to single object subscriptions (non-glob paths)
func (sm *Stream) broadcastObject(pool *Pool, opt BroadcastOpt) {
	obj := opt.Object

	switch opt.Operation {
	case "set":
		if obj == nil {
			return
		}
		filtered, err := opt.FilterObject(pool.Key, *obj)
		if err != nil {
			// Filtered out - send empty object
			filtered = meta.Object{}
		}

		// Save old object before updating cache
		oldObj := pool.cache.Object
		pool.cache.Object = &filtered
		version := sm.nextVersion(pool)

		// Encode the filtered object
		data, encErr := meta.Encode(filtered)
		if encErr != nil {
			data = meta.EmptyObject
		}

		// NoPatch mode - send snapshot
		if sm.NoPatch {
			sm.Console.Log("SEND broadcast[" + pool.Key + "]: snapshot (NoPatch)")
			sm.broadcastPool(pool, data, true, version)
			return
		}

		patch, snapshot, _ := generateObjectPatch(oldObj, &filtered)
		if snapshot || patch == nil {
			sm.Console.Log("SEND broadcast[" + pool.Key + "]: snapshot")
			sm.broadcastPool(pool, data, true, version)
		} else {
			sm.Console.Log("SEND broadcast[" + pool.Key + "]: patch")
			sm.broadcastPool(pool, patch, false, version)
		}

	case "del":
		empty := meta.Object{}
		oldObj := pool.cache.Object
		pool.cache.Object = &empty
		version := sm.nextVersion(pool)

		if sm.NoPatch {
			sm.Console.Log("SEND broadcast[" + pool.Key + "]: del snapshot (NoPatch)")
			sm.broadcastPool(pool, meta.EmptyObject, true, version)
			return
		}

		patch, snapshot, _ := generateObjectPatch(oldObj, &empty)
		if snapshot || patch == nil {
			sm.Console.Log("SEND broadcast[" + pool.Key + "]: del snapshot")
			sm.broadcastPool(pool, meta.EmptyObject, true, version)
		} else {
			sm.Console.Log("SEND broadcast[" + pool.Key + "]: del patch")
			sm.broadcastPool(pool, patch, false, version)
		}
	}
}

// nextVersion generates a new version number for a pool
func (sm *Stream) nextVersion(pool *Pool) int64 {
	pool.cache.Version = monotonic.Now()
	return pool.cache.Version
}

// broadcastPool sends message to all connections in a pool.
// Uses parallel goroutines for large pools to improve throughput.
func (sm *Stream) broadcastPool(pool *Pool, data []byte, snapshot bool, version int64) {
	numConns := len(pool.connections)
	threshold := sm.ParallelThreshold
	if threshold == 0 {
		threshold = DefaultParallelThreshold
	}

	// For small pools, sequential is faster (no goroutine overhead)
	if numConns < threshold {
		for _, client := range pool.connections {
			sm.Write(client, data, snapshot, version)
		}
		return
	}

	// For large pools, use parallel broadcast
	// Pre-build the message once to avoid rebuilding per connection
	msg := buildMessage(data, snapshot, version)
	var wg sync.WaitGroup
	wg.Add(numConns)
	for _, client := range pool.connections {
		go func(c *Conn) {
			defer wg.Done()
			sm.writeBytesPrebuilt(c, msg)
		}(client)
	}
	wg.Wait()
}

// writeBytesPrebuilt writes a pre-built message to a connection.
// This avoids rebuilding the message for each connection in parallel broadcast.
func (sm *Stream) writeBytesPrebuilt(client *Conn, msg []byte) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	client.conn.SetWriteDeadline(time.Now().Add(sm.WriteTimeout))
	err := client.conn.WriteMessage(websocket.BinaryMessage, msg)
	if err != nil {
		client.conn.Close()
		sm.Console.Log("writeStreamErr: ", err)
	}
}

// buildMessage constructs the WebSocket message JSON using append for efficiency.
// This avoids multiple string allocations from concatenation.
func buildMessage(data []byte, snapshot bool, version int64) []byte {
	// Pre-calculate capacity: {"snapshot":true,"version":"hexversion","data":...}
	// Fixed overhead: ~40 bytes + version hex (max 16) + data length
	capacity := 50 + len(data)
	buf := make([]byte, 0, capacity)

	buf = append(buf, `{"snapshot":`...)
	buf = strconv.AppendBool(buf, snapshot)
	buf = append(buf, `,"version":"`...)
	buf = strconv.AppendInt(buf, version, 16)
	buf = append(buf, `","data":`...)
	buf = append(buf, data...)
	buf = append(buf, '}')

	return buf
}

// WriteBytes will write data to a ws connection without string conversion.
// This is more efficient when data is already a []byte.
func (sm *Stream) Write(client *Conn, data []byte, snapshot bool, version int64) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	client.conn.SetWriteDeadline(time.Now().Add(sm.WriteTimeout))
	err := client.conn.WriteMessage(websocket.BinaryMessage, buildMessage(data, snapshot, version))

	if err != nil {
		client.conn.Close()
		sm.Console.Log("writeStreamErr: ", err)
	}
}

// Read will keep alive the ws connection
func (sm *Stream) Read(key string, client *Conn) {
	for {
		_, _, err := client.conn.NextReader()
		if err != nil {
			sm.Console.Err("readSocketError["+key+"]", err)
			sm.Close(key, client)
			break
		}
	}
}

// InitCacheObjects initializes a pool's cache with decoded objects for list subscriptions.
// Only sets version if cache was not previously initialized.
func (sm *Stream) InitCacheObjects(key string, objects []meta.Object) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	pool := sm.getPool(key)
	if pool == nil {
		return
	}
	pool.mutex.Lock()
	defer pool.mutex.Unlock()
	pool.cache.Objects = objects
	if pool.cache.Version == 0 {
		pool.cache.Version = monotonic.Now()
	}
}

// InitCacheObjectsWithVersion initializes a pool's cache and returns the version.
// Creates the pool if it doesn't exist.
func (sm *Stream) InitCacheObjectsWithVersion(key string, objects []meta.Object) int64 {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	pool := sm.getPool(key)
	if pool == nil {
		// Create pool
		now := monotonic.Now()
		pool = &Pool{
			Key: key,
			cache: Cache{
				Version: now,
				Objects: objects,
			},
			connections: []*Conn{},
		}
		sm.pools[key] = pool
		sm.poolIndex.insert(key, pool)
		return now
	}
	pool.mutex.Lock()
	defer pool.mutex.Unlock()
	pool.cache.Objects = objects
	if pool.cache.Version == 0 {
		pool.cache.Version = monotonic.Now()
	}
	return pool.cache.Version
}

// InitCacheObject initializes a pool's cache with a decoded object for single object subscriptions.
// Only sets version if cache was not previously initialized.
func (sm *Stream) InitCacheObject(key string, obj *meta.Object) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	pool := sm.getPool(key)
	if pool == nil {
		return
	}
	pool.mutex.Lock()
	defer pool.mutex.Unlock()
	pool.cache.Object = obj
	if pool.cache.Version == 0 {
		pool.cache.Version = monotonic.Now()
	}
}

// InitCacheObjectWithVersion initializes a pool's cache and returns the version.
// Creates the pool if it doesn't exist.
func (sm *Stream) InitCacheObjectWithVersion(key string, obj *meta.Object) int64 {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	pool := sm.getPool(key)
	if pool == nil {
		// Create pool
		now := monotonic.Now()
		pool = &Pool{
			Key: key,
			cache: Cache{
				Version: now,
				Object:  obj,
			},
			connections: []*Conn{},
		}
		sm.pools[key] = pool
		sm.poolIndex.insert(key, pool)
		return now
	}
	pool.mutex.Lock()
	defer pool.mutex.Unlock()
	pool.cache.Object = obj
	if pool.cache.Version == 0 {
		pool.cache.Version = monotonic.Now()
	}
	return pool.cache.Version
}

// GetCacheVersion by key
func (sm *Stream) GetCacheVersion(key string) (int64, error) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	pool := sm.getPool(key)
	if pool == nil {
		return 0, ErrPoolNotFound
	}
	pool.mutex.RLock()
	defer pool.mutex.RUnlock()
	if pool.cache.Version == 0 {
		return 0, ErrPoolCacheEmpty
	}

	return pool.cache.Version, nil
}
