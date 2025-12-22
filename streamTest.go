package ooo

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"testing"

	"github.com/goccy/go-json"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/benitogf/jsondiff"
	"github.com/benitogf/jsonpatch"
	"github.com/benitogf/ooo/client"
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
	wg.Add(1)
	go func() {
		for {
			_, message, err := wsClient.ReadMessage()
			if err != nil {
				break
			}
			wsEvent, err = messages.DecodeBuffer(message)
			expect.Nil(err)
			server.Console.Log("read wsClient", wsEvent.Data)
			wg.Done()
		}
	}()
	wg.Wait()
	wg.Add(1)
	wsCache = wsEvent.Data
	wsVersion, err := strconv.ParseInt(wsEvent.Version, 16, 64)
	require.NoError(t, err)
	streamCacheVersion, err := server.Stream.GetCacheVersion("test")
	require.NoError(t, err)
	server.Console.Log("post data")
	postIndexResponse, err = ooio.RemoteSetWithResponse(cfg, "test", TEST_DATA)
	require.NoError(t, err)
	require.Equal(t, wsVersion, streamCacheVersion)
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

	wsClient.Close()

	require.Equal(t, wsObject.Created, int64(0))
}

// StreamItemGlobBroadcastTest testing stream function
func StreamItemGlobBroadcastTest(t *testing.T, server *Server) {
	var wg sync.WaitGroup
	// this lock should not be neccesary but the race detector doesnt recognize the wait group preventing the race here
	var lk sync.Mutex
	var postIndexResponse ooio.IndexResponse
	var wsObject meta.Object
	var wsEvent messages.Message
	var wsCache json.RawMessage
	cfg := remoteConfig(server)
	wsURL := url.URL{Scheme: "ws", Host: server.Address, Path: "/test/1"}
	wsClient, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	require.NoError(t, err)
	wg.Add(1)
	go func() {
		for {
			_, message, err := wsClient.ReadMessage()
			if err != nil {
				break
			}
			lk.Lock()
			wsEvent, err = messages.DecodeBuffer(message)
			lk.Unlock()
			expect.Nil(err)
			server.Console.Log("read wsClient", wsEvent.Data)
			wg.Done()
		}
	}()
	wg.Wait()
	wg.Add(1)
	lk.Lock()
	wsCache = wsEvent.Data
	lk.Unlock()
	postIndexResponse, err = ooio.RemoteSetWithResponse(cfg, "test/1", TEST_DATA)
	require.NoError(t, err)
	wg.Wait()
	patch, err := jsonpatch.DecodePatch([]byte(wsEvent.Data))
	require.NoError(t, err)
	modified, err := patch.Apply([]byte(wsCache))
	require.NoError(t, err)
	err = json.Unmarshal(modified, &wsObject)
	require.NoError(t, err)
	wsCache = modified

	require.Equal(t, wsObject.Index, postIndexResponse.Index)
	same, _ := jsondiff.Compare(wsObject.Data, TEST_DATA, &jsondiff.Options{})
	require.Equal(t, same, jsondiff.FullMatch)

	wg.Add(1)
	err = ooio.RemoteDelete(cfg, "test/*")
	require.NoError(t, err)
	wg.Wait()

	patch, err = jsonpatch.DecodePatch([]byte(wsEvent.Data))
	require.NoError(t, err)
	modified, err = patch.Apply([]byte(wsCache))
	require.NoError(t, err)
	err = json.Unmarshal(modified, &wsObject)
	require.NoError(t, err)

	wsClient.Close()

	require.Equal(t, int64(0), wsObject.Created)
}

// StreamGlobBroadcastTest testing stream function
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
	wg.Add(1)
	go func() {
		for {
			_, message, err := wsClient.ReadMessage()
			if err != nil {
				break
			}
			wsEvent, err = messages.DecodeBuffer(message)
			expect.Nil(err)
			wg.Done()
		}
	}()
	wg.Wait()

	wsCache = wsEvent.Data
	server.Console.Log("post data")
	keys := []string{}
	for i := 0; i < n; i++ {
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
			wg.Add(1)
			found := meta.Object{}
			for _, obj := range wsObjects {
				if obj.Index == key {
					found = obj
					break
				}
			}
			testData := found.Data
			require.NoError(t, err)
			currentRaw := gjson.Get(string(testData), "search_metadata.count")
			current := currentRaw.Value().(float64)
			nextSet := current + 1
			server.Console.Log("post", key, i, nextSet)
			newData, err := sjson.Set(string(testData), "search_metadata.count", nextSet)
			require.NoError(t, err)
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

	wg.Add(1)
	err = ooio.RemoteDelete(cfg, "test/*")
	require.NoError(t, err)
	wg.Wait()

	require.False(t, wsEvent.Snapshot)
	patch, err := jsonpatch.DecodePatch([]byte(wsEvent.Data))
	require.NoError(t, err)
	modified, err := patch.Apply([]byte(wsCache))
	require.NoError(t, err)
	err = json.Unmarshal(modified, &wsObjects)
	require.NoError(t, err)

	wsClient.Close()

	require.Equal(t, len(wsObjects), 0)
}

// StreamBroadcastFilterTest testing stream function
func StreamBroadcastFilterTest(t *testing.T, server *Server) {
	var wg sync.WaitGroup
	var wsExtraEvent messages.Message
	// extra filter - returns modified list data
	server.ReadListFilter("test/*", func(key string, objs []meta.Object) ([]meta.Object, error) {
		// Return a single object with extra data
		return []meta.Object{{Data: []byte(`{"extra": "extra"}`)}}, nil
	})
	// extra route
	server.Router = mux.NewRouter()
	server.Start("localhost:0")
	server.Storage.Clear()
	cfg := remoteConfig(server)
	wsExtraURL := url.URL{Scheme: "ws", Host: server.Address, Path: "/test/*"}
	wsExtraClient, _, err := websocket.DefaultDialer.Dial(wsExtraURL.String(), nil)
	require.NoError(t, err)
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
	wg.Add(1)
	server.Console.Log("post data")
	err = ooio.RemotePush(cfg, "test/*", TEST_DATA)
	require.NoError(t, err)
	wg.Wait()
	wsExtraClient.Close()

	// empty operations for a broadcast with no changes
	require.Equal(t, false, wsExtraEvent.Snapshot)
	require.Equal(t, "[]", string(wsExtraEvent.Data))
}

// StreamBroadcastForcePatchTest testing stream function
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

	wsExtraClient.Close()
}

// StreamBroadcastNoPatchTest testing stream function
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

	wsExtraClient.Close()
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
		Ctx:      context.Background(),
		Protocol: "ws",
		Host:     server.Address,
		Silence:  true,
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

	log.Println("push data")
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

	log.Println("post update data", len(keys))
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
				log.Println("up1", __key, current, nextSet)
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

	log.Println("wait update data")
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
		Ctx:      context.Background(),
		Protocol: "ws",
		Host:     server.Address,
		Silence:  true,
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
func StreamLimitFilterTest(t *testing.T, server *Server) {
	type TestItem struct {
		Value int `json:"value"`
	}

	const limit = 3
	const totalInserts = limit + 5 // Insert more than the limit

	var wg sync.WaitGroup
	maxSeen := 0

	// Set up limit filter - uses ReadListFilter to limit view + AfterWrite to cleanup
	server.LimitFilter("limited/*", limit)
	server.OpenFilter("limited/*")
	cfg := remoteConfig(server)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe to the limited path
	wg.Add(1)
	go client.SubscribeList(client.SubscribeConfig{
		Ctx:      ctx,
		Protocol: "ws",
		Host:     server.Address,
		Silence:  true,
	}, "limited/*", client.SubscribeListEvents[TestItem]{
		OnMessage: func(m []client.Meta[TestItem]) {
			if len(m) > maxSeen {
				maxSeen = len(m)
			}
			wg.Done()
		},
	})

	// Wait for initial empty snapshot
	wg.Wait()

	// Insert items via HTTP (triggers AfterWrite for LimitFilter cleanup)
	// Each insert triggers 1 broadcast for the insert
	// When over limit, AfterWrite deletes old items which triggers additional broadcasts
	// First 'limit' inserts: 1 broadcast each
	// Remaining inserts: 2 broadcasts each (insert + delete)
	for i := range totalInserts {
		wg.Add(1) // just the insert broadcast
		err := ooio.RemotePush(cfg, "limited/*", TestItem{Value: i})
		require.NoError(t, err)
		wg.Wait()
	}

	// Verify final state - client should never see more than limit items due to ReadListFilter
	require.LessOrEqual(t, maxSeen, limit, "client should never see more than limit items")

	// Verify storage has exactly 'limit' items after cleanup
	stored, err := server.Storage.GetList("limited/*")
	require.NoError(t, err)
	require.Equal(t, limit, len(stored), "storage should have exactly limit items")
}
