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
	const testKey = "testing"
	stream := Stream{
		Console:   coat.NewConsole(domain, false),
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

func TestGenerateListPatch(t *testing.T) {
	// Test add patch
	obj := meta.Object{
		Created: 1000,
		Index:   "new",
		Path:    "test/new",
		Data:    json.RawMessage(`{"four":4}`),
	}
	patch, err := generateListPatch("add", 3, &obj)
	require.NoError(t, err)
	require.Contains(t, string(patch), `"op":"add"`)
	require.Contains(t, string(patch), `"path":"/3"`)

	// Test replace patch
	patch, err = generateListPatch("replace", 2, &obj)
	require.NoError(t, err)
	require.Contains(t, string(patch), `"op":"replace"`)
	require.Contains(t, string(patch), `"path":"/2"`)

	// Test remove patch
	patch, err = generateListPatch("remove", 1, nil)
	require.NoError(t, err)
	require.Contains(t, string(patch), `"op":"remove"`)
	require.Contains(t, string(patch), `"path":"/1"`)
}

func TestGenerateObjectPatch(t *testing.T) {
	oldObj := &meta.Object{
		Created: 1000,
		Updated: 1000,
		Index:   "test",
		Path:    "test/1",
		Data:    json.RawMessage(`{"name":"Alice","status":"active"}`),
	}
	newObj := &meta.Object{
		Created: 1000,
		Updated: 2000,
		Index:   "test",
		Path:    "test/1",
		Data:    json.RawMessage(`{"name":"Alice","status":"inactive"}`),
	}

	patch, snapshot, err := generateObjectPatch(oldObj, newObj)
	require.NoError(t, err)
	require.False(t, snapshot)
	require.Contains(t, string(patch), `"op":"replace"`)
	require.Contains(t, string(patch), `/data/status`)
}

func TestConcurrentBroadcast(t *testing.T) {
	var wg sync.WaitGroup

	stream := Stream{
		Console:   coat.NewConsole(domain, false),
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
	for i := 0; i < 10; i++ {
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

	for y := 0; y < 10; y++ {
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

func BenchmarkRemoveConn(b *testing.B) {
	// Create a slice with 100 connections
	conns := make([]*Conn, 100)
	for i := range conns {
		conns[i] = &Conn{}
	}
	target := conns[50] // Remove from middle

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Make a copy to avoid modifying the original
		testConns := make([]*Conn, len(conns))
		copy(testConns, conns)
		_ = removeConn(testConns, target)
	}
}
