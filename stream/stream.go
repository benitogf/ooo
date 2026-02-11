package stream

import (
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/benitogf/ooo/meta"
	"github.com/benitogf/ooo/monotonic"

	"github.com/benitogf/coat"
	"github.com/gorilla/websocket"
)

var (
	ErrPoolNotFound   = errors.New("stream: pool not found")
	ErrPoolCacheEmpty = errors.New("stream: pool cache empty")
	ErrHijacked       = errors.New("stream: connection hijacked")
)

// Subscribe : monitoring or filtering of subscriptions
type Subscribe func(key string) error

// Unsubscribe : function callback on subscription closing
type Unsubscribe func(key string)

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

// Cache holds version and decoded objects for efficient broadcast
type Cache struct {
	Version int64
	Objects []meta.Object // For list subscriptions (glob paths)
	Object  *meta.Object  // For single object subscriptions (non-glob)
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

var streamUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return r.Header.Get("Upgrade") == "websocket"
	},
	Subprotocols: []string{"bearer"},
}

func (sm *Stream) getPool(key string) *Pool {
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
				connections: make([]*Conn, 0, 4),
			}
			sm.pools[path] = pool
			sm.poolIndex.insert(path, pool)
		}
	}
}

// New creates a WebSocket connection and sends the initial snapshot
// BEFORE adding the connection to the broadcast pool. This prevents a race condition
// where broadcasts could reach the client before the initial snapshot is sent.
// Pass nil for data if no initial snapshot is needed (e.g., clock connections).
func (sm *Stream) New(key string, w http.ResponseWriter, r *http.Request, data []byte, version int64) (*Conn, error) {
	err := sm.OnSubscribe(key)
	if err != nil {
		return nil, err
	}

	wsClient, err := streamUpgrader.Upgrade(w, r, nil)
	if err != nil {
		sm.Console.Err("stream: socketUpgradeError["+key+"]", err)
		return nil, err
	}

	client := &Conn{
		conn:  wsClient,
		mutex: sync.Mutex{},
	}

	// Send initial snapshot BEFORE adding to pool
	if data != nil {
		client.mutex.Lock()
		client.conn.SetWriteDeadline(time.Now().Add(sm.WriteTimeout))
		err = client.conn.WriteMessage(websocket.BinaryMessage, buildMessage(data, true, version))
		client.mutex.Unlock()
		if err != nil {
			client.conn.Close()
			// Wrap with ErrHijacked since connection was already upgraded
			return nil, errors.Join(ErrHijacked, err)
		}
	}

	// Now add to pool - broadcasts can now reach this client
	sm.addConnToPool(key, client)
	return client, nil
}

// addConnToPool adds an existing connection to the appropriate pool
func (sm *Stream) addConnToPool(key string, client *Conn) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if key == "" {
		if sm.clockPool == nil {
			sm.clockPool = &Pool{Key: ""}
		}
		sm.clockPool.mutex.Lock()
		sm.clockPool.connections = append(sm.clockPool.connections, client)
		sm.Console.Log("stream:connections[clock]: ", len(sm.clockPool.connections))
		sm.clockPool.mutex.Unlock()
		return
	}

	pool := sm.getPool(key)
	if pool == nil {
		pool = &Pool{
			Key:         key,
			connections: []*Conn{client},
		}
		sm.pools[key] = pool
		sm.poolIndex.insert(key, pool)
		sm.Console.Log("stream:connections["+key+"]: ", len(pool.connections))
		return
	}

	pool.mutex.Lock()
	pool.connections = append(pool.connections, client)
	sm.Console.Log("stream: connections["+key+"]: ", len(pool.connections))
	pool.mutex.Unlock()
}

func (sm *Stream) newConn(key string, wsClient *websocket.Conn) *Conn {
	client := &Conn{
		conn:  wsClient,
		mutex: sync.Mutex{},
	}

	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if key == "" {
		if sm.clockPool == nil {
			sm.clockPool = &Pool{Key: ""}
		}
		sm.clockPool.mutex.Lock()
		sm.clockPool.connections = append(sm.clockPool.connections, client)
		sm.Console.Log("stream:connections[clock]: ", len(sm.clockPool.connections))
		sm.clockPool.mutex.Unlock()
		return client
	}

	pool := sm.getPool(key)
	if pool == nil {
		pool = &Pool{
			Key:         key,
			connections: []*Conn{client},
		}
		sm.pools[key] = pool
		sm.poolIndex.insert(key, pool)
		sm.Console.Log("stream:connections["+key+"]: ", len(pool.connections))
		return client
	}

	pool.mutex.Lock()
	pool.connections = append(pool.connections, client)
	sm.Console.Log("stream: connections["+key+"]: ", len(pool.connections))
	pool.mutex.Unlock()
	return client
}

func removeConn(conns []*Conn, client *Conn) []*Conn {
	for i, v := range conns {
		if v == client {
			last := len(conns) - 1
			conns[i] = conns[last]
			conns[last] = nil
			return conns[:last]
		}
	}
	return conns
}

// Close client connection
func (sm *Stream) Close(key string, client *Conn) {
	sm.mutex.Lock()
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

// CloseAll forcefully closes all connections in all pools
func (sm *Stream) CloseAll() {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	// Close clock pool connections
	if sm.clockPool != nil {
		sm.clockPool.mutex.Lock()
		for _, client := range sm.clockPool.connections {
			client.conn.Close()
		}
		sm.clockPool.connections = nil
		sm.clockPool.mutex.Unlock()
	}

	// Close all pool connections
	for _, pool := range sm.pools {
		pool.mutex.Lock()
		for _, client := range pool.connections {
			client.conn.Close()
		}
		pool.connections = nil
		pool.mutex.Unlock()
	}
}

// Broadcast will look for pools that match a path and broadcast updates.
func (sm *Stream) Broadcast(path string, opt BroadcastOpt) {
	sm.mutex.RLock()
	matchingPools := sm.poolIndex.findMatching(path)
	sm.mutex.RUnlock()

	for _, pool := range matchingPools {
		pool.mutex.Lock()
		result := ProcessBroadcast(&pool.cache, pool.Key, opt.Operation, opt.Object, opt.FilterObject, opt.FilterList, sm.NoPatch)
		if !result.Skip {
			version := sm.nextVersion(pool)
			// sm.logBroadcast(pool.Key, result.Snapshot)
			sm.broadcastPool(pool, result.Data, result.Snapshot, version)
		}
		pool.mutex.Unlock()
	}
}

// func (sm *Stream) logBroadcast(key string, snapshot bool) {
// 	if snapshot {
// 		sm.Console.Log("SEND broadcast[" + key + "]: snapshot")
// 	} else {
// 		sm.Console.Log("SEND broadcast[" + key + "]: patch")
// 	}
// }

func (sm *Stream) nextVersion(pool *Pool) int64 {
	pool.cache.Version = monotonic.Now()
	return pool.cache.Version
}

func (sm *Stream) broadcastPool(pool *Pool, data []byte, snapshot bool, version int64) {
	numConns := len(pool.connections)
	threshold := sm.ParallelThreshold
	if threshold == 0 {
		threshold = DefaultParallelThreshold
	}

	msg := buildMessage(data, snapshot, version)

	var failedConns []*Conn

	if numConns < threshold {
		for _, client := range pool.connections {
			err := sm.writeBytesPrebuilt(client, msg)
			if err != nil {
				failedConns = append(failedConns, client)
			}
		}
	} else {
		var mu sync.Mutex
		var wg sync.WaitGroup
		wg.Add(numConns)
		for _, client := range pool.connections {
			go func(c *Conn) {
				defer wg.Done()
				err := sm.writeBytesPrebuilt(c, msg)
				if err != nil {
					mu.Lock()
					failedConns = append(failedConns, c)
					mu.Unlock()
				}
			}(client)
		}
		wg.Wait()
	}

	// Remove failed connections from pool while we still hold the pool lock
	for _, client := range failedConns {
		pool.connections = removeConn(pool.connections, client)
		go sm.OnUnsubscribe(pool.Key)
	}
}

func (sm *Stream) writeBytesPrebuilt(client *Conn, msg []byte) error {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	timeout := sm.WriteTimeout
	if timeout == 0 {
		timeout = DefaultWriteTimeout
	}
	client.conn.SetWriteDeadline(time.Now().Add(timeout))
	err := client.conn.WriteMessage(websocket.BinaryMessage, msg)
	if err != nil {
		client.conn.Close()
		sm.Console.Err("stream:writeBytesPrebuilt:writeStreamErr: ", err)
		return err
	}
	return nil
}

func buildMessage(data []byte, snapshot bool, version int64) []byte {
	// Pre-calculate exact capacity needed to avoid reallocations
	// Format: {"snapshot":true/false,"version":"HEX","data":DATA}
	// Fixed overhead: {"snapshot":,"version":"","data":} = 32 bytes
	// Bool: 4-5 bytes, version hex: ~16 bytes max
	capacity := 52 + len(data)
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

// Write sends data to a ws connection
func (sm *Stream) Write(client *Conn, data []byte, snapshot bool, version int64) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	client.conn.SetWriteDeadline(time.Now().Add(sm.WriteTimeout))
	err := client.conn.WriteMessage(websocket.BinaryMessage, buildMessage(data, snapshot, version))

	if err != nil {
		client.conn.Close()
		sm.Console.Err("stream:Write:writeStreamErr: ", err)
	}
}

// Read will keep alive the ws connection
func (sm *Stream) Read(key string, client *Conn) {
	for {
		_, _, err := client.conn.NextReader()
		if err != nil {
			sm.Console.Err("stream:Read:readSocketError["+key+"]", err)
			sm.Close(key, client)
			break
		}
	}
}

// InitCacheObjectsWithVersion initializes a pool's cache and returns the version.
func (sm *Stream) InitCacheObjectsWithVersion(key string, objects []meta.Object) int64 {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	pool := sm.getPool(key)
	if pool == nil {
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

// InitCacheObjectWithVersion initializes a pool's cache and returns the version.
func (sm *Stream) InitCacheObjectWithVersion(key string, obj *meta.Object) int64 {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	pool := sm.getPool(key)
	if pool == nil {
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

// PoolInfo contains information about a connection pool
type PoolInfo struct {
	Key         string `json:"key"`
	Connections int    `json:"connections"`
}

// GetState returns information about all active connection pools
func (sm *Stream) GetState() []PoolInfo {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	var result []PoolInfo

	// Add clock pool if it has connections
	if sm.clockPool != nil {
		sm.clockPool.mutex.RLock()
		if len(sm.clockPool.connections) > 0 {
			result = append(result, PoolInfo{
				Key:         "(clock)",
				Connections: len(sm.clockPool.connections),
			})
		}
		sm.clockPool.mutex.RUnlock()
	}

	// Add all other pools with connections
	for key, pool := range sm.pools {
		pool.mutex.RLock()
		connCount := len(pool.connections)
		pool.mutex.RUnlock()
		if connCount > 0 {
			result = append(result, PoolInfo{
				Key:         key,
				Connections: connCount,
			})
		}
	}

	return result
}
