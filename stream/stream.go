package stream

import (
	"bytes"
	"errors"
	"net/http"
	"strconv"
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

// GetFn is a function type for retrieving data by key from storage.
type GetFn func(key string) ([]byte, error)

// EncodeFn is a function type for encoding data to a string format.
type EncodeFn func(data []byte) string

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

type BroadcastOpt struct {
	Get      GetFn
	Callback func()
}

// Cache holds version and data
type Cache struct {
	Version int64
	Data    []byte
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
		data, err := opt.Get(pool.Key)
		// this error means that the broadcast was filtered
		if err != nil {
			sm.Console.Err("broadcast["+pool.Key+"]: failed to get data", err)
			pool.mutex.Unlock()
			continue
		}

		// Fast path: skip broadcast if filtered data is unchanged
		// Only check when SkipUnchanged is enabled and cache has been initialized
		// This optimization is particularly useful for LimitFilter where
		// deletes beyond the limit don't change the visible data
		if pool.cache.Version > 0 && bytes.Equal(pool.cache.Data, data) {
			sm.Console.Log("SKIP broadcast[" + pool.Key + "]: data unchanged, version=" + strconv.FormatInt(pool.cache.Version, 10))
			pool.mutex.Unlock()
			if opt.Callback != nil {
				opt.Callback()
			}
			continue
		}

		modifiedData, snapshot, version := sm.patchPool(pool, data)
		sm.Console.Log("SEND broadcast[" + pool.Key + "]: version=" + strconv.FormatInt(pool.cache.Version, 10))
		sm.broadcastPool(pool, modifiedData, snapshot, version)
		pool.mutex.Unlock()
		if opt.Callback != nil {
			opt.Callback()
		}
	}
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

// patchPool will return either the snapshot or the patch for a pool
//
// patch, false (patch)
//
// snapshot, true (snapshot)
func (sm *Stream) patchPool(pool *Pool, data []byte) ([]byte, bool, int64) {
	// no patch, only snapshot
	if sm.NoPatch {
		version := sm.setCachePool(pool, data)
		return data, true, version
	}

	patch, err := jsonpatch.CreatePatch(pool.cache.Data, data)
	if err != nil {
		sm.Console.Err("patch create failed", err)
		version := sm.setCachePool(pool, data)
		return data, true, version
	}
	version := sm.setCachePool(pool, data)
	operations, err := json.Marshal(patch)
	if err != nil {
		sm.Console.Err("patch decode failed", err)
		return data, true, version
	}
	// don't send the operations if they exceed the data size
	if !sm.ForcePatch && len(operations) > len(data) {
		return data, true, version
	}

	return operations, false, version
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

// setCachePool will store data in a pool's cache
func (sm *Stream) setCachePool(pool *Pool, data []byte) int64 {
	// log.Println("SET cache version ", pool.Key, strconv.FormatInt(monotonic.Now(), 16))
	now := monotonic.Now()
	pool.cache.Version = now
	pool.cache.Data = data
	return now
}

// setCache by key
func (sm *Stream) setCache(key string, data []byte) int64 {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	pool := sm.getPool(key)
	if pool == nil {
		now := monotonic.Now()
		// create a pool
		pool = &Pool{
			Key: key,
			cache: Cache{
				Version: now,
				Data:    data,
			},
			connections: []*Conn{},
		}
		sm.pools[key] = pool
		return now
	}

	return sm.setCachePool(pool, data)
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
	if len(pool.cache.Data) == 0 {
		return 0, ErrPoolCacheEmpty
	}

	return pool.cache.Version, nil
}

func (sm *Stream) Refresh(path string, getDataFn GetFn) (Cache, error) {
	raw, err := getDataFn(path)
	if err != nil {
		return Cache{}, err
	}
	if len(raw) == 0 {
		raw = meta.EmptyObject
	}
	cache := Cache{
		Data: raw,
	}
	cacheVersion, err := sm.GetCacheVersion(path)
	if err != nil {
		newVersion := sm.setCache(path, raw)
		// log.Println("NEW version", path, strconv.FormatInt(newVersion, 16))
		cache.Version = newVersion
		return cache, nil
	}

	// log.Println("READ version", path, strconv.FormatInt(cacheVersion, 16))
	cache.Version = cacheVersion
	return cache, nil
}
