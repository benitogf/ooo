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
	"sync/atomic"
	"testing"
	"time"

	"github.com/benitogf/go-json"
	"github.com/benitogf/ooo"
	"github.com/benitogf/ooo/ui"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// TestServerRouterConcurrentEndpointAndServeHTTP asserts that
// Server.Endpoint can run concurrently with request dispatch through
// the server's main Router. Pre-fix gorilla/mux.Router has internal
// state (its `routes` slice and per-route regex caches) that mutates
// inside HandleFunc and is read inside Match; with no external
// synchronization, parallel registration + ServeHTTP fires a data
// race. The route-oracle Router was fixed in PR #99 to take
// sync.RWMutex; the main server.Router needs the same discipline.
func TestServerRouterConcurrentEndpointAndServeHTTP(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	const writers = 6
	const readers = 6
	const iterations = 20

	barrier := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(writers + readers)

	for w := range writers {
		go func() {
			defer wg.Done()
			<-barrier
			for i := range iterations {
				path := fmt.Sprintf("/dyn-route-%d-%d", w, i)
				server.Endpoint(ooo.EndpointConfig{
					Path: path,
					Methods: ooo.Methods{
						http.MethodGet: {Response: nil},
					},
					Handler: func(rw http.ResponseWriter, _ *http.Request) {
						rw.WriteHeader(http.StatusOK)
					},
				})
			}
		}()
	}

	addr := "http://" + server.Address
	client := &http.Client{Timeout: 5 * time.Second}
	for range readers {
		go func() {
			defer wg.Done()
			<-barrier
			for range iterations {
				// Use the real HTTP server so requests go through the
				// production handler chain — syncRouter wraps the mux
				// router only at server.server.Handler. Direct calls
				// to server.Router.ServeHTTP would bypass the wrapper.
				resp, err := client.Get(addr + "/some-key")
				if err == nil {
					resp.Body.Close()
				}
			}
		}()
	}

	close(barrier)
	wg.Wait()
}

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

// denyAllMiddleware returns a middleware that rejects every request
// with 401 Unauthorized. Used by tests that assert a registered
// middleware actually gates the matched handler.
func denyAllMiddleware() mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		})
	}
}

// TestMiddlewareGatesEndpoint asserts that a custom Endpoint
// registered via Server.Endpoint runs inside the middleware chain
// registered via Router.Use(). Pre-refactor the Endpoint handler was
// wrapped in an opinionated AuditHandler that consulted a separate
// Server.Audit hook; now the gorilla/mux middleware chain is the
// single extension point and the Endpoint handler is registered
// naked.
func TestMiddlewareGatesEndpoint(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.Router = mux.NewRouter()
	server.Router.Use(denyAllMiddleware())
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	var handlerRan atomic.Bool
	server.Endpoint(ooo.EndpointConfig{
		Path: "/custom",
		Methods: ooo.Methods{
			http.MethodGet: {Response: nil},
		},
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			handlerRan.Store(true)
			w.WriteHeader(http.StatusOK)
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/custom", nil)
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Result().StatusCode,
		"middleware returning 401 must reject custom Endpoint requests")
	require.False(t, handlerRan.Load(),
		"Endpoint handler must not run when middleware denies")
}

// TestMiddleware exercises Router.Use() middleware across every
// request type the server dispatches: storage REST (GET, POST,
// DELETE), the explorer root, and a WebSocket subscription upgrade.
// All must be gated by the same chain.
func TestMiddleware(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.Router = mux.NewRouter()

	// gate is swapped between phases of the test; the middleware
	// closes over the atomic pointer so changes take effect on the
	// next request without re-registering.
	var gate atomic.Pointer[func(*http.Request) bool]
	initial := func(r *http.Request) bool {
		return r.Header.Get("Upgrade") != "websocket" && r.Method != "GET" && r.Method != "DELETE"
	}
	gate.Store(&initial)
	server.Router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fn := gate.Load()
			if fn != nil && !(*fn)(r) {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	})

	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	index, err := server.Storage.Set("test", json.RawMessage(`{"test": "123"}`))
	require.NoError(t, err)
	require.Equal(t, "test", index)

	// GET /test → denied
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Result().StatusCode)

	// GET / (explorer root) → denied
	req = httptest.NewRequest("GET", "/", nil)
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Result().StatusCode)

	// DELETE /test → denied
	req = httptest.NewRequest("DELETE", "/test", nil)
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Result().StatusCode)

	// WebSocket subscribe → upgrade refused (middleware returns 401
	// before the handler can hijack).
	u := url.URL{Scheme: "ws", Host: server.Address, Path: "/sa/test"}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.Nil(t, c)
	server.Console.Err(err)
	require.Error(t, err)

	// Flip the gate: only GET succeeds.
	getOnly := func(r *http.Request) bool { return r.Method == "GET" }
	gate.Store(&getOnly)

	jsonStr := []byte(`{"data":"test"}`)
	req = httptest.NewRequest("POST", "/test", bytes.NewBuffer(jsonStr))
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Result().StatusCode)

	req = httptest.NewRequest("GET", "/test", nil)
	w = httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Result().StatusCode)

	// Flip again: WebSocket explicitly denied.
	noWS := func(r *http.Request) bool {
		return r.Header.Get("Upgrade") != "websocket"
	}
	gate.Store(&noWS)

	u = url.URL{Scheme: "ws", Host: server.Address, Path: "/"}
	c, _, err = websocket.DefaultDialer.Dial(u.String(), nil)
	require.Nil(t, c)
	server.Console.Err(err)
	require.Error(t, err)
}
