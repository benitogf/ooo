package ooo_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime"
	"sync"
	"testing"

	"github.com/benitogf/go-json"
	"github.com/benitogf/ooo"
	"github.com/benitogf/ooo/ui"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// TestRegisterProxyConcurrentWithRead asserts that RegisterProxy
// can run concurrently with the explorer UI's GetProxies callback
// (and the equivalent Endpoint-slice read via GetEndpoints). Pre-fix
// the endpoints + proxies slices were appended without any lock and
// read without any lock, so under -race a parallel registration + UI
// fetch fired a data race. Sibling registrars RegisterProxyCleanup /
// RegisterPreClose already used mutexes for the same pattern; this
// test pins that the endpoint + proxy registries do too.
//
// The test exercises RegisterProxy specifically (no mux.Router
// mutation) so the assertion isolates the registry-slice race the
// audit item flagged. Concurrent Endpoint() registrations have a
// separate race in gorilla/mux's Router.HandleFunc vs Router.Match
// — tracked as its own audit-list item.
func TestRegisterProxyConcurrentWithRead(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Register one Endpoint up front so the UI read path also iterates
	// over a non-empty endpoints slice during the concurrent section.
	server.Endpoint(ooo.EndpointConfig{
		Path: "/seed",
		Methods: ooo.Methods{
			http.MethodGet: {Response: nil},
		},
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	})

	const writers = 8
	const readers = 8
	const iterations = 25

	barrier := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(writers + readers)

	for w := range writers {
		go func() {
			defer wg.Done()
			<-barrier
			for i := range iterations {
				server.RegisterProxy(ui.ProxyInfo{
					LocalPath: fmt.Sprintf("dyn-proxy-%d-%d", w, i),
				})
			}
		}()
	}

	// The explorer UI's read paths are GET /?api=endpoints and
	// /?api=proxies (see ui/ui.go: `switch r.URL.Query().Get("api")`).
	// A bare GET / serves the static index and never enters
	// getEndpoints/getProxies, so the readers must explicitly request
	// each API to exercise both halves of the fix.
	for r := range readers {
		api := "endpoints"
		if r%2 == 1 {
			api = "proxies"
		}
		go func(api string) {
			defer wg.Done()
			<-barrier
			for range iterations {
				req := httptest.NewRequest(http.MethodGet, "/?api="+api, nil)
				rec := httptest.NewRecorder()
				server.Router.ServeHTTP(rec, req)
				_ = rec.Result()
			}
		}(api)
	}

	close(barrier)
	wg.Wait()
}

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
