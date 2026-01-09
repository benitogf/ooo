package stream

import (
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/benitogf/coat"
	"github.com/benitogf/ooo/meta"
	"github.com/benitogf/ooo/monotonic"
	hjhttptest "github.com/getlantern/httptest"
	"github.com/stretchr/testify/require"
)

func makeStreamRequestMock(url string) (*http.Request, *hjhttptest.HijackableResponseRecorder) {
	req := httptest.NewRequest("GET", url, nil)
	req.Header.Add("Connection", "upgrade")
	req.Header.Add("Sec-Websocket-Version", "13")
	req.Header.Add("Sec-Websocket-Key", "4aRdFZG5uYrEUw8dsNLW6g==")
	req.Header.Add("Upgrade", "websocket")
	w := hjhttptest.NewRecorder(nil)

	return req, w
}

const domain = "http://example.com"

func TestInitCacheObject(t *testing.T) {
	monotonic.Init()
	const testKey = "testing"
	stream := Stream{
		Console:   coat.NewConsole(domain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
		OnSubscribe: func(key string) error {
			log.Println("sub", key)
			return nil
		},
		OnUnsubscribe: func(key string) {
			log.Println("unsub", key)
		},
	}

	req, w := makeStreamRequestMock(domain + "/" + testKey)

	wsConn, err := stream.New(testKey, w, req)
	require.NoError(t, err)
	require.Equal(t, 1, len(stream.pools))
	require.Equal(t, testKey, stream.pools[testKey].Key)
	require.Equal(t, 1, len(stream.pools[testKey].connections))

	obj := &meta.Object{
		Created: 1000,
		Index:   "test",
		Path:    testKey,
		Data:    json.RawMessage(`{"one": 2}`),
	}
	version := stream.InitCacheObjectWithVersion(testKey, obj)
	require.NotZero(t, version)

	cacheVersion, err := stream.GetCacheVersion(testKey)
	require.NoError(t, err)
	require.Equal(t, version, cacheVersion)

	stream.Close(testKey, wsConn)
	require.Equal(t, 1, len(stream.pools))
	require.Equal(t, testKey, stream.pools[testKey].Key)
	require.Equal(t, 0, len(stream.pools[testKey].connections))
}

func TestConcurrentBroadcast(t *testing.T) {
	monotonic.Init()
	var wg sync.WaitGroup

	stream := Stream{
		Console:   coat.NewConsole(domain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
		OnSubscribe: func(key string) error {
			// log.Println("sub", key)
			return nil
		},
		OnUnsubscribe: func(key string) {
			// log.Println("unsub", key)
		},
	}

	req, w := makeStreamRequestMock(domain + "/root")
	wsConn, err := stream.New("root", w, req)
	require.NoError(t, err)
	require.Equal(t, 1, len(stream.pools))
	require.Equal(t, "root", stream.pools["root"].Key)
	require.Equal(t, 1, len(stream.pools["root"].connections))

	reqA, wA := makeStreamRequestMock(domain + "/a")
	wsConnA, err := stream.New("a", wA, reqA)
	require.NoError(t, err)
	require.Equal(t, 2, len(stream.pools))
	require.Equal(t, "a", stream.pools["a"].Key)
	require.Equal(t, 1, len(stream.pools["a"].connections))

	reqB, wB := makeStreamRequestMock(domain + "/b")
	wsConnB, err := stream.New("b", wB, reqB)
	require.NoError(t, err)
	require.Equal(t, 3, len(stream.pools))
	require.Equal(t, "b", stream.pools["b"].Key)
	require.Equal(t, 1, len(stream.pools["b"].connections))

	// Initialize caches with objects
	objA := &meta.Object{Created: 1000, Index: "a", Path: "a", Data: json.RawMessage(`{"key":"a"}`)}
	objB := &meta.Object{Created: 1000, Index: "b", Path: "b", Data: json.RawMessage(`{"key":"b"}`)}
	stream.InitCacheObjectWithVersion("a", objA)
	stream.InitCacheObjectWithVersion("b", objB)

	// Concurrent broadcasts
	wg.Add(20)
	for range 10 {
		go func() {
			defer wg.Done()
			newObj := &meta.Object{Created: 1000, Updated: 2000, Index: "a", Path: "a", Data: json.RawMessage(`{"key":"a","updated":true}`)}
			stream.Broadcast("a", BroadcastOpt{
				Key:       "a",
				Operation: "set",
				Object:    newObj,
				FilterObject: func(key string, obj meta.Object) (meta.Object, error) {
					return obj, nil
				},
				FilterList: func(key string, objs []meta.Object) ([]meta.Object, error) {
					return objs, nil
				},
			})
		}()
	}

	for range 10 {
		go func() {
			defer wg.Done()
			newObj := &meta.Object{Created: 1000, Updated: 2000, Index: "b", Path: "b", Data: json.RawMessage(`{"key":"b","updated":true}`)}
			stream.Broadcast("b", BroadcastOpt{
				Key:       "b",
				Operation: "set",
				Object:    newObj,
				FilterObject: func(key string, obj meta.Object) (meta.Object, error) {
					return obj, nil
				},
				FilterList: func(key string, objs []meta.Object) ([]meta.Object, error) {
					return objs, nil
				},
			})
		}()
	}

	wg.Wait()

	stream.Close("root", wsConn)
	stream.Close("a", wsConnA)
	stream.Close("b", wsConnB)
	require.Equal(t, 3, len(stream.pools))
	require.Equal(t, 0, len(stream.pools["root"].connections))
	require.Equal(t, 0, len(stream.pools["a"].connections))
}

func TestRemoveConn(t *testing.T) {
	// Test removing from middle
	c1 := &Conn{}
	c2 := &Conn{}
	c3 := &Conn{}
	conns := []*Conn{c1, c2, c3}

	result := removeConn(conns, c2)
	require.Equal(t, 2, len(result))
	require.Contains(t, result, c1)
	require.Contains(t, result, c3)

	// Test removing from beginning
	conns = []*Conn{c1, c2, c3}
	result = removeConn(conns, c1)
	require.Equal(t, 2, len(result))
	require.Contains(t, result, c2)
	require.Contains(t, result, c3)

	// Test removing from end
	conns = []*Conn{c1, c2, c3}
	result = removeConn(conns, c3)
	require.Equal(t, 2, len(result))
	require.Contains(t, result, c1)
	require.Contains(t, result, c2)

	// Test removing non-existent
	conns = []*Conn{c1, c2}
	c4 := &Conn{}
	result = removeConn(conns, c4)
	require.Equal(t, 2, len(result))

	// Test removing from single element slice
	conns = []*Conn{c1}
	result = removeConn(conns, c1)
	require.Equal(t, 0, len(result))
}

func TestInitClock(t *testing.T) {
	stream := Stream{}
	stream.InitClock()

	require.NotNil(t, stream.pools)
	require.NotNil(t, stream.poolIndex)
	require.NotNil(t, stream.clockPool)
	require.Equal(t, DefaultWriteTimeout, stream.WriteTimeout)

	// Call again to ensure idempotency
	stream.InitClock()
	require.NotNil(t, stream.clockPool)
}

func TestPreallocatePools(t *testing.T) {
	stream := Stream{
		Console: coat.NewConsole(domain, true),
	}

	paths := []string{"items/*", "users/*", "config"}
	stream.PreallocatePools(paths)

	require.Equal(t, 3, len(stream.pools))
	require.NotNil(t, stream.pools["items/*"])
	require.NotNil(t, stream.pools["users/*"])
	require.NotNil(t, stream.pools["config"])

	// Verify pools have correct keys
	require.Equal(t, "items/*", stream.pools["items/*"].Key)
	require.Equal(t, "users/*", stream.pools["users/*"].Key)
	require.Equal(t, "config", stream.pools["config"].Key)

	// Call again with overlapping paths - should not duplicate
	stream.PreallocatePools([]string{"items/*", "newpath"})
	require.Equal(t, 4, len(stream.pools))
	require.NotNil(t, stream.pools["newpath"])
}

func TestGetState(t *testing.T) {
	monotonic.Init()
	stream := Stream{
		Console:   coat.NewConsole(domain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
		OnSubscribe: func(key string) error {
			return nil
		},
		OnUnsubscribe: func(key string) {},
	}
	stream.InitClock()

	// Empty state initially
	state := stream.GetState()
	require.Equal(t, 0, len(state))

	// Add a connection to a pool
	req, w := makeStreamRequestMock(domain + "/test")
	wsConn, err := stream.New("test", w, req)
	require.NoError(t, err)

	state = stream.GetState()
	require.Equal(t, 1, len(state))
	require.Equal(t, "test", state[0].Key)
	require.Equal(t, 1, state[0].Connections)

	// Add clock connection
	reqClock, wClock := makeStreamRequestMock(domain + "/")
	wsConnClock, err := stream.New("", wClock, reqClock)
	require.NoError(t, err)

	state = stream.GetState()
	require.Equal(t, 2, len(state))

	// Cleanup
	stream.Close("test", wsConn)
	stream.Close("", wsConnClock)
}

func TestBroadcastClock(t *testing.T) {
	monotonic.Init()
	stream := Stream{
		Console:       coat.NewConsole(domain, true),
		pools:         make(map[string]*Pool),
		poolIndex:     newPoolTrie(),
		WriteTimeout:  DefaultWriteTimeout,
		OnSubscribe:   func(key string) error { return nil },
		OnUnsubscribe: func(key string) {},
	}

	// BroadcastClock with nil clockPool should not panic
	stream.BroadcastClock("12345")

	// Initialize clock pool
	stream.InitClock()

	// BroadcastClock with empty connections should not panic
	stream.BroadcastClock("12345")

	// Add a clock connection
	req, w := makeStreamRequestMock(domain + "/")
	wsConn, err := stream.New("", w, req)
	require.NoError(t, err)
	require.Equal(t, 1, len(stream.clockPool.connections))

	// BroadcastClock with connection (will fail to write but shouldn't panic)
	stream.BroadcastClock("12345")

	stream.Close("", wsConn)
}

func TestInitCacheObjectsWithVersion(t *testing.T) {
	monotonic.Init()
	stream := Stream{
		Console:       coat.NewConsole(domain, true),
		pools:         make(map[string]*Pool),
		poolIndex:     newPoolTrie(),
		OnSubscribe:   func(key string) error { return nil },
		OnUnsubscribe: func(key string) {},
	}

	objects := []meta.Object{
		{Created: 1000, Index: "a", Path: "items/a", Data: json.RawMessage(`{"id":"a"}`)},
		{Created: 2000, Index: "b", Path: "items/b", Data: json.RawMessage(`{"id":"b"}`)},
	}

	// Initialize cache for new pool
	version := stream.InitCacheObjectsWithVersion("items/*", objects)
	require.NotZero(t, version)
	require.Equal(t, 1, len(stream.pools))
	require.Equal(t, 2, len(stream.pools["items/*"].cache.Objects))

	// Update existing pool cache
	newObjects := []meta.Object{
		{Created: 3000, Index: "c", Path: "items/c", Data: json.RawMessage(`{"id":"c"}`)},
	}
	version2 := stream.InitCacheObjectsWithVersion("items/*", newObjects)
	require.Equal(t, version, version2) // Version should remain the same
	require.Equal(t, 1, len(stream.pools["items/*"].cache.Objects))
}

func TestGetCacheVersionErrors(t *testing.T) {
	monotonic.Init()
	stream := Stream{
		Console:   coat.NewConsole(domain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
	}

	// Pool not found
	_, err := stream.GetCacheVersion("nonexistent")
	require.Error(t, err)
	require.Equal(t, ErrPoolNotFound, err)

	// Pool exists but cache is empty
	stream.pools["empty"] = &Pool{Key: "empty"}
	_, err = stream.GetCacheVersion("empty")
	require.Error(t, err)
	require.Equal(t, ErrPoolCacheEmpty, err)
}
