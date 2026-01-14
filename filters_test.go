package ooo

import (
	"bytes"
	"errors"
	"io"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"

	"github.com/benitogf/jsondiff"
	"github.com/benitogf/ooo/filters"
	"github.com/benitogf/ooo/meta"
	"github.com/goccy/go-json"

	"github.com/stretchr/testify/require"
)

func TestFilters(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true
	unacceptedData := json.RawMessage(`{"test":false}`)
	acceptedData := json.RawMessage(`{"test":true}`)
	uninterceptedData := json.RawMessage(`{"intercepted":false}`)
	interceptedData := json.RawMessage(`{"intercepted":true}`)
	filteredData := json.RawMessage(`{"filtered":true}`)
	unfilteredData := json.RawMessage(`{"filtered":false}`)
	notified := false
	server.WriteFilter("test1", func(key string, data json.RawMessage) (json.RawMessage, error) {
		comparison, _ := jsondiff.Compare(data, unfilteredData, &jsondiff.Options{})
		if comparison != jsondiff.FullMatch {
			return nil, errors.New("filtered")
		}

		return data, nil
	})
	server.WriteFilter("test/*", func(key string, data json.RawMessage) (json.RawMessage, error) {
		comparison, _ := jsondiff.Compare(data, acceptedData, &jsondiff.Options{})
		if comparison != jsondiff.FullMatch {
			return nil, errors.New("filtered")
		}

		return data, nil
	})
	// Use meta-based list filter for bag/*
	server.ReadListFilter("bag/*", func(key string, objs []meta.Object) ([]meta.Object, error) {
		return []meta.Object{{Data: interceptedData}}, nil
	})
	// Use meta-based object filter for bag/1 (single item read)
	server.ReadObjectFilter("bag/*", func(key string, obj meta.Object) (meta.Object, error) {
		return meta.Object{Data: interceptedData, Created: 1, Index: "1"}, nil
	})

	server.WriteFilter("book/*", func(key string, data json.RawMessage) (json.RawMessage, error) {
		return data, nil
	})
	// Use meta-based list filter for book/*
	server.ReadListFilter("book/*", func(key string, objs []meta.Object) ([]meta.Object, error) {
		return objs, nil
	})
	server.AfterWriteFilter("flyer", func(key string) {
		notified = true
	})

	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	_, err := server.filters.Write.Check("test/1", unacceptedData, false)
	require.Error(t, err)
	_, err = server.filters.Write.Check("test/1", acceptedData, false)
	require.NoError(t, err)
	// Test meta-based object filter
	obj, err := server.filters.ReadObject.Check("bag/1", meta.Object{Data: uninterceptedData}, false)
	require.NoError(t, err)
	comparison, _ := jsondiff.Compare(obj.Data, interceptedData, &jsondiff.Options{})
	require.Equal(t, comparison, jsondiff.FullMatch)
	_, err = server.filters.Write.Check("test1", filteredData, false)
	require.Error(t, err)
	_, err = server.filters.Write.Check("test1", unfilteredData, false)
	require.NoError(t, err)
	// test static - write filters
	_, err = server.filters.Write.Check("book", unacceptedData, true)
	require.Error(t, err)
	_, err = server.filters.Write.Check("book/1/1", unacceptedData, true)
	require.Error(t, err)
	_, err = server.filters.Write.Check("book/1/1/1", unacceptedData, true)
	require.Error(t, err)
	// test static - read list filters
	_, err = server.filters.ReadList.Check("book", nil, true)
	require.Error(t, err)
	_, err = server.filters.ReadList.Check("book/1/1", nil, true)
	require.Error(t, err)
	_, err = server.filters.ReadList.Check("book/1/1/1", nil, true)
	require.Error(t, err)
	_, err = server.filters.Write.Check("book/1", unfilteredData, true)
	require.NoError(t, err)
	_, err = server.filters.ReadList.Check("book/1", nil, true)
	require.NoError(t, err)
	req := httptest.NewRequest("POST", "/test/1", bytes.NewBuffer(TEST_DATA))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, 400, resp.StatusCode)

	req = httptest.NewRequest("POST", "/bag/1", bytes.NewBuffer(uninterceptedData))
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, 200, resp.StatusCode)

	req = httptest.NewRequest("POST", "/flyer", bytes.NewBuffer(TEST_DATA))
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, 200, resp.StatusCode)
	require.True(t, notified)

	req = httptest.NewRequest("GET", "/bag/1", nil)
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp = w.Result()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	// Response is a meta.Object, decode and check the data field
	var respObj meta.Object
	err = json.Unmarshal(body, &respObj)
	require.NoError(t, err)
	comparison, _ = jsondiff.Compare(respObj.Data, interceptedData, &jsondiff.Options{})
	require.Equal(t, comparison, jsondiff.FullMatch)
}

func TestFiltersEdgeCases(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true

	// Test NoopFilter and NoopHook
	data := json.RawMessage(`{"test":true}`)
	result, err := NoopFilter("test", data)
	require.NoError(t, err)
	require.Equal(t, data, result)

	err = NoopHook("test")
	require.NoError(t, err)

	// Test OpenFilter
	server.OpenFilter("opentest")
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Verify open filter allows operations
	req := httptest.NewRequest("POST", "/opentest", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, 200, resp.StatusCode)
}

func TestFiltersStaticMode(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true
	server.Static = true

	// Add filter for specific path
	server.WriteFilter("allowed/*", func(key string, data json.RawMessage) (json.RawMessage, error) {
		return data, nil
	})
	server.ReadListFilter("allowed/*", func(key string, objs []meta.Object) ([]meta.Object, error) {
		return objs, nil
	})
	server.DeleteFilter("allowed/*", func(key string) error {
		return nil
	})

	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	data := json.RawMessage(`{"test":true}`)

	// Test allowed path
	req := httptest.NewRequest("POST", "/allowed/test", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, 200, resp.StatusCode)

	// Test disallowed path in static mode
	req = httptest.NewRequest("POST", "/disallowed/test", bytes.NewBuffer(data))
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, 400, resp.StatusCode)
}

func TestFiltersReturnError(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true

	// Filter that returns error
	server.WriteFilter("errortest", func(key string, data json.RawMessage) (json.RawMessage, error) {
		return nil, errors.New("filter error")
	})

	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	data := json.RawMessage(`{"test":true}`)
	req := httptest.NewRequest("POST", "/errortest", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, 400, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "filter error")
}

func TestFiltersReturnEmptyData(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true

	// Filter that returns empty data
	server.WriteFilter("emptytest", func(key string, data json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(``), nil
	})

	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	data := json.RawMessage(`{"test":true}`)
	req := httptest.NewRequest("POST", "/emptytest", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, 400, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "error")
}

func TestFiltersReturnInvalidJSON(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true

	// Filter that returns invalid JSON bytes that will cause marshal error
	server.WriteFilter("invalidtest", func(key string, data json.RawMessage) (json.RawMessage, error) {
		// Return invalid JSON bytes that will cause json.Marshal to fail in the filter check
		return json.RawMessage("\xff\xfe\xfd"), nil
	})

	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	data := json.RawMessage(`{"test":true}`)
	req := httptest.NewRequest("POST", "/invalidtest", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, 400, resp.StatusCode)
}

func TestFiltersDelete(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true

	// Delete filter that denies deletion
	server.DeleteFilter("nodelete", func(key string) error {
		return errors.New("deletion not allowed")
	})

	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// First set some data
	data := json.RawMessage(`{"test":true}`)
	req := httptest.NewRequest("POST", "/nodelete", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, 200, resp.StatusCode)

	// Try to delete (should be denied)
	req = httptest.NewRequest("DELETE", "/nodelete", nil)
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, 400, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "deletion not allowed")
}

func TestFiltersAfterWrite(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true

	callbackTriggered := false
	server.AfterWriteFilter("callbacktest", func(key string) {
		callbackTriggered = true
		require.Equal(t, "callbacktest", key)
	})

	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	data := json.RawMessage(`{"test":true}`)
	req := httptest.NewRequest("POST", "/callbacktest", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, 200, resp.StatusCode)

	require.True(t, callbackTriggered)
}

func TestFiltersGlobMatching(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true

	// Filter with glob pattern
	server.WriteFilter("glob/*/test", func(key string, data json.RawMessage) (json.RawMessage, error) {
		// Modify data to prove filter was applied
		return json.RawMessage(`{"filtered":true}`), nil
	})

	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	data := json.RawMessage(`{"original":true}`)
	req := httptest.NewRequest("POST", "/glob/123/test", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, 200, resp.StatusCode)

	// Verify data was filtered
	storedData, err := server.Storage.Get("glob/123/test")
	require.NoError(t, err)
	require.Contains(t, string(storedData.Data), "filtered")
	require.NotContains(t, string(storedData.Data), "original")
}

func TestFiltersMultipleMatches(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true

	// Multiple filters for same path - first match should win
	server.WriteFilter("multi", func(key string, data json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"first":true}`), nil
	})
	server.WriteFilter("multi", func(key string, data json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"second":true}`), nil
	})

	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	data := json.RawMessage(`{"original":true}`)
	req := httptest.NewRequest("POST", "/multi", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, 200, resp.StatusCode)

	// Verify first filter was applied
	storedData, err := server.Storage.Get("multi")
	require.NoError(t, err)
	require.Contains(t, string(storedData.Data), "first")
	require.NotContains(t, string(storedData.Data), "second")
}

func TestFiltersOpenGlobAllowsIndividualReads(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true
	server.Static = true

	// OpenFilter for glob path should allow reading both list and individual items
	server.OpenFilter("things/*")

	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Create an item via POST
	data := json.RawMessage(`{"name":"test item"}`)
	req := httptest.NewRequest("POST", "/things/*", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, 200, resp.StatusCode, "POST to things/* should succeed")

	// Parse the response to get the created key
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var createResp struct {
		Index string `json:"index"`
	}
	err = json.Unmarshal(body, &createResp)
	require.NoError(t, err)
	require.NotEmpty(t, createResp.Index)

	// Read the list - should work
	req = httptest.NewRequest("GET", "/things/*", nil)
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, 200, resp.StatusCode, "GET things/* (list) should succeed")

	// Read the individual item - should also work
	req = httptest.NewRequest("GET", "/things/"+createResp.Index, nil)
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, 200, resp.StatusCode, "GET things/123 (individual) should succeed")

	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "test item")
}

func TestFiltersLimitAllowsPostAndRead(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true
	server.Static = true

	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// LimitFilter should allow POST, GET list, and GET individual items
	// Must be called after Start() because it needs app.Storage
	server.LimitFilter("limited/*", filters.LimitFilterConfig{Limit: 5})

	// Create an item via POST
	data := json.RawMessage(`{"name":"limited item"}`)
	req := httptest.NewRequest("POST", "/limited/*", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, 200, resp.StatusCode, "POST to limited/* should succeed")

	// Parse the response to get the created key
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var createResp struct {
		Index string `json:"index"`
	}
	err = json.Unmarshal(body, &createResp)
	require.NoError(t, err)
	require.NotEmpty(t, createResp.Index)

	// Read the list - should work
	req = httptest.NewRequest("GET", "/limited/*", nil)
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, 200, resp.StatusCode, "GET limited/* (list) should succeed")

	// Read the individual item - should also work
	req = httptest.NewRequest("GET", "/limited/"+createResp.Index, nil)
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, 200, resp.StatusCode, "GET limited/123 (individual) should succeed")

	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "limited item")
}

func TestGetListWithLimitFilter(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true
	server.Static = true

	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// LimitFilter with limit of 3, OrderDesc (newest first)
	server.LimitFilter("games/*", filters.LimitFilterConfig{Limit: 3, Order: filters.OrderDesc})

	// Create 5 items with different timestamps
	for i := 1; i <= 5; i++ {
		data := json.RawMessage(`{"name":"game` + string(rune('0'+i)) + `"}`)
		req := httptest.NewRequest("POST", "/games/*", bytes.NewBuffer(data))
		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, req)
		resp := w.Result()
		require.Equal(t, 200, resp.StatusCode)
	}

	// Use GetList to read - should return only 3 newest items
	type Game struct {
		Name string `json:"name"`
	}
	games, err := GetList[Game](&server, "games/*")
	require.NoError(t, err)
	require.Equal(t, 3, len(games), "GetList should respect LimitFilter limit")
}

func TestGetListWithLimitFilterOrderAsc(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true
	server.Static = true

	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// LimitFilter with limit of 3, OrderAsc (oldest first for display, but keeps newest)
	server.LimitFilter("logs/*", filters.LimitFilterConfig{Limit: 3, Order: filters.OrderAsc})

	// Create 5 items
	for i := 1; i <= 5; i++ {
		data := json.RawMessage(`{"level":"info","seq":` + string(rune('0'+i)) + `}`)
		req := httptest.NewRequest("POST", "/logs/*", bytes.NewBuffer(data))
		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, req)
		resp := w.Result()
		require.Equal(t, 200, resp.StatusCode)
	}

	// Use GetList to read - should return 3 items (newest kept, sorted oldest first)
	type Log struct {
		Level string `json:"level"`
		Seq   int    `json:"seq"`
	}
	logs, err := GetList[Log](&server, "logs/*")
	require.NoError(t, err)
	require.Equal(t, 3, len(logs), "GetList should respect LimitFilter limit with OrderAsc")

	// Verify items are sorted oldest first (ascending Created)
	for i := 1; i < len(logs); i++ {
		require.True(t, logs[i-1].Created <= logs[i].Created,
			"Items should be sorted oldest first with OrderAsc")
	}
}

func TestGetListWithLimitFilterKeepsNewest(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true
	server.Static = true

	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// LimitFilter with limit of 2
	server.LimitFilter("items/*", filters.LimitFilterConfig{Limit: 2, Order: filters.OrderAsc})

	// Create 3 items - oldest should be dropped
	var createdKeys []string
	for i := 1; i <= 3; i++ {
		data := json.RawMessage(`{"value":` + string(rune('0'+i)) + `}`)
		req := httptest.NewRequest("POST", "/items/*", bytes.NewBuffer(data))
		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, req)
		resp := w.Result()
		require.Equal(t, 200, resp.StatusCode)

		body, _ := io.ReadAll(resp.Body)
		var createResp struct{ Index string }
		json.Unmarshal(body, &createResp)
		createdKeys = append(createdKeys, createResp.Index)
	}

	// Use GetList - should have 2 items (the 2 newest)
	type Item struct {
		Value int `json:"value"`
	}
	items, err := GetList[Item](&server, "items/*")
	require.NoError(t, err)
	require.Equal(t, 2, len(items), "GetList should return only 2 newest items")

	// Verify the oldest item (first created) is not in the list
	for _, item := range items {
		require.NotEqual(t, createdKeys[0], item.Index,
			"Oldest item should have been filtered out")
	}
}

func TestReadListFilterAllowsIndividualReads(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true
	server.Static = true

	// ReadListFilter for glob path should allow reading both list and individual items
	// This is the fix for: logs/* should allow accessing logs/18895b6dc6b09a1b
	server.WriteFilter("logs/*", NoopFilter)
	server.DeleteFilter("logs/*", NoopHook)
	server.ReadListFilter("logs/*", func(key string, objs []meta.Object) ([]meta.Object, error) {
		return objs, nil
	})

	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Create an item via POST
	data := json.RawMessage(`{"level":"info","message":"test log"}`)
	req := httptest.NewRequest("POST", "/logs/*", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, 200, resp.StatusCode, "POST to logs/* should succeed")

	// Parse the response to get the created key
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var createResp struct {
		Index string `json:"index"`
	}
	err = json.Unmarshal(body, &createResp)
	require.NoError(t, err)
	require.NotEmpty(t, createResp.Index)

	// Read the list - should work
	req = httptest.NewRequest("GET", "/logs/*", nil)
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, 200, resp.StatusCode, "GET logs/* (list) should succeed")

	// Read the individual item - should also work (this was the bug)
	req = httptest.NewRequest("GET", "/logs/"+createResp.Index, nil)
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, 200, resp.StatusCode, "GET logs/123 (individual) should succeed - ReadListFilter must also allow individual reads")

	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "test log")
}
