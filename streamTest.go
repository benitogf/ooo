package ooo

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"testing"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/benitogf/jsondiff"
	"github.com/benitogf/jsonpatch"
	"github.com/benitogf/ooo/client"
	"github.com/benitogf/ooo/filters"
	ooio "github.com/benitogf/ooo/io"
	"github.com/benitogf/ooo/messages"
	"github.com/benitogf/ooo/meta"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/pkg/expect"
	"github.com/stretchr/testify/require"
)

// remoteConfig creates a RemoteConfig for the given server
func remoteConfig(server *Server) ooio.RemoteConfig {
	return ooio.RemoteConfig{
		Client: &http.Client{},
		Host:   server.Address,
	}
}

// StreamBroadcastTest testing stream function
// Note: Uses raw websocket to verify patch protocol and meta.Object structure
func StreamBroadcastTest(t *testing.T, server *Server) {
	var wg sync.WaitGroup
	var postIndexResponse ooio.IndexResponse
	var wsObject meta.Object
	var wsEvent messages.Message
	var wsCache json.RawMessage
	cfg := remoteConfig(server)
	wsURL := url.URL{Scheme: "ws", Host: server.Address, Path: "/test"}
	wsClient, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	require.NoError(t, err)
	defer wsClient.Close()

	wg.Add(1)
	go func() {
		for {
			_, message, err := wsClient.ReadMessage()
			if err != nil {
				break
			}
			wsEvent, err = messages.DecodeBuffer(message)
			if err == nil {
				server.Console.Log("read wsClient", wsEvent.Data)
				wg.Done()
			}
		}
	}()
	wg.Wait()
	wg.Add(1)
	wsCache = wsEvent.Data

	server.Console.Log("post data")
	postIndexResponse, err = ooio.RemoteSetWithResponse(cfg, "test", TEST_DATA)
	require.NoError(t, err)
	wg.Wait()
	wg.Add(1)

	if !wsEvent.Snapshot {
		patch, err := jsonpatch.DecodePatch(wsEvent.Data)
		require.NoError(t, err)
		modified, err := patch.Apply([]byte(wsCache))
		require.NoError(t, err)
		err = json.Unmarshal(modified, &wsObject)
		require.NoError(t, err)
		wsCache = modified
	} else {
		err = json.Unmarshal(wsEvent.Data, &wsObject)
		require.NoError(t, err)
		wsCache = wsEvent.Data
	}

	require.Equal(t, wsObject.Index, postIndexResponse.Index)
	same, _ := jsondiff.Compare(wsObject.Data, TEST_DATA, &jsondiff.Options{})
	require.Equal(t, same, jsondiff.FullMatch)

	err = ooio.RemoteDelete(cfg, "test")
	require.NoError(t, err)
	wg.Wait()

	if !wsEvent.Snapshot {
		patch, err := jsonpatch.DecodePatch([]byte(wsEvent.Data))
		require.NoError(t, err)
		modified, err := patch.Apply([]byte(wsCache))
		require.NoError(t, err)
		err = json.Unmarshal(modified, &wsObject)
		require.NoError(t, err)
	} else {
		err = json.Unmarshal(wsEvent.Data, &wsObject)
		require.NoError(t, err)
	}

	require.Equal(t, wsObject.Created, int64(0))
}

// StreamItemGlobBroadcastTest testing stream function
// Note: Uses raw websocket to verify patch protocol and meta.Object structure
func StreamItemGlobBroadcastTest(t *testing.T, server *Server) {
	var wg sync.WaitGroup
	var postIndexResponse ooio.IndexResponse
	var wsObject meta.Object
	var wsEvent messages.Message
	var wsCache json.RawMessage
	cfg := remoteConfig(server)
	wsURL := url.URL{Scheme: "ws", Host: server.Address, Path: "/test/1"}
	wsClient, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	require.NoError(t, err)
	defer wsClient.Close()

	wg.Add(1)
	go func() {
		for {
			_, message, err := wsClient.ReadMessage()
			if err != nil {
				return
			}
			wsEvent, err = messages.DecodeBuffer(message)
			if err == nil {
				server.Console.Log("read wsClient", wsEvent.Data)
				wg.Done()
			}
		}
	}()

	// Wait for initial snapshot
	wg.Wait()
	wsCache = wsEvent.Data

	wg.Add(1)
	postIndexResponse, err = ooio.RemoteSetWithResponse(cfg, "test/1", TEST_DATA)
	require.NoError(t, err)
	wg.Wait()

	if !wsEvent.Snapshot {
		patch, err := jsonpatch.DecodePatch([]byte(wsEvent.Data))
		require.NoError(t, err)
		modified, err := patch.Apply([]byte(wsCache))
		require.NoError(t, err)
		err = json.Unmarshal(modified, &wsObject)
		require.NoError(t, err)
		wsCache = modified
	} else {
		err = json.Unmarshal(wsEvent.Data, &wsObject)
		require.NoError(t, err)
		wsCache = wsEvent.Data
	}

	require.Equal(t, wsObject.Index, postIndexResponse.Index)
	same, _ := jsondiff.Compare(wsObject.Data, TEST_DATA, &jsondiff.Options{})
	require.Equal(t, same, jsondiff.FullMatch)

	wg.Add(1)
	err = ooio.RemoteDelete(cfg, "test/*")
	require.NoError(t, err)
	wg.Wait()

	if !wsEvent.Snapshot {
		patch, err := jsonpatch.DecodePatch([]byte(wsEvent.Data))
		require.NoError(t, err)
		modified, err := patch.Apply([]byte(wsCache))
		require.NoError(t, err)
		err = json.Unmarshal(modified, &wsObject)
		require.NoError(t, err)
	} else {
		err = json.Unmarshal(wsEvent.Data, &wsObject)
		require.NoError(t, err)
	}

	require.Equal(t, int64(0), wsObject.Created)
}

// StreamGlobBroadcastTest testing stream function
// Note: This test uses raw websocket to verify patch/snapshot protocol behavior
func StreamGlobBroadcastTest(t *testing.T, server *Server, n int) {
	var wg sync.WaitGroup
	var indexResponse ooio.IndexResponse
	var wsObjects []meta.Object
	var wsEvent messages.Message
	var wsCache json.RawMessage
	cfg := remoteConfig(server)
	wsURL := url.URL{Scheme: "ws", Host: server.Address, Path: "/test/*"}
	wsClient, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	require.NoError(t, err)
	defer wsClient.Close()

	wg.Add(1)
	go func() {
		for {
			_, message, err := wsClient.ReadMessage()
			if err != nil {
				return
			}
			wsEvent, err = messages.DecodeBuffer(message)
			if err == nil {
				wg.Done()
			}
		}
	}()

	// Wait for initial snapshot
	wg.Wait()
	wsCache = wsEvent.Data

	server.Console.Log("post data")
	keys := []string{}
	for range n {
		wg.Add(1)
		indexResponse, err = ooio.RemotePushWithResponse(cfg, "test/*", TEST_DATA)
		require.NoError(t, err)
		wg.Wait()
		require.False(t, wsEvent.Snapshot)
		patch, err := jsonpatch.DecodePatch([]byte(wsEvent.Data))
		require.NoError(t, err)
		modified, err := patch.Apply([]byte(wsCache))
		require.NoError(t, err)
		err = json.Unmarshal(modified, &wsObjects)
		require.NoError(t, err)
		wsCache = modified
		keys = append(keys, indexResponse.Index)
	}

	require.Equal(t, wsObjects[len(wsObjects)-1].Index, indexResponse.Index)
	same, _ := jsondiff.Compare(wsObjects[len(wsObjects)-1].Data, TEST_DATA, &jsondiff.Options{})
	require.Equal(t, same, jsondiff.FullMatch)

	server.Console.Log("post update data")
	nextGet := float64(0)
	Q := 3
	for i := range Q {
		for _, key := range keys {
			found := meta.Object{}
			for _, obj := range wsObjects {
				if obj.Index == key {
					found = obj
					break
				}
			}
			testData := found.Data
			currentRaw := gjson.Get(string(testData), "search_metadata.count")
			current := currentRaw.Value().(float64)
			nextSet := current + 1
			server.Console.Log("post", key, i, nextSet)
			newData, err := sjson.Set(string(testData), "search_metadata.count", nextSet)
			require.NoError(t, err)
			wg.Add(1)
			err = ooio.RemoteSet(cfg, "test/"+key, json.RawMessage(newData))
			require.NoError(t, err)
			wg.Wait()
			require.False(t, wsEvent.Snapshot)
			patch, err := jsonpatch.DecodePatch([]byte(wsEvent.Data))
			require.NoError(t, err)
			modified, err := patch.Apply([]byte(wsCache))
			require.NoError(t, err)
			err = json.Unmarshal(modified, &wsObjects)
			require.NoError(t, err)
			found = meta.Object{}
			for _, obj := range wsObjects {
				if obj.Index == key {
					found = obj
					break
				}
			}
			require.NotEmpty(t, found.Index)
			nextRaw := gjson.Get(string(found.Data), "search_metadata.count")
			nextGet = nextRaw.Value().(float64)
			require.Equal(t, nextSet, nextGet)
			wsCache = modified
		}
	}

	require.Equal(t, float64(4+Q), nextGet)

	// Delete all items - glob delete sends a single broadcast with empty list
	wg.Add(1)
	err = ooio.RemoteDelete(cfg, "test/*")
	require.NoError(t, err)
	wg.Wait()
	// After glob delete, the broadcast contains an empty list snapshot
	err = json.Unmarshal(wsEvent.Data, &wsObjects)
	if err != nil {
		wsObjects = []meta.Object{}
	}

	// Verify storage is empty
	stored, err := server.Storage.GetList("test/*")
	require.NoError(t, err)
	require.Equal(t, 0, len(stored))
}

// StreamBroadcastFilterTest testing stream function
// Note: This test uses raw websocket to verify snapshot/patch protocol behavior
func StreamBroadcastFilterTest(t *testing.T, server *Server) {
	var wg sync.WaitGroup
	var wsExtraEvent messages.Message
	var callCount int
	// extra filter - returns modified list data that changes each call
	server.ReadListFilter("test/*", func(key string, objs []meta.Object) ([]meta.Object, error) {
		callCount++
		// Return data that changes each time to ensure broadcasts aren't skipped
		return []meta.Object{{Data: []byte(`{"extra": "` + strconv.Itoa(callCount) + `"}`)}}, nil
	})
	// extra route
	server.Router = mux.NewRouter()
	server.Start("localhost:0")
	server.Storage.Clear()
	cfg := remoteConfig(server)
	wsExtraURL := url.URL{Scheme: "ws", Host: server.Address, Path: "/test/*"}
	wsExtraClient, _, err := websocket.DefaultDialer.Dial(wsExtraURL.String(), nil)
	require.NoError(t, err)
	defer wsExtraClient.Close()

	wg.Add(1)
	go func() {
		for {
			_, message, err := wsExtraClient.ReadMessage()
			if err != nil {
				break
			}
			wsExtraEvent, err = messages.DecodeBuffer(message)
			expect.Nil(err)
			server.Console.Log("read wsClient", string(message))
			wg.Done()
		}
	}()
	wg.Wait()
	// First message should be a snapshot with the filtered data
	require.Equal(t, true, wsExtraEvent.Snapshot)
	require.Contains(t, string(wsExtraEvent.Data), `"extra"`)

	wg.Add(1)
	server.Console.Log("post data")
	err = ooio.RemotePush(cfg, "test/*", TEST_DATA)
	require.NoError(t, err)
	wg.Wait()

	// Second message should be a patch (not snapshot) since data changed
	require.Equal(t, false, wsExtraEvent.Snapshot)
}

// StreamBroadcastForcePatchTest testing stream function
// Note: This test uses raw websocket to verify ForcePatch behavior (no snapshots after first)
func StreamBroadcastForcePatchTest(t *testing.T, server *Server) {
	var wg sync.WaitGroup
	var wsExtraEvent messages.Message
	// extra route
	server.Router = mux.NewRouter()
	server.ForcePatch = true
	server.Start("localhost:0")
	server.Storage.Clear()
	cfg := remoteConfig(server)
	wsExtraURL := url.URL{Scheme: "ws", Host: server.Address, Path: "/test/*"}
	wsExtraClient, _, err := websocket.DefaultDialer.Dial(wsExtraURL.String(), nil)
	require.NoError(t, err)
	defer wsExtraClient.Close()

	firstMessage := true
	wg.Add(1)
	go func() {
		for {
			_, message, err := wsExtraClient.ReadMessage()
			if err != nil {
				break
			}
			wsExtraEvent, err = messages.DecodeBuffer(message)
			expect.Nil(err)
			if !firstMessage {
				expect.True(!wsExtraEvent.Snapshot)
			}
			firstMessage = false
			server.Console.Log("read wsClient", string(message))
			wg.Done()
		}
	}()
	wg.Wait()
	wg.Add(1)
	server.Console.Log("post data")
	err = ooio.RemotePush(cfg, "test/*", TEST_DATA)
	require.NoError(t, err)
	wg.Wait()

	wg.Add(1)
	server.Console.Log("post data")
	err = ooio.RemotePush(cfg, "test/*", TEST_DATA)
	require.NoError(t, err)
	wg.Wait()

	wg.Add(1)
	server.Console.Log("post data")
	err = ooio.RemotePush(cfg, "test/*", TEST_DATA)
	require.NoError(t, err)
	wg.Wait()

	wg.Add(1)
	server.Console.Log("post data")
	err = ooio.RemotePush(cfg, "test/*", TEST_DATA)
	require.NoError(t, err)
	wg.Wait()
}

// StreamBroadcastNoPatchTest testing stream function
// Note: This test uses raw websocket to verify NoPatch behavior (all messages are snapshots)
func StreamBroadcastNoPatchTest(t *testing.T, server *Server) {
	var wg sync.WaitGroup
	var wsExtraEvent messages.Message
	// extra route
	server.Router = mux.NewRouter()
	server.NoPatch = true
	server.Start("localhost:0")
	server.Storage.Clear()
	cfg := remoteConfig(server)
	wsExtraURL := url.URL{Scheme: "ws", Host: server.Address, Path: "/test/*"}
	wsExtraClient, _, err := websocket.DefaultDialer.Dial(wsExtraURL.String(), nil)
	require.NoError(t, err)
	defer wsExtraClient.Close()

	wg.Add(1)
	go func() {
		for {
			_, message, err := wsExtraClient.ReadMessage()
			if err != nil {
				break
			}
			wsExtraEvent, err = messages.DecodeBuffer(message)
			expect.Nil(err)
			expect.True(wsExtraEvent.Snapshot)
			server.Console.Log("read wsClient", string(message))
			wg.Done()
		}
	}()
	wg.Wait()
	wg.Add(1)
	server.Console.Log("post data")
	err = ooio.RemotePush(cfg, "test/*", TEST_DATA)
	require.NoError(t, err)
	wg.Wait()

	wg.Add(1)
	server.Console.Log("post data")
	err = ooio.RemotePush(cfg, "test/*", TEST_DATA)
	require.NoError(t, err)
	wg.Wait()
}

func StreamGlobBroadcastConcurrentTest(t *testing.T, server *Server, n int) {
	type TestData struct {
		SearchMetadata struct {
			Count float64 `json:"count"`
		} `json:"search_metadata"`
	}
	var wg sync.WaitGroup

	entries := []client.Meta[TestData]{}
	var entriesLock sync.Mutex

	wg.Add(1)
	go client.SubscribeList(client.SubscribeConfig{
		Ctx:     t.Context(),
		Server:  client.Server{Protocol: "ws", Host: server.Address},
		Silence: true,
	}, "/test/*", client.SubscribeListEvents[TestData]{
		OnMessage: func(m []client.Meta[TestData]) {
			entriesLock.Lock()
			entries = m
			// log.Println("CLIENT", len(m))
			wg.Done()
			entriesLock.Unlock()
		},
	})

	wg.Wait() // first read
	entriesLock.Lock()
	require.Zero(t, len(entries))
	entriesLock.Unlock()

	keys := []string{}
	for i := range n {
		wg.Add(1)
		// _key := key.Build("test/*")
		_key := "test/" + strconv.Itoa(i)
		_, err := server.Storage.Set(_key, TEST_DATA)
		require.NoError(t, err)
		keys = append(keys, _key)
	}

	wg.Wait() // created

	entriesLock.Lock()
	require.Equal(t, len(keys), len(entries))
	require.Equal(t, float64(4), entries[0].Data.SearchMetadata.Count)
	entriesLock.Unlock()

	Q := 3
	for _, _key := range keys {
		wg.Add(Q)
		go func(__key string) {
			for range Q {
				currentObj, err := server.Storage.GetAndLock(__key)
				expect.Nil(err)
				currentRaw := gjson.Get(string(currentObj.Data), "search_metadata.count")
				current := currentRaw.Value().(float64)
				nextSet := current + 1
				// log.Println("up1", __key, current, nextSet)
				newData, err := sjson.Set(string(currentObj.Data), "search_metadata.count", nextSet)
				expect.Nil(err)
				_, err = server.Storage.SetAndUnlock(__key, json.RawMessage(newData))
				expect.Nil(err)
			}
		}(_key)
	}

	for _, _key := range keys {
		wg.Add(Q)
		go func(__key string) {
			for range Q {
				currentObj, err := server.Storage.GetAndLock(__key)
				expect.Nil(err)
				currentRaw := gjson.Get(string(currentObj.Data), "search_metadata.something")
				current := currentRaw.Value().(string)
				nextSet := "popo"
				if current == "popo" {
					nextSet = "nopo"
				}
				// log.Println("up2", __key, current, nextSet)
				newData, err := sjson.Set(string(currentObj.Data), "search_metadata.something", nextSet)
				expect.Nil(err)
				_, err = server.Storage.SetAndUnlock(__key, json.RawMessage(newData))
				expect.Nil(err)
			}
		}(_key)
	}

	wg.Wait() // updated
	entriesLock.Lock()
	require.Equal(t, len(keys), len(entries))
	for _, obj := range entries {
		require.Equal(t, float64(4+Q), obj.Data.SearchMetadata.Count)
	}
	entriesLock.Unlock()
}

func StreamBroadcastPatchTest(t *testing.T, server *Server) {
	var wg sync.WaitGroup
	type TestSubField struct {
		One   string `json:"one"`
		Two   string `json:"two"`
		Three int    `json:"three"`
	}
	type TestData struct {
		SubFields []TestSubField `json:"subFields"`
	}

	current := TestData{}

	wg.Add(1)
	go client.Subscribe(client.SubscribeConfig{
		Ctx:     t.Context(),
		Server:  client.Server{Protocol: "ws", Host: server.Address},
		Silence: true,
	}, "test", client.SubscribeEvents[TestData]{
		OnMessage: func(m client.Meta[TestData]) {
			current = m.Data
			wg.Done()
		},
	})
	wg.Wait() // first read

	wg.Add(1)
	_td := TestData{
		SubFields: []TestSubField{
			{
				One:   "one",
				Two:   "two",
				Three: 3,
			},
		},
	}
	td, err := json.Marshal(_td)
	require.NoError(t, err)
	server.Storage.Set("test", td)
	wg.Wait()

	require.Equal(t, 1, len(current.SubFields))

	wg.Add(1)
	_td = TestData{
		SubFields: []TestSubField{
			{
				One:   "one",
				Two:   "two",
				Three: 3,
			},
			{
				One:   "one",
				Two:   "two",
				Three: 3,
			},
		},
	}
	td, err = json.Marshal(_td)
	require.NoError(t, err)
	server.Storage.Set("test", td)
	wg.Wait()

	require.Equal(t, 2, len(current.SubFields))
}

// StreamLimitFilterTest tests that the LimitFilter correctly maintains the limit
// when items are inserted and broadcast to subscribed clients.
// The client should never see more than the limit number of items due to ReadListFilter.
// Note: This test uses raw websocket to verify patch protocol and limit enforcement
func StreamLimitFilterTest(t *testing.T, server *Server) {
	var wg sync.WaitGroup
	var wsObjects []meta.Object
	var wsEvent messages.Message
	var wsCache json.RawMessage

	const limit = 3
	const totalInserts = limit + 5 // Insert more than the limit

	// Set up limit filter - uses ReadListFilter to limit view + AfterWrite to cleanup
	server.LimitFilter("limited/*", filters.LimitFilterConfig{Limit: limit})
	cfg := remoteConfig(server)

	// Subscribe using raw websocket
	wsURL := url.URL{Scheme: "ws", Host: server.Address, Path: "/limited/*"}
	wsClient, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	require.NoError(t, err)
	defer wsClient.Close()

	wg.Add(1)
	go func() {
		for {
			_, message, err := wsClient.ReadMessage()
			if err != nil {
				return
			}
			wsEvent, err = messages.DecodeBuffer(message)
			if err == nil {
				wg.Done()
			}
		}
	}()

	// Wait for initial snapshot
	wg.Wait()
	require.True(t, wsEvent.Snapshot)
	wsCache = wsEvent.Data
	err = json.Unmarshal(wsEvent.Data, &wsObjects)
	require.NoError(t, err)
	// t.Logf("Initial snapshot: %d items", len(wsObjects))

	// Insert items via HTTP
	// Each insert triggers exactly 1 broadcast:
	// - When under limit: "add" broadcast
	// - When at/over limit: The ReadListFilter limits the view, so the broadcast
	//   should handle both adding the new item and removing the pushed-out item
	for i := range totalInserts {
		wg.Add(1)
		err := ooio.RemotePush(cfg, "limited/*", map[string]int{"value": i})
		require.NoError(t, err)
		wg.Wait()

		// Apply patch to cache
		require.False(t, wsEvent.Snapshot, "expected patch, got snapshot for item %d", i)
		patch, patchErr := jsonpatch.DecodePatch(wsEvent.Data)
		require.NoError(t, patchErr, "failed to decode patch for item %d: %s", i, string(wsEvent.Data))
		modified, applyErr := patch.Apply(wsCache)
		require.NoError(t, applyErr, "failed to apply patch for item %d", i)
		wsCache = modified

		err = json.Unmarshal(wsCache, &wsObjects)
		require.NoError(t, err)
		// t.Logf("After insert %d: %d items, patch: %s", i, len(wsObjects), string(wsEvent.Data))

		// Client should never see more than limit items
		require.LessOrEqual(t, len(wsObjects), limit, "client should never see more than limit items after insert %d", i)
	}

	// Verify storage has exactly 'limit' items after cleanup
	stored, err := server.Storage.GetList("limited/*")
	require.NoError(t, err)
	require.Equal(t, limit, len(stored), "storage should have exactly limit items")
}

// ClientCompatibilityTest covers the key behaviors expected by ooo-client:
// 1. Object lifecycle: create (updated=0) → update (updated>0) → delete (empty object)
// 2. List lifecycle: create → update → delete single item
// 3. Glob delete: multiple items deleted with single broadcast returning empty list
// 4. List sort order: newest first (descending by created)
// 5. Nested paths: box/*/things/* pattern matching
func ClientCompatibilityTest(t *testing.T, server *Server) {
	cfg := remoteConfig(server)

	// Test 1: Object lifecycle - verify updated field behavior
	// Note: Uses raw websocket to verify patch protocol behavior
	t.Run("ObjectLifecycle", func(t *testing.T) {
		var wg sync.WaitGroup
		var wsEvent messages.Message
		wsURL := url.URL{Scheme: "ws", Host: server.Address, Path: "/objtest"}
		wsClient, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
		require.NoError(t, err)
		defer wsClient.Close()

		wg.Add(1)
		go func() {
			for {
				_, message, err := wsClient.ReadMessage()
				if err != nil {
					return
				}
				wsEvent, _ = messages.DecodeBuffer(message)
				wg.Done()
			}
		}()

		// Initial empty object
		wg.Wait()
		var obj meta.Object
		err = json.Unmarshal(wsEvent.Data, &obj)
		require.NoError(t, err)
		require.Equal(t, int64(0), obj.Created, "initial object should have created=0")
		require.Equal(t, int64(0), obj.Updated, "initial object should have updated=0")
		require.Equal(t, json.RawMessage("{}"), obj.Data, "initial object should have data={}")

		// Create object
		wg.Add(1)
		err = ooio.RemoteSet(cfg, "objtest", map[string]string{"name": "test1"})
		require.NoError(t, err)
		wg.Wait()
		err = json.Unmarshal(wsEvent.Data, &obj)
		if !wsEvent.Snapshot {
			patch, _ := jsonpatch.DecodePatch(wsEvent.Data)
			modified, _ := patch.Apply([]byte(`{"created":0,"updated":0,"index":"","data":{}}`))
			json.Unmarshal(modified, &obj)
		}
		require.Greater(t, obj.Created, int64(0), "created object should have created>0")
		require.Equal(t, int64(0), obj.Updated, "newly created object should have updated=0")

		// Update object
		wg.Add(1)
		err = ooio.RemoteSet(cfg, "objtest", map[string]string{"name": "test2"})
		require.NoError(t, err)
		wg.Wait()
		var updatedObj meta.Object
		if wsEvent.Snapshot {
			json.Unmarshal(wsEvent.Data, &updatedObj)
		} else {
			objBytes, _ := json.Marshal(obj)
			patch, _ := jsonpatch.DecodePatch(wsEvent.Data)
			modified, _ := patch.Apply(objBytes)
			json.Unmarshal(modified, &updatedObj)
		}
		require.Greater(t, updatedObj.Updated, int64(0), "updated object should have updated>0")

		// Delete object
		wg.Add(1)
		err = ooio.RemoteDelete(cfg, "objtest")
		require.NoError(t, err)
		wg.Wait()
		var deletedObj meta.Object
		if wsEvent.Snapshot {
			json.Unmarshal(wsEvent.Data, &deletedObj)
		} else {
			objBytes, _ := json.Marshal(updatedObj)
			patch, _ := jsonpatch.DecodePatch(wsEvent.Data)
			modified, _ := patch.Apply(objBytes)
			json.Unmarshal(modified, &deletedObj)
		}
		require.Equal(t, int64(0), deletedObj.Created, "deleted object should have created=0")
		require.Equal(t, json.RawMessage("{}"), deletedObj.Data, "deleted object should have data={}")
	})

	// Test 2: List lifecycle with single item
	// Note: Uses raw websocket to verify patch protocol behavior
	t.Run("ListLifecycle", func(t *testing.T) {
		var wg sync.WaitGroup
		var wsEvent messages.Message
		var wsCache json.RawMessage
		wsURL := url.URL{Scheme: "ws", Host: server.Address, Path: "/items/*"}
		wsClient, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
		require.NoError(t, err)
		defer wsClient.Close()

		wg.Add(1)
		go func() {
			for {
				_, message, err := wsClient.ReadMessage()
				if err != nil {
					return
				}
				wsEvent, _ = messages.DecodeBuffer(message)
				wg.Done()
			}
		}()

		// Initial empty list
		wg.Wait()
		wsCache = wsEvent.Data
		var items []meta.Object
		err = json.Unmarshal(wsEvent.Data, &items)
		require.NoError(t, err)
		require.Equal(t, 0, len(items), "initial list should be empty")

		// Create item
		wg.Add(1)
		indexResp, err := ooio.RemotePushWithResponse(cfg, "items/*", map[string]string{"name": "item1"})
		require.NoError(t, err)
		wg.Wait()
		if !wsEvent.Snapshot {
			patch, _ := jsonpatch.DecodePatch(wsEvent.Data)
			modified, _ := patch.Apply(wsCache)
			wsCache = modified
		} else {
			wsCache = wsEvent.Data
		}
		json.Unmarshal(wsCache, &items)
		require.Equal(t, 1, len(items), "list should have 1 item after create")
		require.Equal(t, int64(0), items[0].Updated, "newly created item should have updated=0")

		// Update item
		wg.Add(1)
		err = ooio.RemoteSet(cfg, "items/"+indexResp.Index, map[string]string{"name": "item1-updated"})
		require.NoError(t, err)
		wg.Wait()
		if !wsEvent.Snapshot {
			patch, _ := jsonpatch.DecodePatch(wsEvent.Data)
			modified, _ := patch.Apply(wsCache)
			wsCache = modified
		} else {
			wsCache = wsEvent.Data
		}
		json.Unmarshal(wsCache, &items)
		require.Equal(t, 1, len(items), "list should still have 1 item after update")
		require.Greater(t, items[0].Updated, int64(0), "updated item should have updated>0")

		// Delete item
		wg.Add(1)
		err = ooio.RemoteDelete(cfg, "items/"+indexResp.Index)
		require.NoError(t, err)
		wg.Wait()
		if !wsEvent.Snapshot {
			patch, _ := jsonpatch.DecodePatch(wsEvent.Data)
			modified, _ := patch.Apply(wsCache)
			wsCache = modified
		} else {
			wsCache = wsEvent.Data
		}
		json.Unmarshal(wsCache, &items)
		require.Equal(t, 0, len(items), "list should be empty after delete")
	})

	// Test 3: Glob delete - multiple items deleted with single broadcast
	// Note: Uses raw websocket to verify message count behavior
	t.Run("GlobDelete", func(t *testing.T) {
		var wg sync.WaitGroup
		var wsEvent messages.Message
		var msgCount int
		wsURL := url.URL{Scheme: "ws", Host: server.Address, Path: "/things/*"}
		wsClient, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
		require.NoError(t, err)
		defer wsClient.Close()

		go func() {
			for {
				_, message, err := wsClient.ReadMessage()
				if err != nil {
					return
				}
				wsEvent, _ = messages.DecodeBuffer(message)
				msgCount++
				wg.Done()
			}
		}()

		// Initial empty list
		wg.Add(1)
		wg.Wait()

		// Create 5 items
		numItems := 5
		for i := range numItems {
			wg.Add(1)
			_, err := ooio.RemotePushWithResponse(cfg, "things/*", map[string]int{"value": i})
			require.NoError(t, err)
			wg.Wait()
		}

		// Glob delete - should receive exactly 1 broadcast (not 5)
		msgCountBefore := msgCount
		wg.Add(1)
		err = ooio.RemoteDelete(cfg, "things/*")
		require.NoError(t, err)
		wg.Wait()

		// Verify only 1 message received for glob delete
		require.Equal(t, msgCountBefore+1, msgCount, "glob delete should send exactly 1 broadcast")

		// Verify the broadcast contains empty list
		var items []meta.Object
		err = json.Unmarshal(wsEvent.Data, &items)
		require.NoError(t, err)
		require.Equal(t, 0, len(items), "glob delete broadcast should contain empty list")

		// Verify storage is empty
		stored, err := server.Storage.GetList("things/*")
		require.NoError(t, err)
		require.Equal(t, 0, len(stored), "storage should be empty after glob delete")
	})

	// Test 4: List sort order - newest first (descending)
	t.Run("ListSortOrder", func(t *testing.T) {
		// Create items with different timestamps
		_, err := ooio.RemotePushWithResponse(cfg, "sorted/*", map[string]string{"name": "first"})
		require.NoError(t, err)
		// Small delay to ensure different timestamps
		_, err = ooio.RemotePushWithResponse(cfg, "sorted/*", map[string]string{"name": "second"})
		require.NoError(t, err)

		// Get list via HTTP - should be sorted newest first
		items, err := ooio.RemoteGetList[map[string]string](cfg, "sorted/*")
		require.NoError(t, err)
		require.Equal(t, 2, len(items))
		require.Equal(t, "second", items[0].Data["name"], "newest item should be first")
		require.Equal(t, "first", items[1].Data["name"], "oldest item should be last")

		// Cleanup
		ooio.RemoteDelete(cfg, "sorted/*")
	})

	// Test 5: Nested paths - box/*/things/* pattern
	t.Run("NestedPaths", func(t *testing.T) {
		// Create items with nested paths
		err := ooio.RemoteSet(cfg, "box/1/things/1", map[string]string{"name": "thing in box 1"})
		require.NoError(t, err)
		err = ooio.RemoteSet(cfg, "box/2/things/0", map[string]string{"name": "thing in box 2"})
		require.NoError(t, err)

		// Get list with nested glob pattern
		items, err := ooio.RemoteGetList[map[string]string](cfg, "box/*/things/*")
		require.NoError(t, err)
		require.Equal(t, 2, len(items), "should find 2 items matching nested pattern")

		// Newest item should be first (box/2/things/0 was created second)
		require.Equal(t, "thing in box 2", items[0].Data["name"], "newest nested item should be first")

		// Get specific nested item
		item, err := ooio.RemoteGet[map[string]string](cfg, "box/2/things/0")
		require.NoError(t, err)
		require.Equal(t, "thing in box 2", item.Data["name"], "should get correct nested item")

		// Cleanup
		ooio.RemoteDelete(cfg, "box/1/things/1")
		ooio.RemoteDelete(cfg, "box/2/things/0")
	})
}
