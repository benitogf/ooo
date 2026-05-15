package ooo_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime"
	"testing"

	"github.com/benitogf/go-json"
	"github.com/benitogf/ooo"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// TestAuditGatesEndpoint asserts that a custom Endpoint registered
// via Server.Endpoint honors Server.Audit. Pre-fix the endpoint
// handler was registered directly on Router with no Audit wrapping,
// so a deny-everything Audit had no effect — custom endpoint paths
// silently bypassed auth/rate-limiting/observability gates that
// every REST handler already respects.
func TestAuditGatesEndpoint(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.Audit = func(r *http.Request) bool { return false }
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	var handlerRan bool
	server.Endpoint(ooo.EndpointConfig{
		Path: "/custom",
		Methods: ooo.Methods{
			http.MethodGet: {Response: nil},
		},
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			handlerRan = true
			w.WriteHeader(http.StatusOK)
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/custom", nil)
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Result().StatusCode,
		"Audit returning false must reject custom Endpoint requests")
	require.False(t, handlerRan, "Endpoint handler must not run when Audit denies")
}

func TestAudit(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.Audit = func(r *http.Request) bool {
		return r.Header.Get("Upgrade") != "websocket" && r.Method != "GET" && r.Method != "DELETE"
	}
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	index, err := server.Storage.Set("test", json.RawMessage(`{"test": "123"}`))
	require.NoError(t, err)
	require.Equal(t, "test", index)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, 401, resp.StatusCode)

	req = httptest.NewRequest("GET", "/", nil)
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, 401, resp.StatusCode)

	req = httptest.NewRequest("DELETE", "/test", nil)
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, 401, resp.StatusCode)

	u := url.URL{Scheme: "ws", Host: server.Address, Path: "/sa/test"}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.Nil(t, c)
	server.Console.Err(err)
	require.Error(t, err)

	server.Audit = func(r *http.Request) bool {
		return r.Method == "GET"
	}

	var jsonStr = []byte(`{"data":"test"}`)
	req = httptest.NewRequest("POST", "/test", bytes.NewBuffer(jsonStr))
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, 401, resp.StatusCode)

	req = httptest.NewRequest("GET", "/test", nil)
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp = w.Result()
	require.Equal(t, 200, resp.StatusCode)

	server.Audit = func(r *http.Request) bool {
		return r.Header.Get("Upgrade") != "websocket"
	}

	u = url.URL{Scheme: "ws", Host: server.Address, Path: "/"}
	c, _, err = websocket.DefaultDialer.Dial(u.String(), nil)
	require.Nil(t, c)
	server.Console.Err(err)
	require.Error(t, err)
}
