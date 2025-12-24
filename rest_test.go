package ooo_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/benitogf/ooo"
	"github.com/benitogf/ooo/meta"
	"github.com/stretchr/testify/require"
)

func TestRestPostNonObject(t *testing.T) {
	// t.Parallel()
	app := ooo.Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)
	var jsonStr = []byte(`non object`)
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewBuffer(jsonStr))
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRestPostEmptyData(t *testing.T) {
	// t.Parallel()
	app := ooo.Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)
	var jsonStr = []byte(``)
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewBuffer(jsonStr))
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRestPostInvalidData(t *testing.T) {
	// t.Parallel()
	app := ooo.Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)
	var jsonStr = []byte(`oldkoskdasoejd`)
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewBuffer(jsonStr))
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRestPostKey(t *testing.T) {
	// t.Parallel()
	app := ooo.Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)
	var jsonStr = []byte(`{"data":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/test//a", bytes.NewBuffer(jsonStr))
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusMovedPermanently, resp.StatusCode)
}

func TestRestDel(t *testing.T) {
	// t.Parallel()
	app := ooo.Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)
	_ = app.Storage.Del("test")
	index, err := app.Storage.Set("test", ooo.TEST_DATA)
	require.NoError(t, err)
	require.Equal(t, "test", index)

	req := httptest.NewRequest("DELETE", "/test", nil)
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	data, _ := app.Storage.Get("test")
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.Empty(t, data)

	req = httptest.NewRequest("DELETE", "/test", nil)
	w = httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	index, err = app.Storage.Set("test/1", ooo.TEST_DATA)
	require.NoError(t, err)
	require.Equal(t, "1", index)
	index, err = app.Storage.Set("test/2", ooo.TEST_DATA_UPDATE)
	require.NoError(t, err)
	require.Equal(t, "2", index)

	req = httptest.NewRequest("DELETE", "/test/*", nil)
	w = httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	_, err = app.Storage.Get("test/1")
	require.Error(t, err)
	_, err = app.Storage.Get("test/2")
	require.Error(t, err)
}

func TestRestGet(t *testing.T) {
	// t.Parallel()
	app := ooo.Server{}
	app.Silence = true
	app.Start("localhost:0")
	app.Storage.Clear()
	defer app.Close(os.Interrupt)
	_ = app.Storage.Del("test")
	index, err := app.Storage.Set("test", ooo.TEST_DATA)
	require.NoError(t, err)
	require.Equal(t, "test", index)
	index, err = app.Storage.Set("sources", ooo.TEST_DATA_UPDATE)
	require.NoError(t, err)
	require.Equal(t, "sources", index)
	data, _ := app.Storage.Get("test")
	dataSources, _ := app.Storage.Get("sources")
	dataEncoded, _ := meta.Encode(data)
	dataSourcesEncoded, _ := meta.Encode(dataSources)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	require.Equal(t, string(dataEncoded), string(body))

	req = httptest.NewRequest(http.MethodGet, "/sources", nil)
	w = httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp = w.Result()
	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	require.Equal(t, string(dataSourcesEncoded), string(body))

	req = httptest.NewRequest(http.MethodGet, "/test/notest", nil)
	w = httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestRestStats(t *testing.T) {
	// t.Parallel()
	app := ooo.Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	index, err := app.Storage.Set("test/1", ooo.TEST_DATA)
	require.NoError(t, err)
	require.NotEmpty(t, index)

	req := httptest.NewRequest(http.MethodGet, "/?api=keys", nil)
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	require.Contains(t, string(body), "\"keys\":[\"test/1\"]")
	require.Contains(t, string(body), "\"total\":1")

	_ = app.Storage.Del("test/1")

	req = httptest.NewRequest(http.MethodGet, "/?api=keys", nil)
	w = httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp = w.Result()
	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	require.Contains(t, string(body), "\"keys\":[]")
	require.Contains(t, string(body), "\"total\":0")
}

func TestRestResponseCode(t *testing.T) {
	// t.Parallel()
	app := ooo.Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	index, err := app.Storage.Set("test", ooo.TEST_DATA)
	require.NoError(t, err)
	require.NotEmpty(t, index)

	index, err = app.Storage.Set("test/1", ooo.TEST_DATA_UPDATE)
	require.NoError(t, err)
	require.NotEmpty(t, index)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	w = httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	req = httptest.NewRequest(http.MethodGet, "/*", nil)
	w = httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	req = httptest.NewRequest(http.MethodDelete, "/test", nil)
	w = httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	req = httptest.NewRequest(http.MethodDelete, "/test/1", nil)
	w = httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	w = httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	req = httptest.NewRequest(http.MethodPatch, "/none/*", nil)
	w = httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRestGetBadRequest(t *testing.T) {
	// t.Parallel()
	app := ooo.Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)
	req := httptest.NewRequest(http.MethodGet, "//test", nil)
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()

	require.Equal(t, 301, resp.StatusCode)
}

func TestRestPostInvalidKey(t *testing.T) {
	// t.Parallel()
	app := ooo.Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)
	req := httptest.NewRequest(http.MethodPost, "/test/*/*", nil)
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRestGetInvalidKey(t *testing.T) {
	// t.Parallel()
	app := ooo.Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)
	req := httptest.NewRequest(http.MethodGet, "/test/*/**", nil)
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestRestDeleteInvalidKey(t *testing.T) {
	// t.Parallel()
	app := ooo.Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)
	req := httptest.NewRequest(http.MethodDelete, "/test/*/**", nil)
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestPatch(t *testing.T) {
	app := ooo.Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	var testInput = []byte(`{"one":"test"}`)
	var testUpdate = []byte(`{"two":"testing"}`)
	var testOutput = []byte(`{"one":"test","two":"testing"}`)
	index, err := app.Storage.Set("test/1", testInput)
	require.NoError(t, err)
	require.NotEmpty(t, index)

	req := httptest.NewRequest(http.MethodPatch, "/test/*", bytes.NewBuffer(testUpdate))
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	obj, err := app.Storage.Get("test/1")
	require.NoError(t, err)

	require.Equal(t, string(testOutput), string(obj.Data))
}
