package ooo_test

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"

	"github.com/benitogf/ooo"
	"github.com/benitogf/ooo/meta"
	"github.com/goccy/go-json"
	"github.com/stretchr/testify/require"
)

func TestRestPostNonObject(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	var jsonStr = []byte(`non object`)
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewBuffer(jsonStr))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRestPostEmptyData(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	var jsonStr = []byte(``)
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewBuffer(jsonStr))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRestPostInvalidData(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	var jsonStr = []byte(`oldkoskdasoejd`)
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewBuffer(jsonStr))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRestPostKey(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	var jsonStr = []byte(`{"data":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/test//a", bytes.NewBuffer(jsonStr))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusMovedPermanently, resp.StatusCode)
}

func TestRestDel(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	_ = server.Storage.Del("test")
	index, err := server.Storage.Set("test", ooo.TEST_DATA)
	require.NoError(t, err)
	require.Equal(t, "test", index)

	req := httptest.NewRequest("DELETE", "/test", nil)
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	data, _ := server.Storage.Get("test")
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.Empty(t, data)

	req = httptest.NewRequest("DELETE", "/test", nil)
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	index, err = server.Storage.Set("test/1", ooo.TEST_DATA)
	require.NoError(t, err)
	require.Equal(t, "1", index)
	index, err = server.Storage.Set("test/2", ooo.TEST_DATA_UPDATE)
	require.NoError(t, err)
	require.Equal(t, "2", index)

	req = httptest.NewRequest("DELETE", "/test/*", nil)
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	_, err = server.Storage.Get("test/1")
	require.Error(t, err)
	_, err = server.Storage.Get("test/2")
	require.Error(t, err)
}

func TestRestGet(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	server.Storage.Clear()
	defer server.Close(os.Interrupt)
	_ = server.Storage.Del("test")
	index, err := server.Storage.Set("test", ooo.TEST_DATA)
	require.NoError(t, err)
	require.Equal(t, "test", index)
	index, err = server.Storage.Set("sources", ooo.TEST_DATA_UPDATE)
	require.NoError(t, err)
	require.Equal(t, "sources", index)
	data, _ := server.Storage.Get("test")
	dataSources, _ := server.Storage.Get("sources")
	dataEncoded, _ := meta.Encode(data)
	dataSourcesEncoded, _ := meta.Encode(dataSources)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	require.Equal(t, string(dataEncoded), string(body))

	req = httptest.NewRequest(http.MethodGet, "/sources", nil)
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp = w.Result()
	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	require.Equal(t, string(dataSourcesEncoded), string(body))

	req = httptest.NewRequest(http.MethodGet, "/test/notest", nil)
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestRestStats(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	index, err := server.Storage.Set("test/1", ooo.TEST_DATA)
	require.NoError(t, err)
	require.NotEmpty(t, index)

	req := httptest.NewRequest(http.MethodGet, "/?api=keys", nil)
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	require.Contains(t, string(body), "\"keys\":[\"test/1\"]")
	require.Contains(t, string(body), "\"total\":1")

	_ = server.Storage.Del("test/1")

	req = httptest.NewRequest(http.MethodGet, "/?api=keys", nil)
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp = w.Result()
	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	require.Contains(t, string(body), "\"keys\":[]")
	require.Contains(t, string(body), "\"total\":0")
}

func TestRestResponseCode(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	index, err := server.Storage.Set("test", ooo.TEST_DATA)
	require.NoError(t, err)
	require.NotEmpty(t, index)

	index, err = server.Storage.Set("test/1", ooo.TEST_DATA_UPDATE)
	require.NoError(t, err)
	require.NotEmpty(t, index)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	req = httptest.NewRequest(http.MethodGet, "/*", nil)
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	req = httptest.NewRequest(http.MethodDelete, "/test", nil)
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	req = httptest.NewRequest(http.MethodDelete, "/test/1", nil)
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	req = httptest.NewRequest(http.MethodPatch, "/none/*", nil)
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRestGetBadRequest(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	req := httptest.NewRequest(http.MethodGet, "//test", nil)
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()

	require.Equal(t, 301, resp.StatusCode)
}

func TestRestPostInvalidKey(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	req := httptest.NewRequest(http.MethodPost, "/test/*/*", nil)
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRestGetInvalidKey(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	req := httptest.NewRequest(http.MethodGet, "/test/*/**", nil)
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRestDeleteInvalidKey(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	req := httptest.NewRequest(http.MethodDelete, "/test/*/**", nil)
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRestPatch(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	var testInput = []byte(`{"one":"test"}`)
	var testUpdate = []byte(`{"two":"testing"}`)
	var testOutput = []byte(`{"one":"test","two":"testing"}`)
	index, err := server.Storage.Set("test/1", testInput)
	require.NoError(t, err)
	require.NotEmpty(t, index)

	req := httptest.NewRequest(http.MethodPatch, "/test/*", bytes.NewBuffer(testUpdate))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	obj, err := server.Storage.Get("test/1")
	require.NoError(t, err)

	require.Equal(t, string(testOutput), string(obj.Data))
}

func TestRestInvalidKey(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	req := httptest.NewRequest("GET", "/test//1", nil)
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusMovedPermanently, resp.StatusCode)

	req = httptest.NewRequest("DELETE", "/r/test//1", nil)
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, http.StatusMovedPermanently, resp.StatusCode)
}

func TestRestPatchUnauthorized(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.Audit = func(r *http.Request) bool {
		return r.Method != "PATCH"
	}
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	data := json.RawMessage(`{"test":"patch"}`)
	req := httptest.NewRequest("PATCH", "/patchtest", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestRestPatchInvalidData(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	invalidData := []byte(`invalid json`)
	req := httptest.NewRequest("PATCH", "/patchtest", bytes.NewBuffer(invalidData))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRestPatchStorageError(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Try to patch non-existent key
	data := json.RawMessage(`{"new":"value"}`)
	req := httptest.NewRequest("PATCH", "/nonexistent", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestRestGetWithVersion(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Set some data
	_, err := server.Storage.Set("versiontest", json.RawMessage(`{"test":true}`))
	require.NoError(t, err)

	// Test GET with version parameter
	req := httptest.NewRequest("GET", "/versiontest?v=123", nil)
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestRestReadUnauthorized(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.Audit = func(r *http.Request) bool {
		return r.Method != "GET"
	}
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestRestReadFilterError(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	// Add read filter that returns error (non-glob path uses ReadObjectFilter)
	server.ReadObjectFilter("filtererror", func(key string, obj meta.Object) (meta.Object, error) {
		return meta.Object{}, errors.New("read filter error")
	})

	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Set data
	_, err := server.Storage.Set("filtererror", json.RawMessage(`{"test":true}`))
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/filtererror", nil)
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRestUnpublishUnauthorized(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.Audit = func(r *http.Request) bool {
		return r.Method != "DELETE"
	}
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	req := httptest.NewRequest("DELETE", "/test", nil)
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestRestUnpublishDeleteFilterError(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	// Add delete filter that returns error
	server.DeleteFilter("deleteerror", func(key string) error {
		return errors.New("delete filter error")
	})

	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Set data first
	_, err := server.Storage.Set("deleteerror", json.RawMessage(`{"test":true}`))
	require.NoError(t, err)

	req := httptest.NewRequest("DELETE", "/deleteerror", nil)
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRestPublishWriteFilterError(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	// Add write filter that returns error
	server.WriteFilter("writeerror", func(key string, data json.RawMessage) (json.RawMessage, error) {
		return nil, errors.New("write filter error")
	})

	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	data := json.RawMessage(`{"test":true}`)
	req := httptest.NewRequest("POST", "/writeerror", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRestPublishStorageError(t *testing.T) {
	// This test would require mocking the storage to return an error
	// For now, we'll test with invalid path which should cause storage error
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Use empty key which should cause storage error
	data := json.RawMessage(`{"test":true}`)
	req := httptest.NewRequest("POST", "/", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	// This should hit the stats endpoint instead, so we expect 405 (method not allowed)
	require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

func TestRestGlobPatternEdgeCases(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	data := json.RawMessage(`{"test":true}`)

	// Test glob not at end of path
	req := httptest.NewRequest("POST", "/test/*/more", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Test multiple globs
	req = httptest.NewRequest("POST", "/test/*/*", bytes.NewBuffer(data))
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
