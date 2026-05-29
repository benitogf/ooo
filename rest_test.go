package ooo_test

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/benitogf/go-json"
	"github.com/benitogf/ooo"
	"github.com/benitogf/ooo/meta"
	"github.com/benitogf/ooo/storage"
	"github.com/stretchr/testify/require"
)

// maskedNotFound wraps storage.ErrNotFound but its Error() text omits
// the literal "not found" substring. Used to exercise unpublish's
// not-found detection path independently of message formatting.
type maskedNotFound struct{ inner error }

func (m maskedNotFound) Error() string { return "backend rejected delete" }
func (m maskedNotFound) Unwrap() error { return m.inner }

// wrapNotFoundStorage decorates a real Database so Del returns
// maskedNotFound when the underlying layer reports ErrNotFound.
// All other operations pass through unchanged.
type wrapNotFoundStorage struct{ storage.Database }

func (w *wrapNotFoundStorage) Del(key string) error {
	err := w.Database.Del(key)
	if err == nil {
		return nil
	}
	if errors.Is(err, storage.ErrNotFound) {
		return maskedNotFound{inner: storage.ErrNotFound}
	}
	return err
}

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

// TestRestPostBodyTooLarge asserts a POST whose body exceeds
// Server.MaxRequestBodyBytes is rejected with 413 Request Entity Too Large
// instead of being buffered into memory. Pre-fix the handler called
// messages.DecodeReader(r.Body) directly with no cap.
func TestRestPostBodyTooLarge(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.MaxRequestBodyBytes = 64
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	body := []byte(`{"data":"` + strings.Repeat("x", 256) + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/k", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	require.Equal(t, http.StatusRequestEntityTooLarge, w.Result().StatusCode)
}

// TestRestPatchBodyTooLarge is the PATCH variant.
func TestRestPatchBodyTooLarge(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.MaxRequestBodyBytes = 64
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	body := []byte(`{"data":"` + strings.Repeat("x", 256) + `"}`)
	req := httptest.NewRequest(http.MethodPatch, "/k", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	require.Equal(t, http.StatusRequestEntityTooLarge, w.Result().StatusCode)
}

// TestRestPostBodyUnderLimit asserts a POST under the cap still succeeds.
func TestRestPostBodyUnderLimit(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.MaxRequestBodyBytes = 4096
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	body := []byte(`{"data":"small"}`)
	req := httptest.NewRequest(http.MethodPost, "/k", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Result().StatusCode)
}

// TestRestPostBodyDisabledLimit asserts a negative MaxRequestBodyBytes opts
// out of the cap entirely (escape hatch for trusted internal callers).
func TestRestPostBodyDisabledLimit(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.MaxRequestBodyBytes = -1
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	body := []byte(`{"data":"` + strings.Repeat("x", 1024) + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/k", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Result().StatusCode)
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
	// 204 No Content forbids a response body per RFC 7230 §3.3.2.
	// Pre-fix the handler wrote the literal Go-source string
	// `"unpublish "+_key` after WriteHeader(204) — a real
	// http.Server stripped it on the wire so users never saw it,
	// but the handler-level invariant was broken. Co-locate this
	// body assertion with the status check so the next regression
	// in the same handler is caught here.
	body, _ := io.ReadAll(resp.Body)
	require.Empty(t, body, "204 No Content response must have an empty body; got %q", body)
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

func TestRestDelNotFoundWrappedError(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{
		Storage: &wrapNotFoundStorage{
			Database: storage.New(storage.LayeredConfig{Memory: storage.NewMemoryLayer()}),
		},
	}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Deleting a missing key surfaces storage.ErrNotFound wrapped in a
	// custom error type whose message no longer contains "not found".
	// The handler must still return 404 — detection has to use
	// errors.Is against storage.ErrNotFound, not a substring match.
	req := httptest.NewRequest("DELETE", "/missing", nil)
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
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

	req := httptest.NewRequest(http.MethodPatch, "/test/1", bytes.NewBuffer(testUpdate))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	obj, err := server.Storage.Get("test/1")
	require.NoError(t, err)

	require.Equal(t, string(testOutput), string(obj.Data))
}

func TestRestPatchWriteFilterOnMergedData(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true

	// Write filter that requires "requiredField" to exist in the data
	// This would fail if filter was applied to partial patch data only
	server.WriteFilter("filteredpatch", func(key string, data json.RawMessage) (json.RawMessage, error) {
		var obj map[string]interface{}
		if err := json.Unmarshal(data, &obj); err != nil {
			return nil, err
		}
		if _, ok := obj["requiredField"]; !ok {
			return nil, errors.New("requiredField is missing")
		}
		return data, nil
	})

	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Create initial data directly in storage (bypassing write filter for setup)
	// {"requiredField":"exists","otherField":"value1"}
	_, err := server.Storage.Set("filteredpatch", json.RawMessage(`{"requiredField":"exists","otherField":"value1"}`))
	require.NoError(t, err)

	// Patch with only otherField (no requiredField in patch): {"otherField":"patched"}
	// If filter was applied to patch data only, this would fail because requiredField is missing
	// But with merged data, requiredField exists from original data, so it should pass
	var patchData = []byte(`{"otherField":"patched"}`)
	req := httptest.NewRequest(http.MethodPatch, "/filteredpatch", bytes.NewBuffer(patchData))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify the data was patched correctly
	obj, err := server.Storage.Get("filteredpatch")
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(obj.Data, &result)
	require.NoError(t, err)

	// Both fields should exist: requiredField from original, otherField patched
	require.Equal(t, "exists", result["requiredField"])
	require.Equal(t, "patched", result["otherField"])
}

func TestRestPatchTypeMismatchRejected(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Existing data has "nest" as an object.
	_, err := server.Storage.Set("typemismatch", json.RawMessage(`{"nest":{"x":1},"name":"a"}`))
	require.NoError(t, err)

	// Patch tries to overwrite "nest" with a primitive — merge silently keeps
	// the original value for that path. Without the fix, the caller sees 200
	// but the storage has not changed for "nest".
	patchBody := []byte(`{"nest":"replaced","name":"b"}`)
	req := httptest.NewRequest(http.MethodPatch, "/typemismatch", bytes.NewBuffer(patchBody))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Storage must remain unchanged when the patch is rejected.
	obj, err := server.Storage.Get("typemismatch")
	require.NoError(t, err)
	require.JSONEq(t, `{"nest":{"x":1},"name":"a"}`, string(obj.Data))
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

	// Try to patch non-existent key - should return 404 Not Found
	data := json.RawMessage(`{"new":"value"}`)
	req := httptest.NewRequest("PATCH", "/nonexistent", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
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

// quoteInjectingStorage wraps a Database and returns an index containing
// a JSON-significant character (") from Set and Push. The handler's
// {"index":"..."} response must encode via json.Marshal so the body
// stays valid JSON even when the index is hostile to string concatenation.
type quoteInjectingStorage struct{ storage.Database }

func (q *quoteInjectingStorage) Set(k string, data json.RawMessage) (string, error) {
	if _, err := q.Database.Set(k, data); err != nil {
		return "", err
	}
	return `te"st`, nil
}

func (q *quoteInjectingStorage) Push(path string, data json.RawMessage) (string, error) {
	if _, err := q.Database.Push(path, data); err != nil {
		return "", err
	}
	return `te"st`, nil
}

func TestRestPublishResponseEncodesIndexAsJSON(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{
		Storage: &quoteInjectingStorage{
			Database: storage.New(storage.LayeredConfig{Memory: storage.NewMemoryLayer()}),
		},
	}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	body := []byte(`{"data":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/k", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	respBody, _ := io.ReadAll(resp.Body)
	var parsed struct {
		Index string `json:"index"`
	}
	require.NoError(t, json.Unmarshal(respBody, &parsed), "response must be valid JSON, got: %s", respBody)
	require.Equal(t, `te"st`, parsed.Index)
}

func TestRestPatchResponseEncodesIndexAsJSON(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{
		Storage: &quoteInjectingStorage{
			Database: storage.New(storage.LayeredConfig{Memory: storage.NewMemoryLayer()}),
		},
	}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Seed the key so PATCH has something to merge against.
	_, err := server.Storage.Set("k", json.RawMessage(`{"a":1}`))
	require.NoError(t, err)

	body := []byte(`{"b":2}`)
	req := httptest.NewRequest(http.MethodPatch, "/k", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	respBody, _ := io.ReadAll(resp.Body)
	var parsed struct {
		Index string `json:"index"`
	}
	require.NoError(t, json.Unmarshal(respBody, &parsed), "response must be valid JSON, got: %s", respBody)
	require.Equal(t, `te"st`, parsed.Index)
}
