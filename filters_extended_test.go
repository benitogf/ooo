package ooo

import (
	"bytes"
	"errors"
	"io"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/goccy/go-json"
	"github.com/stretchr/testify/require"
)

func TestFiltersEdgeCases(t *testing.T) {
	app := Server{}
	app.Silence = true

	// Test NoopFilter and NoopHook
	data := json.RawMessage(`{"test":true}`)
	result, err := NoopFilter("test", data)
	require.NoError(t, err)
	require.Equal(t, data, result)

	err = NoopHook("test")
	require.NoError(t, err)

	// Test OpenFilter
	app.OpenFilter("opentest")
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	// Verify open filter allows operations
	req := httptest.NewRequest("POST", "/opentest", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, 200, resp.StatusCode)
}

func TestFiltersStaticMode(t *testing.T) {
	app := Server{}
	app.Silence = true
	app.Static = true

	// Add filter for specific path
	app.WriteFilter("allowed/*", func(key string, data json.RawMessage) (json.RawMessage, error) {
		return data, nil
	})
	app.ReadFilter("allowed/*", func(key string, data json.RawMessage) (json.RawMessage, error) {
		return data, nil
	})
	app.DeleteFilter("allowed/*", func(key string) error {
		return nil
	})

	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	data := json.RawMessage(`{"test":true}`)

	// Test allowed path
	req := httptest.NewRequest("POST", "/allowed/test", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, 200, resp.StatusCode)

	// Test disallowed path in static mode
	req = httptest.NewRequest("POST", "/disallowed/test", bytes.NewBuffer(data))
	w = httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, 400, resp.StatusCode)
}

func TestFiltersReturnError(t *testing.T) {
	app := Server{}
	app.Silence = true

	// Filter that returns error
	app.WriteFilter("errortest", func(key string, data json.RawMessage) (json.RawMessage, error) {
		return nil, errors.New("filter error")
	})

	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	data := json.RawMessage(`{"test":true}`)
	req := httptest.NewRequest("POST", "/errortest", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, 400, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "filter error")
}

func TestFiltersReturnEmptyData(t *testing.T) {
	app := Server{}
	app.Silence = true

	// Filter that returns empty data
	app.WriteFilter("emptytest", func(key string, data json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(``), nil
	})

	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	data := json.RawMessage(`{"test":true}`)
	req := httptest.NewRequest("POST", "/emptytest", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, 400, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "error")
}

func TestFiltersReturnInvalidJSON(t *testing.T) {
	app := Server{}
	app.Silence = true

	// Filter that returns invalid JSON bytes that will cause marshal error
	app.WriteFilter("invalidtest", func(key string, data json.RawMessage) (json.RawMessage, error) {
		// Return invalid JSON bytes that will cause json.Marshal to fail in the filter check
		return json.RawMessage("\xff\xfe\xfd"), nil
	})

	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	data := json.RawMessage(`{"test":true}`)
	req := httptest.NewRequest("POST", "/invalidtest", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, 400, resp.StatusCode)
}

func TestDeleteFilter(t *testing.T) {
	app := Server{}
	app.Silence = true

	// Delete filter that denies deletion
	app.DeleteFilter("nodelete", func(key string) error {
		return errors.New("deletion not allowed")
	})

	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	// First set some data
	data := json.RawMessage(`{"test":true}`)
	req := httptest.NewRequest("POST", "/nodelete", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, 200, resp.StatusCode)

	// Try to delete (should be denied)
	req = httptest.NewRequest("DELETE", "/nodelete", nil)
	w = httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, 400, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "deletion not allowed")
}

func TestAfterWriteCallback(t *testing.T) {
	app := Server{}
	app.Silence = true

	callbackTriggered := false
	app.AfterWrite("callbacktest", func(key string) {
		callbackTriggered = true
		require.Equal(t, "callbacktest", key)
	})

	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	data := json.RawMessage(`{"test":true}`)
	req := httptest.NewRequest("POST", "/callbacktest", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, 200, resp.StatusCode)

	require.True(t, callbackTriggered)
}

func TestFiltersGlobMatching(t *testing.T) {
	app := Server{}
	app.Silence = true

	// Filter with glob pattern
	app.WriteFilter("glob/*/test", func(key string, data json.RawMessage) (json.RawMessage, error) {
		// Modify data to prove filter was applied
		return json.RawMessage(`{"filtered":true}`), nil
	})

	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	data := json.RawMessage(`{"original":true}`)
	req := httptest.NewRequest("POST", "/glob/123/test", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, 200, resp.StatusCode)

	// Verify data was filtered
	storedData, err := app.Storage.Get("glob/123/test")
	require.NoError(t, err)
	require.Contains(t, string(storedData), "filtered")
	require.NotContains(t, string(storedData), "original")
}

func TestFiltersMultipleMatches(t *testing.T) {
	app := Server{}
	app.Silence = true

	// Multiple filters for same path - first match should win
	app.WriteFilter("multi", func(key string, data json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"first":true}`), nil
	})
	app.WriteFilter("multi", func(key string, data json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"second":true}`), nil
	})

	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	data := json.RawMessage(`{"original":true}`)
	req := httptest.NewRequest("POST", "/multi", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, 200, resp.StatusCode)

	// Verify first filter was applied
	storedData, err := app.Storage.Get("multi")
	require.NoError(t, err)
	require.Contains(t, string(storedData), "first")
	require.NotContains(t, string(storedData), "second")
}
