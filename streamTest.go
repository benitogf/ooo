package ooo

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"

	"github.com/goccy/go-json"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/benitogf/jsondiff"
	"github.com/benitogf/jsonpatch"
	"github.com/benitogf/ooo/messages"
	"github.com/benitogf/ooo/meta"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/pkg/expect"
	"github.com/stretchr/testify/require"
)

// StreamBroadcastTest testing stream function
func StreamBroadcastTest(t *testing.T, app *Server) {
	var wg sync.WaitGroup
	// this lock should not be neccesary but the race detector doesnt recognize the wait group preventing the race here
	var lk sync.Mutex
	var postObject meta.Object
	var wsObject meta.Object
	var wsEvent messages.Message
	var wsCache json.RawMessage
	wsURL := url.URL{Scheme: "ws", Host: app.Address, Path: "/test"}
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
			app.Console.Log("read wsClient", wsEvent.Data)
			wg.Done()
		}
	}()
	wg.Wait()
	wg.Add(1)
	lk.Lock()
	wsCache = wsEvent.Data
	wsVersion, err := strconv.ParseInt(wsEvent.Version, 16, 64)
	lk.Unlock()
	require.NoError(t, err)
	streamCacheVersion, err := app.Stream.GetCacheVersion("test")
	require.NoError(t, err)
	app.Console.Log("post data")
	req := httptest.NewRequest("POST", "/test", bytes.NewBuffer(TEST_DATA))
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	err = json.Unmarshal(body, &postObject)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	require.Equal(t, wsVersion, streamCacheVersion)
	wg.Wait()
	wg.Add(1)

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

	require.Equal(t, wsObject.Index, postObject.Index)
	same, _ := jsondiff.Compare(wsObject.Data, TEST_DATA, &jsondiff.Options{})
	require.Equal(t, same, jsondiff.FullMatch)

	req = httptest.NewRequest("DELETE", "/test", nil)
	w = httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
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
func StreamItemGlobBroadcastTest(t *testing.T, app *Server) {
	var wg sync.WaitGroup
	// this lock should not be neccesary but the race detector doesnt recognize the wait group preventing the race here
	var lk sync.Mutex
	var postObject meta.Object
	var wsObject meta.Object
	var wsEvent messages.Message
	var wsCache json.RawMessage
	wsURL := url.URL{Scheme: "ws", Host: app.Address, Path: "/test/1"}
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
			app.Console.Log("read wsClient", wsEvent.Data)
			wg.Done()
		}
	}()
	wg.Wait()
	wg.Add(1)
	lk.Lock()
	wsCache = wsEvent.Data
	lk.Unlock()
	var jsonStr = []byte(TEST_DATA)
	req := httptest.NewRequest("POST", "/test/1", bytes.NewBuffer(jsonStr))
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	err = json.Unmarshal(body, &postObject)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	wg.Wait()
	patch, err := jsonpatch.DecodePatch([]byte(wsEvent.Data))
	require.NoError(t, err)
	modified, err := patch.Apply([]byte(wsCache))
	require.NoError(t, err)
	err = json.Unmarshal(modified, &wsObject)
	require.NoError(t, err)
	wsCache = modified

	require.Equal(t, wsObject.Index, postObject.Index)
	same, _ := jsondiff.Compare(wsObject.Data, TEST_DATA, &jsondiff.Options{})
	require.Equal(t, same, jsondiff.FullMatch)

	wg.Add(1)
	req = httptest.NewRequest("DELETE", "/test/*", nil)
	w = httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
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
func StreamGlobBroadcastTest(t *testing.T, app *Server, n int) {
	var wg sync.WaitGroup
	// this lock should not be neccesary but the race detector doesnt recognize the wait group preventing the race here
	var lk sync.Mutex
	var postObject meta.Object
	var wsObjects []meta.Object
	var wsEvent messages.Message
	var wsCache json.RawMessage
	wsURL := url.URL{Scheme: "ws", Host: app.Address, Path: "/test/*"}
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
			// app.Console.Log("read wsClient", wsEvent.Data)
			wg.Done()
		}
	}()
	wg.Wait()

	lk.Lock()
	wsCache = wsEvent.Data
	lk.Unlock()
	app.Console.Log("post data")
	keys := []string{}
	for i := 0; i < n; i++ {
		wg.Add(1)
		req := httptest.NewRequest("POST", "/test/*", bytes.NewBuffer(TEST_DATA))
		w := httptest.NewRecorder()
		app.Router.ServeHTTP(w, req)
		resp := w.Result()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		err = json.Unmarshal(body, &postObject)
		require.NoError(t, err)
		require.Equal(t, 200, resp.StatusCode)
		wg.Wait()
		lk.Lock()
		require.False(t, wsEvent.Snapshot)
		patch, err := jsonpatch.DecodePatch([]byte(wsEvent.Data))
		require.NoError(t, err)
		modified, err := patch.Apply([]byte(wsCache))
		require.NoError(t, err)
		err = json.Unmarshal(modified, &wsObjects)
		require.NoError(t, err)
		wsCache = modified
		lk.Unlock()
		keys = append(keys, postObject.Index)
	}

	require.Equal(t, wsObjects[len(wsObjects)-1].Index, postObject.Index)
	same, _ := jsondiff.Compare(wsObjects[len(wsObjects)-1].Data, TEST_DATA, &jsondiff.Options{})
	require.Equal(t, same, jsondiff.FullMatch)

	app.Console.Log("post update data")
	nextGet := float64(0)
	Q := 3
	for i := 0; i < Q; i++ {
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
			app.Console.Log("post", key, i, nextSet)
			newData, err := sjson.Set(string(testData), "search_metadata.count", nextSet)
			require.NoError(t, err)
			req := httptest.NewRequest("POST", "/test/"+key, bytes.NewBuffer([]byte(newData)))
			w := httptest.NewRecorder()
			app.Router.ServeHTTP(w, req)
			resp := w.Result()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			err = json.Unmarshal(body, &postObject)
			require.NoError(t, err)
			require.Equal(t, 200, resp.StatusCode)
			wg.Wait()
			lk.Lock()
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
			lk.Unlock()
		}
	}

	require.Equal(t, float64(4+Q), nextGet)

	wg.Add(1)
	req := httptest.NewRequest("DELETE", "/test/*", nil)
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	wg.Wait()

	lk.Lock()
	require.False(t, wsEvent.Snapshot)
	patch, err := jsonpatch.DecodePatch([]byte(wsEvent.Data))
	require.NoError(t, err)
	modified, err := patch.Apply([]byte(wsCache))
	require.NoError(t, err)
	err = json.Unmarshal(modified, &wsObjects)
	require.NoError(t, err)
	lk.Unlock()

	wsClient.Close()

	require.Equal(t, len(wsObjects), 0)
}

// StreamBroadcastFilterTest testing stream function
func StreamBroadcastFilterTest(t *testing.T, app *Server) {
	var wg sync.WaitGroup
	var postObject meta.Object
	var wsExtraEvent messages.Message
	// extra filter
	app.ReadFilter("test/*", func(index string, data json.RawMessage) (json.RawMessage, error) {
		return []byte(`{"extra": "extra"}`), nil
	})
	// extra route
	app.Router = mux.NewRouter()
	app.Start("localhost:0")
	app.Storage.Clear()
	wsExtraURL := url.URL{Scheme: "ws", Host: app.Address, Path: "/test/*"}
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
			app.Console.Log("read wsClient", string(message))
			wg.Done()
		}
	}()
	wg.Wait()
	wg.Add(1)
	app.Console.Log("post data")
	req := httptest.NewRequest("POST", "/test/*", bytes.NewBuffer(TEST_DATA))
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	err = json.Unmarshal(body, &postObject)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	wg.Wait()
	wsExtraClient.Close()

	// empty operations for a broadcast with no changes
	require.Equal(t, false, wsExtraEvent.Snapshot)
	require.Equal(t, "[]", string(wsExtraEvent.Data))
}

// StreamBroadcastForcePatchTest testing stream function
func StreamBroadcastForcePatchTest(t *testing.T, app *Server) {
	var wg sync.WaitGroup
	var postObject meta.Object
	var wsExtraEvent messages.Message
	// extra route
	app.Router = mux.NewRouter()
	app.ForcePatch = true
	app.Start("localhost:0")
	app.Storage.Clear()
	wsExtraURL := url.URL{Scheme: "ws", Host: app.Address, Path: "/test/*"}
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
			app.Console.Log("read wsClient", string(message))
			wg.Done()
		}
	}()
	wg.Wait()
	wg.Add(1)
	app.Console.Log("post data")
	req := httptest.NewRequest("POST", "/test/*", bytes.NewBuffer(TEST_DATA))
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	err = json.Unmarshal(body, &postObject)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	wg.Wait()

	wg.Add(1)
	app.Console.Log("post data")
	req = httptest.NewRequest("POST", "/test/*", bytes.NewBuffer(TEST_DATA))
	w = httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp = w.Result()
	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	err = json.Unmarshal(body, &postObject)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	wg.Wait()

	wg.Add(1)
	app.Console.Log("post data")
	req = httptest.NewRequest("POST", "/test/*", bytes.NewBuffer(TEST_DATA))
	w = httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp = w.Result()
	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	err = json.Unmarshal(body, &postObject)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	wg.Wait()

	wg.Add(1)
	app.Console.Log("post data")
	req = httptest.NewRequest("POST", "/test/*", bytes.NewBuffer(TEST_DATA))
	w = httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp = w.Result()
	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	err = json.Unmarshal(body, &postObject)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	wg.Wait()

	wsExtraClient.Close()
}

// StreamBroadcastNoPatchTest testing stream function
func StreamBroadcastNoPatchTest(t *testing.T, app *Server) {
	var wg sync.WaitGroup
	var postObject meta.Object
	var wsExtraEvent messages.Message
	// extra route
	app.Router = mux.NewRouter()
	app.NoPatch = true
	app.Start("localhost:0")
	app.Storage.Clear()
	wsExtraURL := url.URL{Scheme: "ws", Host: app.Address, Path: "/test/*"}
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
			app.Console.Log("read wsClient", string(message))
			wg.Done()
		}
	}()
	wg.Wait()
	wg.Add(1)
	app.Console.Log("post data")
	req := httptest.NewRequest("POST", "/test/*", bytes.NewBuffer(TEST_DATA))
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	err = json.Unmarshal(body, &postObject)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	wg.Wait()

	wg.Add(1)
	app.Console.Log("post data")
	req = httptest.NewRequest("POST", "/test/*", bytes.NewBuffer(TEST_DATA))
	w = httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp = w.Result()
	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	err = json.Unmarshal(body, &postObject)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	wg.Wait()

	wsExtraClient.Close()
}

// StreamGlobBroadcastConcurretTest testing stream function
func StreamGlobBroadcastConcurretTest(t *testing.T, app *Server, n int) {
	var wg sync.WaitGroup
	var wsObjects []meta.Object
	var wsEvent messages.Message
	var wsCache json.RawMessage
	wsURL := url.URL{Scheme: "ws", Host: app.Address, Path: "/test/*"}
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
			if !wsEvent.Snapshot {
				patch, err := jsonpatch.DecodePatch([]byte(wsEvent.Data))
				require.NoError(t, err)
				modified, err := patch.Apply([]byte(wsCache))
				require.NoError(t, err)
				err = json.Unmarshal(modified, &wsObjects)
				require.NoError(t, err)
				wsCache = modified
			} else {
				err = json.Unmarshal(wsEvent.Data, &wsObjects)
				require.NoError(t, err)
				wsCache = wsEvent.Data
			}
			wg.Done()
		}
	}()
	wg.Wait() // first read
	require.Zero(t, len(wsObjects))

	app.Console.Log("post data")
	keys := []string{}
	for i := 0; i < n; i++ {
		wg.Add(1)
		// _key := key.Build("test/*")
		_key := "test/" + strconv.Itoa(i)
		app.Storage.Set(_key, TEST_DATA)
		keys = append(keys, _key)
	}

	wg.Wait() // created
	require.Equal(t, len(keys), len(wsObjects))
	same, _ := jsondiff.Compare(wsObjects[0].Data, TEST_DATA, &jsondiff.Options{})
	require.Equal(t, same, jsondiff.FullMatch)

	app.Console.Log("post update data", len(keys))
	Q := 6
	for _, _key := range keys {
		wg.Add(Q)
		go func(__key string) {
			for i := 0; i < Q; i++ {
				currentObj := meta.Object{}
				rawCurrent, err := app.Storage.GetAndLock(__key)
				expect.Nil(err)
				err = json.Unmarshal(rawCurrent, &currentObj)
				expect.Nil(err)
				currentRaw := gjson.Get(string(currentObj.Data), "search_metadata.count")
				current := currentRaw.Value().(float64)
				nextSet := current + 1
				app.Console.Log("up", __key, nextSet)
				newData, err := sjson.Set(string(currentObj.Data), "search_metadata.count", nextSet)
				expect.Nil(err)
				_, err = app.Storage.SetAndUnlock(__key, json.RawMessage(newData))
				expect.Nil(err)
			}
		}(_key)
	}

	for _, _key := range keys {
		wg.Add(Q)
		go func(__key string) {
			for i := 0; i < Q; i++ {
				currentObj := meta.Object{}
				rawCurrent, err := app.Storage.GetAndLock(__key)
				expect.Nil(err)
				err = json.Unmarshal(rawCurrent, &currentObj)
				expect.Nil(err)
				currentRaw := gjson.Get(string(currentObj.Data), "search_metadata.something")
				current := currentRaw.Value().(string)
				nextSet := "popo"
				if current == "popo" {
					nextSet = "nopo"
				}
				app.Console.Log("up", __key, nextSet)
				newData, err := sjson.Set(string(currentObj.Data), "search_metadata.something", nextSet)
				expect.Nil(err)
				_, err = app.Storage.SetAndUnlock(__key, json.RawMessage(newData))
				expect.Nil(err)
			}
		}(_key)
	}

	app.Console.Log("wait update data")
	wg.Wait() // updated

	// wsKeys := []string{}
	// for _, obj := range wsObjects {
	// 	wsKeys = append(wsKeys, obj.Index)
	// }
	// app.Console.Log("after", keys, wsKeys)
	require.Equal(t, len(keys), len(wsObjects))
	for _, obj := range wsObjects {
		currentRaw := gjson.Get(string(obj.Data), "search_metadata.count")
		current := currentRaw.Value().(float64)
		require.Equal(t, float64(4+Q), current)
	}

	_, err = app.Storage.GetAndLock("test/*")
	require.Error(t, err)
	_, err = app.Storage.SetAndUnlock("test/*", TEST_DATA)
	require.Error(t, err)

	wsClient.Close()
}
