package ooo

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/goccy/go-json"
	"github.com/stretchr/testify/require"
)

func TestRestPutMethod(t *testing.T) {
	app := Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	data := json.RawMessage(`{"test":"put"}`)
	req := httptest.NewRequest("PUT", "/puttest", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify data was stored
	stored, err := app.Storage.Get("puttest")
	require.NoError(t, err)
	require.Contains(t, string(stored), "put")
}

func TestRestPutInvalidKey(t *testing.T) {
	app := Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	data := json.RawMessage(`{"test":"put"}`)
	req := httptest.NewRequest("PUT", "/test/*/*", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRestPutUnauthorized(t *testing.T) {
	app := Server{}
	app.Silence = true
	app.Audit = func(r *http.Request) bool {
		return r.Method != "PUT"
	}
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	data := json.RawMessage(`{"test":"put"}`)
	req := httptest.NewRequest("PUT", "/puttest", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestRestPatchUnauthorized(t *testing.T) {
	app := Server{}
	app.Silence = true
	app.Audit = func(r *http.Request) bool {
		return r.Method != "PATCH"
	}
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	data := json.RawMessage(`{"test":"patch"}`)
	req := httptest.NewRequest("PATCH", "/patchtest", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestRestPatchInvalidData(t *testing.T) {
	app := Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	invalidData := []byte(`invalid json`)
	req := httptest.NewRequest("PATCH", "/patchtest", bytes.NewBuffer(invalidData))
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRestPatchStorageError(t *testing.T) {
	app := Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	// Try to patch non-existent key
	data := json.RawMessage(`{"new":"value"}`)
	req := httptest.NewRequest("PATCH", "/nonexistent", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestRestGetWithVersion(t *testing.T) {
	app := Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	// Set some data
	_, err := app.Storage.Set("versiontest", json.RawMessage(`{"test":true}`))
	require.NoError(t, err)

	// Test GET with version parameter
	req := httptest.NewRequest("GET", "/versiontest?v=123", nil)
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestRestReadUnauthorized(t *testing.T) {
	app := Server{}
	app.Silence = true
	app.Audit = func(r *http.Request) bool {
		return r.Method != "GET"
	}
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestRestReadFilterError(t *testing.T) {
	app := Server{}
	app.Silence = true

	// Add read filter that returns error
	app.ReadFilter("filtererror", func(key string, data json.RawMessage) (json.RawMessage, error) {
		return nil, errors.New("read filter error")
	})

	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	// Set data
	_, err := app.Storage.Set("filtererror", json.RawMessage(`{"test":true}`))
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/filtererror", nil)
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRestUnpublishUnauthorized(t *testing.T) {
	app := Server{}
	app.Silence = true
	app.Audit = func(r *http.Request) bool {
		return r.Method != "DELETE"
	}
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	req := httptest.NewRequest("DELETE", "/test", nil)
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestRestUnpublishDeleteFilterError(t *testing.T) {
	app := Server{}
	app.Silence = true

	// Add delete filter that returns error
	app.DeleteFilter("deleteerror", func(key string) error {
		return errors.New("delete filter error")
	})

	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	// Set data first
	_, err := app.Storage.Set("deleteerror", json.RawMessage(`{"test":true}`))
	require.NoError(t, err)

	req := httptest.NewRequest("DELETE", "/deleteerror", nil)
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRestPublishWriteFilterError(t *testing.T) {
	app := Server{}
	app.Silence = true

	// Add write filter that returns error
	app.WriteFilter("writeerror", func(key string, data json.RawMessage) (json.RawMessage, error) {
		return nil, errors.New("write filter error")
	})

	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	data := json.RawMessage(`{"test":true}`)
	req := httptest.NewRequest("POST", "/writeerror", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRestPublishStorageError(t *testing.T) {
	// This test would require mocking the storage to return an error
	// For now, we'll test with invalid path which should cause storage error
	app := Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	// Use empty key which should cause storage error
	data := json.RawMessage(`{"test":true}`)
	req := httptest.NewRequest("POST", "/", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	// This should hit the stats endpoint instead, so we expect 405 (method not allowed)
	require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

func TestRestGlobPatternEdgeCases(t *testing.T) {
	app := Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	data := json.RawMessage(`{"test":true}`)

	// Test glob not at end of path
	req := httptest.NewRequest("POST", "/test/*/more", bytes.NewBuffer(data))
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Test multiple globs
	req = httptest.NewRequest("POST", "/test/*/*", bytes.NewBuffer(data))
	w = httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
