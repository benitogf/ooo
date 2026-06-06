package ooo

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/benitogf/coat"
	"github.com/benitogf/ooo/filters"
	"github.com/benitogf/ooo/key"
	"github.com/benitogf/ooo/meta"
	"github.com/benitogf/ooo/monotonic"
	"github.com/benitogf/ooo/storage"
	"github.com/benitogf/ooo/stream"
	"github.com/benitogf/ooo/ui"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/rs/cors"
)

const deadlineMsg = "ooo: server deadline reached"

// syncRouter wraps Server.Router so the gorilla/mux internal routes
// slice (mutated by HandleFunc/Handle, read by Match) cannot race
// with concurrent request dispatch. ServeHTTP takes the routerMu
// read lock around Match only — the dispatched handler runs WITHOUT
// the lock so long-running handlers (WebSocket forwarders, hijacked
// connections) do not block subsequent registrations.
//
// mux.SetURLVars propagates the matched vars into the request
// context so handlers continue to read them via mux.Vars(r). This
// covers every consumer in the tree (REST, WS, proxy); none of them
// use mux.CurrentRoute, which gorilla/mux does not expose a public
// setter for.
//
// Known deviations from mux.Router.ServeHTTP:
//   - No path cleaning + 301 redirect for non-canonical paths
//     (mux does this when SkipClean(true) has NOT been set). The
//     codebase does not depend on it. Add a port of mux's cleanPath
//     here if a future consumer needs it.
//   - mux Router.Use() middlewares are not fanned out. The codebase
//     does not register any. If middleware support is added later,
//     this wrapper must chain them onto the matched handler.
type syncRouter struct {
	router *mux.Router
	mu     *sync.RWMutex
}

func (s *syncRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	var match mux.RouteMatch
	matched := s.router.Match(r, &match)
	s.mu.RUnlock()

	var handler http.Handler
	if matched {
		handler = match.Handler
		// Match mux's unconditional vars propagation — handlers
		// distinguish "no vars" from "vars never set" by reading
		// the context, so don't gate on len.
		r = mux.SetURLVars(r, match.Vars)
	}
	if handler == nil && match.MatchErr == mux.ErrMethodMismatch {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	if handler == nil {
		http.NotFound(w, r)
		return
	}
	handler.ServeHTTP(w, r)
}

// RouterMutate runs fn while holding the routerMu write lock so
// Server.Router mutations (HandleFunc, Handle, MatcherFunc chaining)
// serialize with the syncRouter wrapper's Match. Used by Endpoint,
// setupRoutes, and the proxy package's Route/RouteList/RouteWithVars
// registrars.
func (server *Server) RouterMutate(fn func()) {
	server.routerMu.Lock()
	defer server.routerMu.Unlock()
	fn()
}

// Server.active state machine. The intermediate serverStarting value
// is what makes Active() return false during the listen-bind window
// while still keeping concurrent StartWithError callers off the
// reallocation path via the CAS in StartWithError.
const (
	serverInactive int64 = 0
	serverActive   int64 = 1
	serverStarting int64 = 2
)

// DefaultMaxRequestBodyBytes is the default cap on REST request body size
// (POST / PATCH). Override via Server.MaxRequestBodyBytes. Set the field to a
// negative value to disable the cap.
const DefaultMaxRequestBodyBytes = 10 * 1024 * 1024 // 10 MiB

// audit requests function
// will define approval or denial by the return value
// r: the request to be audited
// returns
// true: approve the request
// false: rejects the request
type audit func(r *http.Request) bool

// CloseHookPhase identifies when a teardown hook runs during Server.Close.
// Phases run in declaration order; multiple hooks at the same phase run
// in registration order. See RegisterCloseHook.
type CloseHookPhase int

const (
	// PreShutdown runs first, before any internal teardown. Storage,
	// stream, and HTTP are still up — hooks may broadcast, read, or
	// write. Use this to flush in-memory state or notify subscribers
	// that the server is shutting down.
	PreShutdown CloseHookPhase = iota
	// ProxyTeardown runs after PreShutdown but before stream
	// connections are closed. Use this to unsubscribe from upstream
	// proxy servers while the stream layer is still available.
	ProxyTeardown
	// PostShutdown runs last, after every internal teardown step.
	// Storage, stream, and HTTP are all torn down — use this for
	// closing user-owned resources (DB pools, log handles). The
	// deprecated Server.OnClose field, if set, runs after every
	// PostShutdown hook.
	PostShutdown
	closeHookPhaseCount
)

// Server is the main application struct for the ooo server.
//
// Name: display name for the server, shown in the storage explorer title
//
// Router: can be predefined with routes and passed to be extended
//
// Stream: manages WebSocket connections and broadcasts
//
// NoBroadcastKeys: array of keys that should not broadcast on changes
//
// Audit: function to audit requests, returns true to approve, false to deny
//
// Workers: number of workers to use as readers of the storage->broadcast channel
//
// ForcePatch: flag to force patch operations even if the patch is bigger than the snapshot
//
// NoPatch: flag to disable patch operations entirely, always send full snapshots
//
// OnSubscribe: function to monitor subscribe events, can return error to deny subscription
//
// OnUnsubscribe: function to monitor unsubscribe events
//
// OnStart: function that triggers after the server has started successfully
//
// OnClose: function that triggers after closing the application
//
// Deadline: time duration of a request before timing out
//
// AllowedOrigins: list of allowed origins for cross domain access, defaults to ["*"]
//
// AllowedMethods: list of allowed methods for cross domain access, defaults to ["GET", "POST", "DELETE", "PUT", "PATCH"]
//
// AllowedHeaders: list of allowed headers for cross domain access, defaults to ["Authorization", "Content-Type"]
//
// ExposedHeaders: list of exposed headers for cross domain access, defaults to nil
//
// Storage: database interface implementation
//
// Address: the address the server is listening on (populated after Start)
//
// Silence: output silence flag, suppresses console output when true
//
// Static: static routing flag, when true only filtered routes are allowed
//
// Tick: time interval between ticks on the clock websocket
//
// Console: logging console for the server
//
// Signal: os signal channel for graceful shutdown
//
// Client: http client to make requests
//
// ReadTimeout: maximum duration for reading the entire request
//
// WriteTimeout: maximum duration before timing out writes of the response
//
// ReadHeaderTimeout: amount of time allowed to read request headers
//
// IdleTimeout: maximum amount of time to wait for the next request
//
// OnStorageEvent: callback function triggered on storage events
//
// BeforeRead: callback function triggered before read operations
//
// AfterWrite: callback function triggered after a successful write
// (Set / Push / Patch / Del). Wired once at Start; further changes
// have no effect.
type Server struct {
	wg           sync.WaitGroup
	watchWg      sync.WaitGroup
	listenWg     sync.WaitGroup
	handlerWg    sync.WaitGroup
	clockWg      sync.WaitGroup
	server       *http.Server
	Name         string
	Router       *mux.Router
	Stream       stream.Stream
	filters      filters.Filters
	limitFilters map[string]*limitFilterReg // tracks limit filter registrations
	endpoints    []ui.EndpointInfo          // registered custom endpoints
	proxies      []ui.ProxyInfo             // registered proxy routes
	// registryMu protects endpoints + proxies. Sibling cleanup-slice
	// registrars (RegisterProxyCleanup, RegisterPreClose) already
	// lock; these used to mutate without one and raced against
	// getEndpoints/getProxies from the explorer UI hot path.
	registryMu sync.RWMutex
	// routerMu protects Server.Router. gorilla/mux.Router mutates an
	// internal routes slice on HandleFunc/Handle and reads it on
	// Match; without serialization a registration concurrent with
	// request dispatch races on those internals. Mutators take Lock
	// via RouterMutate; the syncRouter wrapper installed in
	// waitListen takes RLock around Match, then dispatches the
	// matched handler without the lock so long-running handlers do
	// not block subsequent registrations.
	routerMu        sync.RWMutex
	routeOracle     *mux.Router                   // mirrors Endpoint/Proxy paths so the data wildcard can defer to them
	routeOracleMu   sync.RWMutex                  // protects routeOracle; readers (routeOracleSkip) take RLock on the data-wildcard hot path, registrations take Lock
	closeHooks      [closeHookPhaseCount][]func() // teardown hooks indexed by CloseHookPhase
	closeHooksMu    sync.Mutex                    // protects closeHooks
	NoBroadcastKeys []string
	Audit           audit
	Workers         int
	ForcePatch      bool
	NoPatch         bool
	OnSubscribe     stream.Subscribe
	OnUnsubscribe   stream.Unsubscribe
	OnStart         func()
	// OnClose runs as the final teardown step, after every PostShutdown
	// hook. Deprecated: use RegisterCloseHook(PostShutdown, fn) instead;
	// the field is kept for backwards compatibility and runs last so
	// existing callers see no behaviour change.
	OnClose func()
	// CloseCallbackBudget caps the aggregate runtime of user-supplied
	// teardown hooks registered via RegisterCloseHook (and the
	// deprecated RegisterPreClose / RegisterProxyCleanup / OnClose
	// surface) during Close. Only the time spent INSIDE hooks
	// counts against this budget — the wall clock spent in ooo's own
	// internal teardown (Deadline-bounded HTTP drain, Storage.Close,
	// waitgroup joins) is not charged. A callback already running is
	// not interrupted, but once the cumulative callback-runtime
	// exceeds this budget, subsequent callbacks are skipped and a
	// one-line warning is logged via the server's Console. Zero (the
	// default) means no bound — the pre-existing contract where every
	// callback runs to completion regardless of duration. Opt in by
	// setting a positive value, for example to keep the
	// SIGTERM-to-SIGKILL window from being exhausted by a single
	// misbehaving callback.
	CloseCallbackBudget time.Duration
	Deadline            time.Duration
	AllowedOrigins      []string
	AllowedMethods      []string
	AllowedHeaders      []string
	ExposedHeaders      []string
	Storage             storage.Database
	Address             string
	closing             int64
	// active follows the serverInactive/serverStarting/serverActive
	// state machine so Active() can distinguish "Start has been called
	// but the listener has not yet bound" from "listener bound and
	// serving". See StartWithError for the CAS that claims the slot.
	active  int64
	Silence bool
	Static  bool
	Tick    time.Duration
	Console *coat.Console
	Signal  chan os.Signal
	Client  *http.Client
	// CertFile and KeyFile, when both non-empty, switch the listener
	// from plaintext HTTP to HTTPS. The keypair is loaded once at Start;
	// a load failure aborts startup with an error rather than serving
	// plaintext. Leave both empty to serve plain HTTP (the default).
	CertFile string
	KeyFile  string
	// TLSConfig, when non-nil, is the base TLS configuration for the
	// HTTPS listener (min version, cipher suites, client-auth, ...). The
	// keypair loaded from CertFile/KeyFile is appended to its
	// Certificates. When nil a minimal modern default (TLS 1.2 floor) is
	// used. Ignored when CertFile/KeyFile are empty.
	TLSConfig           *tls.Config
	ReadTimeout         time.Duration
	WriteTimeout        time.Duration
	ReadHeaderTimeout   time.Duration
	IdleTimeout         time.Duration
	MaxRequestBodyBytes int64 // cap on REST request body size; defaults to DefaultMaxRequestBodyBytes (10 MiB). Set to a negative value to disable.
	OnStorageEvent      storage.EventCallback
	OnWatchPanic        func(ev storage.Event, r any) // optional: invoked on each recovered watch-goroutine panic with the offending event
	OnDroppedEvent      func(ev storage.Event)        // optional: invoked when the sharded watcher channel drops an event after timing out
	BeforeRead          func(key string)
	AfterWrite          func(key string)
	GetPivotInfo        func() *ui.PivotInfo // Optional: returns pivot status for UI
	NoCompress          bool                 // Disable gzip compression (useful for tests)
	WatchPanics         int64                // Atomic counter of panics recovered in watch goroutines
	DroppedEvents       int64                // Atomic counter of events dropped by the sharded watcher on send timeout
	startErr            chan error           // channel for startup errors
	clockStop           chan struct{}        // channel to signal clock goroutine to stop
	consoleAutoBuilt    bool                 // true when defaults() built Console (so post-Listen rebuild can swap address without clobbering a user-supplied Console)
}

// Validate checks the server configuration for common issues.
// Call this before Start() to catch configuration errors early.
func (server *Server) Validate() error {
	if server.ForcePatch && server.NoPatch {
		return ErrForcePatchConflict
	}
	if server.Workers < 0 {
		return ErrNegativeWorkers
	}
	if server.Deadline < 0 {
		return ErrNegativeDeadline
	}
	return nil
}

// getServerInfo returns server configuration for the explorer
func (server *Server) getServerInfo() ui.ServerInfo {
	return ui.ServerInfo{
		Name:              server.Name,
		Address:           server.Address,
		Deadline:          server.Deadline,
		ReadTimeout:       server.ReadTimeout,
		WriteTimeout:      server.WriteTimeout,
		ReadHeaderTimeout: server.ReadHeaderTimeout,
		IdleTimeout:       server.IdleTimeout,
		ForcePatch:        server.ForcePatch,
		NoPatch:           server.NoPatch,
		Static:            server.Static,
		Silence:           server.Silence,
		Workers:           server.Workers,
		Tick:              server.Tick,
		WatchPanics:       atomic.LoadInt64(&server.WatchPanics),
		DroppedEvents:     atomic.LoadInt64(&server.DroppedEvents),
	}
}

// pivotPrefix is the prefix used for internal pivot synchronization keys
const pivotPrefix = "pivot"

// isPivotPath checks if a path is a pivot internal path.
// A path is internal only when it equals the bare prefix or sits under it
// as a path segment (pivot/...); a user key like "pivothings" is not.
func isPivotPath(path string) bool {
	return path == pivotPrefix || strings.HasPrefix(path, pivotPrefix+"/")
}

// limitFilterReg stores a limit filter registration for lazy evaluation
type limitFilterReg struct {
	filter      *filters.LimitFilter
	description string
	schema      map[string]any
}

// getLimitFiltersInfo returns current limit filter info, evaluating dynamic limits lazily
func (server *Server) getLimitFiltersInfo() map[string]filters.LimitFilterInfo {
	result := make(map[string]filters.LimitFilterInfo, len(server.limitFilters))
	for path, reg := range server.limitFilters {
		order := "desc"
		if reg.filter.Order() == filters.OrderAsc {
			order = "asc"
		}
		info := filters.LimitFilterInfo{
			Limit:        reg.filter.Limit(),
			LimitDynamic: reg.filter.IsDynamic(),
			Order:        order,
			Description:  reg.description,
			Schema:       reg.schema,
		}
		if reg.filter.HasMaxAge() {
			info.MaxAge = reg.filter.MaxAge().String()
			info.MaxAgeDynamic = reg.filter.IsDynamic()
		}
		result[path] = info
	}
	return result
}

// getFiltersInfo returns detailed filter information for the explorer
// Filters out pivot-prefixed paths as they are internal use only
func (server *Server) getFiltersInfo() []ui.FilterInfo {
	filtersInfo := server.filters.PathsInfo(server.getLimitFiltersInfo())
	result := make([]ui.FilterInfo, 0, len(filtersInfo))
	for _, f := range filtersInfo {
		// Skip pivot-prefixed paths (internal use only)
		if isPivotPath(f.Path) {
			continue
		}
		result = append(result, ui.FilterInfo{
			Path:            f.Path,
			Type:            f.Type,
			CanRead:         f.CanRead,
			CanWrite:        f.CanWrite,
			CanDelete:       f.CanDelete,
			IsGlob:          f.IsGlob,
			Limit:           f.Limit,
			LimitDynamic:    f.LimitDynamic,
			MaxAge:          f.MaxAge,
			MaxAgeDynamic:   f.MaxAgeDynamic,
			Order:           f.Order,
			CleanupEnabled:  f.CleanupEnabled,
			CleanupInterval: f.CleanupInterval,
			DescWrite:       f.DescWrite,
			DescRead:        f.DescRead,
			DescDelete:      f.DescDelete,
			DescAfterWrite:  f.DescAfterWrite,
			DescLimit:       f.DescLimit,
			Schema:          f.Schema,
		})
	}
	return result
}

// RegisterLimitFilter registers a limit filter and tracks it for the ui.
// The LimitFilter should already be created and its filters added to the server.
// This method stores a reference to the filter for lazy evaluation of dynamic limits.
func (server *Server) RegisterLimitFilter(lf *filters.LimitFilter, description string, schema map[string]any) {
	if server.limitFilters == nil {
		server.limitFilters = make(map[string]*limitFilterReg)
	}
	server.limitFilters[lf.Path()] = &limitFilterReg{
		filter:      lf,
		description: description,
		schema:      schema,
	}
}

// getStreamState returns stream connection pool information for the explorer
func (server *Server) getStreamState() []ui.PoolInfo {
	state := server.Stream.GetState()
	result := make([]ui.PoolInfo, len(state))
	for i, p := range state {
		result[i] = ui.PoolInfo{
			Key:         p.Key,
			Connections: p.Connections,
		}
	}
	return result
}

// tcpKeepAliveListener sets TCP keep-alive timeouts on accepted
// connections. It's used by ListenAndServe and ListenAndServeTLS so
// dead TCP connections (e.g. closing laptop mid-download) eventually
// go away.
type tcpKeepAliveListener struct {
	*net.TCPListener
}

func (server *Server) waitListen() {
	defer server.listenWg.Done()
	var err error
	storageOpt := storage.Options{
		NoBroadcastKeys: server.NoBroadcastKeys,
		Workers:         server.Workers,
	}

	if server.BeforeRead != nil {
		storageOpt.BeforeRead = server.BeforeRead
	}
	if server.AfterWrite != nil {
		storageOpt.AfterWrite = server.AfterWrite
	}
	err = server.Storage.Start(storageOpt)
	if err != nil {
		server.startErr <- fmt.Errorf("ooo: storage start failed: %w", err)
		server.wg.Done()
		return
	}
	// Wrap server.Router so concurrent Endpoint/Proxy registrations
	// cannot race with the request-dispatch Match. The wrapper takes
	// the routerMu read lock around Match and dispatches the matched
	// handler without holding the lock.
	var handler http.Handler = &syncRouter{router: server.Router, mu: &server.routerMu}
	if !server.NoCompress {
		handler = handlers.CompressHandler(handler)
	}
	server.server = &http.Server{
		WriteTimeout:      server.WriteTimeout,
		ReadTimeout:       server.ReadTimeout,
		ReadHeaderTimeout: server.ReadHeaderTimeout,
		IdleTimeout:       server.IdleTimeout,
		Addr:              server.Address,
		Handler: cors.New(cors.Options{
			AllowedMethods: server.AllowedMethods,
			AllowedOrigins: server.AllowedOrigins,
			AllowedHeaders: server.AllowedHeaders,
			ExposedHeaders: server.ExposedHeaders,
			// AllowCredentials: true,
			// Debug:          true,
		}).Handler(handler)}
	// When a keypair is configured the server serves HTTPS. Load it
	// before binding so a bad cert/key aborts startup with an error
	// instead of falling through to a plaintext listener.
	tlsEnabled := server.CertFile != "" && server.KeyFile != ""
	if tlsEnabled {
		cert, certErr := tls.LoadX509KeyPair(server.CertFile, server.KeyFile)
		if certErr != nil {
			atomic.StoreInt64(&server.active, serverInactive)
			server.startErr <- fmt.Errorf("ooo: failed to load TLS keypair: %w", certErr)
			server.wg.Done()
			return
		}
		tlsConf := server.TLSConfig
		if tlsConf == nil {
			tlsConf = &tls.Config{}
		} else {
			tlsConf = tlsConf.Clone()
		}
		if tlsConf.MinVersion == 0 {
			tlsConf.MinVersion = tls.VersionTLS12
		}
		tlsConf.Certificates = append(tlsConf.Certificates, cert)
		server.server.TLSConfig = tlsConf
	}
	ln, err := net.Listen("tcp4", server.Address)
	if err != nil {
		// Roll the CAS claim back so a retry (or Start-after-Close)
		// sees the slot free. Must happen before wg.Done() so the
		// parent observing wg.Wait() never sees the intermediate
		// serverStarting value.
		atomic.StoreInt64(&server.active, serverInactive)
		server.startErr <- fmt.Errorf("ooo: failed to start tcp: %w", err)
		server.wg.Done()
		return
	}
	server.Address = ln.Addr().String()
	atomic.StoreInt64(&server.active, serverActive)
	server.wg.Done()
	keepAliveLn := tcpKeepAliveListener{ln.(*net.TCPListener)}
	if tlsEnabled {
		// Cert/key already loaded into server.server.TLSConfig above, so
		// pass empty filenames — ServeTLS uses the configured keypair.
		err = server.server.ServeTLS(keepAliveLn, "", "")
	} else {
		err = server.server.Serve(keepAliveLn)
	}
	if atomic.LoadInt64(&server.closing) != 1 && err != nil {
		server.Console.Err("server error", err)
	}
}

// Active reports whether the server is fully running — listener bound,
// not yet shutting down. Returns false during the listen-bind window
// (Start has been called but the listener has not yet been accepted
// by waitListen) and false once Close has been called.
func (server *Server) Active() bool {
	return atomic.LoadInt64(&server.active) == serverActive && atomic.LoadInt64(&server.closing) == 0
}

func (server *Server) waitStart() error {
	// Check for startup errors from waitListen
	select {
	case err := <-server.startErr:
		return err
	default:
	}

	if atomic.LoadInt64(&server.active) != serverActive || !server.Storage.Active() {
		return ErrServerStartFailed
	}

	// Start workers for sharded watcher (per-key ordering)
	shardedWatcher := server.Storage.WatchSharded()
	shardedWatcher.SetOnDrop(server.handleDroppedEvent)
	for i := range shardedWatcher.Count() {
		server.watchWg.Add(1)
		go server.watch(shardedWatcher.Shard(i))
	}

	server.Console.Log("glad to serve[" + server.Address + "]")
	return nil
}

// FetchResult holds the result of a fetch operation for initial WebSocket message
type FetchResult struct {
	Data    []byte
	Version int64
}

// Fetch data, update cache and apply filter
func (server *Server) fetch(path string) (FetchResult, error) {
	// Check if any filter exists for static mode validation
	if server.Static {
		hasFilter := server.filters.ReadObject.HasMatch(path) != -1 ||
			server.filters.ReadList.HasMatch(path) != -1
		if !hasFilter {
			return FetchResult{}, filters.ErrRouteNotDefined
		}
	}

	if key.HasGlob(path) {
		// List subscription - use descending order (newest first)
		objs, err := server.Storage.GetListDescending(path)
		if err != nil {
			return FetchResult{}, err
		}
		filtered, err := server.filters.ReadList.Check(path, objs, server.Static)
		if err != nil {
			return FetchResult{}, err
		}
		// Initialize decoded cache (creates pool if needed)
		version := server.Stream.InitCacheObjectsWithVersion(path, filtered)
		// Encode for sending
		data, err := meta.Encode(filtered)
		if err != nil {
			return FetchResult{}, err
		}
		return FetchResult{Data: data, Version: version}, nil
	}

	// Single object subscription
	obj, err := server.Storage.Get(path)
	if err != nil {
		// Object not found - return empty object
		obj = meta.Object{}
	}
	filtered, err := server.filters.ReadObject.CheckWithListFallback(path, obj, server.Static, server.filters.ReadList)
	if err != nil {
		return FetchResult{}, err
	}
	// Initialize decoded cache (creates pool if needed)
	version := server.Stream.InitCacheObjectWithVersion(path, &filtered)
	// Encode for sending
	var data []byte
	if filtered.Created == 0 && filtered.Index == "" {
		data = meta.EmptyObject
	} else {
		data, err = meta.Encode(filtered)
		if err != nil {
			return FetchResult{}, err
		}
	}
	return FetchResult{Data: data, Version: version}, nil
}

func (server *Server) watch(sc storage.StorageChan) {
	defer server.watchWg.Done()
	for {
		ev, ok := <-sc
		if !ok {
			// Channel closed
			break
		}
		server.processEvent(ev)
		if !server.Storage.Active() {
			break
		}
	}
}

// handleDroppedEvent runs on the storage layer's sender goroutine each time
// the sharded watcher channel drops an event after exhausting its send
// timeout. The dropped event has been durably committed but no subscriber
// will ever see it — operators monitor DroppedEvents to detect a stuck or
// slow watcher consumer.
func (server *Server) handleDroppedEvent(ev storage.Event) {
	atomic.AddInt64(&server.DroppedEvents, 1)
	server.Console.Err(fmt.Sprintf("watch:dropped key=%q op=%q total=%d (consumer stuck or slow; subscribers will not see this event)",
		ev.Key, ev.Operation, atomic.LoadInt64(&server.DroppedEvents)))
	if server.OnDroppedEvent != nil {
		server.OnDroppedEvent(ev)
	}
}

func (server *Server) processEvent(ev storage.Event) {
	defer func() {
		if r := recover(); r != nil {
			atomic.AddInt64(&server.WatchPanics, 1)
			server.Console.Err(fmt.Sprintf("watch:panic recovered key=%q op=%q total=%d: %v",
				ev.Key, ev.Operation, atomic.LoadInt64(&server.WatchPanics), r))
			if server.OnWatchPanic != nil {
				server.OnWatchPanic(ev, r)
			}
		}
	}()
	if ev.Key == "" {
		return
	}
	server.Console.Log("broadcast[" + ev.Key + "]")
	server.Stream.Broadcast(ev.Key, stream.BroadcastOpt{
		Key:       ev.Key,
		Operation: ev.Operation,
		Object:    ev.Object,
		FilterObject: func(key string, obj meta.Object) (meta.Object, error) {
			return server.filters.ReadObject.CheckWithListFallback(key, obj, server.Static, server.filters.ReadList)
		},
		FilterList: func(key string, objs []meta.Object) ([]meta.Object, error) {
			return server.filters.ReadList.Check(key, objs, server.Static)
		},
		Static: server.Static,
	})
	if server.OnStorageEvent != nil {
		server.OnStorageEvent(ev)
	}
}

// defaultCORS sets default CORS configuration.
func (server *Server) defaultCORS() {
	if len(server.AllowedOrigins) == 0 {
		server.AllowedOrigins = []string{"*"}
	}
	if len(server.AllowedMethods) == 0 {
		server.AllowedMethods = []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodDelete,
			http.MethodPut,
			http.MethodPatch,
		}
	}
	if len(server.AllowedHeaders) == 0 {
		server.AllowedHeaders = []string{"Authorization", "Content-Type"}
	}
}

// defaultTimeouts sets default timeout values.
func (server *Server) defaultTimeouts() {
	if server.Deadline.Nanoseconds() == 0 {
		server.Deadline = time.Second * 10
	}
	if server.Tick == 0 {
		server.Tick = 1 * time.Second
	}
	if server.ReadTimeout == 0 {
		server.ReadTimeout = 1 * time.Minute
	}
	if server.WriteTimeout == 0 {
		server.WriteTimeout = 1 * time.Minute
	}
	if server.ReadHeaderTimeout == 0 {
		server.ReadHeaderTimeout = 10 * time.Second
	}
	if server.IdleTimeout == 0 {
		server.IdleTimeout = 10 * time.Second
	}
	if server.MaxRequestBodyBytes == 0 {
		server.MaxRequestBodyBytes = DefaultMaxRequestBodyBytes
	}
}

// defaultCallbacks sets default callback functions.
func (server *Server) defaultCallbacks() {
	if server.OnStart == nil {
		server.OnStart = func() {}
	}
	if server.OnClose == nil {
		server.OnClose = func() {}
	}
	if server.Audit == nil {
		server.Audit = func(r *http.Request) bool { return true }
	}
	if server.OnSubscribe == nil {
		server.OnSubscribe = func(key string) error { return nil }
	}
	if server.OnUnsubscribe == nil {
		server.OnUnsubscribe = func(key string) {}
	}
}

// defaultClient sets up the default HTTP client.
// defaultTransport builds the http.Transport used by the server's
// outbound Client. Shared by defaultClient and NewClient so connection
// tuning stays consistent whether or not a custom TLS root pool is set.
func defaultTransport() *http.Transport {
	return &http.Transport{
		Dial: (&net.Dialer{
			Timeout:   1 * time.Second,
			KeepAlive: 10 * time.Second,
		}).Dial,
		IdleConnTimeout:       10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		MaxConnsPerHost:       3000,
		MaxIdleConns:          10000,
		MaxIdleConnsPerHost:   1000,
		DisableKeepAlives:     false,
	}
}

func (server *Server) defaultClient() {
	if server.Client == nil {
		server.Client = &http.Client{
			Timeout:   10 * time.Second,
			Transport: defaultTransport(),
		}
	}
}

// defaults will populate the server fields with their zero values.
func (server *Server) defaults() {
	if server.Name == "" {
		server.Name = "ooo"
	}
	if server.Router == nil {
		server.Router = mux.NewRouter()
	}
	if server.routeOracle == nil {
		server.routeOracle = mux.NewRouter()
	}
	if server.Console == nil {
		server.Console = coat.NewConsole(server.Address, server.Silence)
		server.consoleAutoBuilt = true
	}
	if server.Stream.Console == nil {
		server.Stream.Console = server.Console
	}
	if server.Storage == nil {
		server.Storage = storage.New(storage.LayeredConfig{
			Memory: storage.NewMemoryLayer(),
		})
	}
	if server.Workers == 0 {
		server.Workers = 6
	}
	if server.NoBroadcastKeys == nil {
		server.NoBroadcastKeys = []string{}
	}

	server.defaultTimeouts()
	server.defaultCORS()
	server.defaultCallbacks()
	server.defaultClient()

	// Stream configuration
	if server.Stream.OnSubscribe == nil {
		server.Stream.OnSubscribe = server.OnSubscribe
	}
	if server.Stream.OnUnsubscribe == nil {
		server.Stream.OnUnsubscribe = server.OnUnsubscribe
	}
	server.Stream.ForcePatch = server.ForcePatch
	server.Stream.NoPatch = server.NoPatch
	server.Stream.AllowedOrigins = server.AllowedOrigins
	if server.Stream.ForcePatch && server.Stream.NoPatch {
		server.Console.Err("both ForcePatch and NoPatch are enabled, only NoPatch will be used")
	}
	server.Stream.InitClock()
}

// setupRoutes configures the HTTP routes for the server.
//
// Runs from defaults() inside StartWithError, which is CAS-gated and
// runs before any traffic reaches the wrapper, so this code is not
// racing with ServeHTTP. The routerMu.Lock is defensive — it keeps
// the "mutators take Lock" invariant uniform across every router
// registration site, including the boot-time ones.
func (server *Server) setupRoutes() {
	server.routerMu.Lock()
	defer server.routerMu.Unlock()
	// https://ieftimov.com/post/make-resilient-golang-net-http-servers-using-timeouts-deadlines-context-cancellation/
	explorerHandler := &ui.Handler{
		GetKeys:        server.Storage.Keys,
		GetInfo:        server.getServerInfo,
		GetFilters:     server.filters.Paths,
		GetFiltersInfo: server.getFiltersInfo,
		GetState:       server.getStreamState,
		GetPivotInfo:   server.GetPivotInfo,
		GetEndpoints:   server.getEndpoints,
		GetProxies:     server.getProxies,
		GetOrphanKeys:  server.getOrphanKeys,
		AuditFunc:      server.Audit,
		ClockFunc:      server.clock,
	}
	server.Router.Handle("/", explorerHandler).Methods("GET")
	// Register routes for reserved UI paths
	for _, path := range ui.ReservedPaths {
		if path == "components" {
			server.Router.PathPrefix("/components/").Handler(explorerHandler).Methods("GET")
		} else {
			server.Router.Handle("/"+path, explorerHandler).Methods("GET")
		}
	}
	// The data wildcard accepts the same character class as key.IsValid so
	// that hyphenated/dotted/underscored keys (UUIDs, ISO dates, filenames,
	// snake_case identifiers) are addressable via REST and ws subscriptions.
	// Custom Endpoint and Proxy paths registered via server.Endpoint() /
	// proxy.Register() take precedence: routeOracleSkip checks the oracle
	// router that mirrors those registrations and tells the wildcard to
	// step aside when the request matches an explicit route.
	skip := mux.MatcherFunc(server.routeOracleSkip)
	keyPath := "/{key:" + key.PathPattern + "}"
	server.Router.Handle(keyPath, http.TimeoutHandler(
		http.HandlerFunc(server.unpublish), server.Deadline, deadlineMsg)).Methods("DELETE").MatcherFunc(skip)
	server.Router.Handle(keyPath, http.TimeoutHandler(
		http.HandlerFunc(server.publish), server.Deadline, deadlineMsg)).Methods("POST").MatcherFunc(skip)
	server.Router.Handle(keyPath, http.TimeoutHandler(
		http.HandlerFunc(server.patch), server.Deadline, deadlineMsg)).Methods("PATCH").MatcherFunc(skip)
	server.Router.HandleFunc(keyPath, server.read).Methods("GET").MatcherFunc(skip)
	server.Router.HandleFunc(keyPath, server.read).Queries("v", "{[\\d]}").Methods("GET").MatcherFunc(skip)
}

// routeOracleSkip returns true if the request does NOT match any registered
// Endpoint or Proxy route. It is wired as a MatcherFunc on the data wildcard
// so explicit routes take precedence regardless of registration order.
//
// Takes RLock on the routeOracle so concurrent data-wildcard requests can
// run their oracle check in parallel; RegisterOracleRoute takes the
// write Lock. The mux.Router's Match method is documented as safe for
// concurrent read use once the route set is established.
func (server *Server) routeOracleSkip(r *http.Request, _ *mux.RouteMatch) bool {
	server.routeOracleMu.RLock()
	defer server.routeOracleMu.RUnlock()
	if server.routeOracle == nil {
		return true
	}
	var m mux.RouteMatch
	return !server.routeOracle.Match(r, &m)
}

// RegisterOracleRoute mirrors a path registration onto the oracle router so
// the data wildcard can defer to it. Method-restricted routes pass methods;
// pass nil for any-method routes (proxies). Endpoint() and the proxy package
// call this after registering on Server.Router.
func (server *Server) RegisterOracleRoute(path string, methods []string) {
	server.routeOracleMu.Lock()
	defer server.routeOracleMu.Unlock()
	if server.routeOracle == nil {
		server.routeOracle = mux.NewRouter()
	}
	route := server.routeOracle.HandleFunc(path, func(http.ResponseWriter, *http.Request) {})
	if len(methods) > 0 {
		route.Methods(methods...)
	}
}

// StartWithError initializes and starts the http server and database connection.
// Returns an error if startup fails instead of calling log.Fatal.
//
// Safe to call from multiple goroutines: an atomic CompareAndSwap
// claims the startup slot via the `serverStarting` sentinel so only
// the first caller proceeds; concurrent callers return
// ErrServerAlreadyActive immediately. The sentinel keeps `Active()`
// returning false through the listen-bind window — the field flips
// to `serverActive` only once waitListen has bound, and rolls back
// to `serverInactive` if the bind fails. Mirrors the Close-side CAS
// fix (PR #89) without shifting Active() semantics.
func (server *Server) StartWithError(address string) error {
	if !atomic.CompareAndSwapInt64(&server.active, serverInactive, serverStarting) {
		return ErrServerAlreadyActive
	}
	server.Address = address
	atomic.StoreInt64(&server.closing, 0)
	server.startErr = make(chan error, 1)
	monotonic.Init()
	server.defaults()
	server.setupRoutes()
	// Preallocate stream pools for all registered filter paths
	server.Stream.PreallocatePools(server.filters.Paths())
	server.wg.Add(1)
	server.listenWg.Add(1)
	go server.waitListen()
	server.wg.Wait()
	err := server.waitStart()
	if err != nil {
		return err
	}
	// Rebuild the auto-built Console with the resolved listen address; only
	// when defaults() built it, so a user-supplied Console is preserved.
	if server.consoleAutoBuilt {
		server.Console = coat.NewConsole(server.Address, server.Silence)
		server.Stream.Console = server.Console
	}
	server.clockStop = make(chan struct{})
	server.clockWg.Add(1)
	go server.startClock()
	server.OnStart()
	return nil
}

// Start initializes and starts the http server and database connection.
// Panics if startup fails. Use StartWithError for error handling.
// If the server is already active, this is a no-op (does not panic).
func (server *Server) Start(address string) {
	err := server.StartWithError(address)
	if err != nil && err != ErrServerAlreadyActive {
		log.Fatal(err)
	}
}

// Close runs the graceful shutdown sequence: drains in-flight HTTP
// requests up to Server.Deadline, closes all WebSocket connections,
// closes the storage backend, and invokes user-supplied callbacks at
// well-defined points.
//
// Teardown sequence (in order):
//
//  1. Mark the server as closing (atomic, idempotent — second concurrent
//     call returns immediately).
//  2. Run PreShutdown hooks (registered via RegisterCloseHook;
//     RegisterPreClose is the deprecated equivalent). Storage,
//     stream, and HTTP are still up — hooks may broadcast, read, or
//     write.
//  3. Stop the internal clock goroutine (bounded — clock selects on a
//     stop channel).
//  4. Run ProxyTeardown hooks (registered via RegisterCloseHook;
//     RegisterProxyCleanup is the deprecated equivalent).
//  5. Close all WebSocket connections (bounded — per-connection TCP close).
//  6. Stop the HTTP server: graceful shutdown with a context bounded by
//     Server.Deadline (default 10s), then force-close to cancel any
//     still-running request contexts. Handlers that honour
//     r.Context().Done() exit then; handlers that ignore the context
//     leak (Go cannot kill them).
//  7. Wait for HTTP handlers and the listen goroutine to exit.
//  8. Close the storage backend.
//  9. Wait for storage-watcher goroutines (bounded — they exit when
//     storage closes their channels).
//  10. Run PostShutdown hooks, then the deprecated Server.OnClose
//     field if set. Storage, stream, and HTTP are all torn down — use
//     these for closing user-owned resources only.
//
// Bound:
//
//   - HTTP request drain: bounded by Server.Deadline.
//   - Internal waitgroups (clock, listen, watch, handler): bounded.
//     Clock selects on clockStop, watch goroutines exit when storage
//     closes their channels, the listen goroutine exits when the HTTP
//     server's listener is force-closed at step 6, and the handler
//     waitgroup only tracks the long-lived clock long-poll and
//     WebSocket handlers — both of which exit when Stream.CloseAll
//     closes their connections at step 5. REST publish / read / patch /
//     unpublish and custom Endpoint handlers are not tracked by the
//     handler waitgroup; see the next bullet for how they're bounded.
//   - Storage.Close: bounded for in-tree layers; depends on the bottom-layer
//     contract for embedded storages (e.g. ko, nopog).
//   - preClose cleanups, proxy cleanups, and OnClose: bounded only when
//     Server.CloseCallbackBudget is set to a positive value, which caps
//     the aggregate runtime across all three batches. A callback already
//     in flight is not interrupted, but once the budget is exhausted
//     subsequent callbacks are skipped with a one-line warning. With
//     the default (zero) budget, every callback runs to completion
//     regardless of duration — keep them short or the orchestrator's
//     SIGTERM-to-SIGKILL window has to cover the worst case across all
//     of them in addition to Deadline.
//   - HTTP handlers that ignore r.Context().Done(): NOT bounded. They
//     can outlive Close. Custom Endpoint handlers in particular must
//     check the context.
//
// Hard-kill: SIGKILL is uncatchable; nothing in this sequence runs.
// See WaitClose for the SIGTERM/SIGKILL contract operators should
// configure for.
//
// Safe to call from multiple goroutines: an atomic CompareAndSwap on
// `closing` lets only the first caller through; concurrent callers
// observe the in-progress shutdown and return immediately. The CAS is
// load-bearing — a plain Load + Store guard would let two callers both
// pass and both `close(server.clockStop)`, panicking the second.
func (server *Server) Close(sig os.Signal) {
	if atomic.CompareAndSwapInt64(&server.closing, 0, 1) {
		atomic.StoreInt64(&server.active, serverInactive)
		// callbackElapsed accumulates ONLY the runtime of user-supplied
		// callbacks — the wall clock between batches (HTTP drain bounded
		// by Deadline, Storage.Close, waitgroup joins) does not count
		// against the budget. Zero CloseCallbackBudget means no bound.
		// A callback already in flight is not interrupted; only
		// callbacks that have not started yet are skipped once the
		// cumulative callback time exceeds the budget.
		var callbackElapsed time.Duration
		budgetExceeded := func() bool {
			return server.CloseCallbackBudget > 0 && callbackElapsed >= server.CloseCallbackBudget
		}
		runOne := func(cb func()) {
			start := time.Now()
			cb()
			callbackElapsed += time.Since(start)
		}
		runPhase := func(phase CloseHookPhase, name string) {
			server.closeHooksMu.Lock()
			hooks := server.closeHooks[phase]
			server.closeHooks[phase] = nil
			server.closeHooksMu.Unlock()
			var skipped int
			for _, cb := range hooks {
				if budgetExceeded() {
					skipped++
					continue
				}
				runOne(cb)
			}
			if skipped > 0 {
				server.Console.Err("close: skipped", skipped, name, "hook(s); CloseCallbackBudget exceeded")
			}
		}
		// PreShutdown hooks run first, before any stream/storage cleanup.
		runPhase(PreShutdown, "PreShutdown")
		close(server.clockStop) // Signal clock goroutine to stop immediately
		server.clockWg.Wait()   // Wait for clock goroutine to exit before touching Stream
		// ProxyTeardown hooks run before stream connections are closed
		// so proxy subscriptions can unsubscribe cleanly.
		runPhase(ProxyTeardown, "ProxyTeardown")
		// Force close all stream connections first
		server.Stream.CloseAll()
		// Shutdown HTTP server to stop accepting new connections. Bound the
		// graceful window by Deadline so a stuck handler cannot block
		// shutdown indefinitely. Custom Endpoint handlers are not wrapped
		// in TimeoutHandler, so they must respect r.Context().Done() to
		// exit cleanly during this window.
		//
		// After the graceful window, force-close to cancel any still-running
		// request contexts. Handlers that honour the context exit then;
		// handlers that ignore it leak (they cannot be killed in Go).
		if server.server != nil {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), server.Deadline)
			server.server.Shutdown(shutdownCtx)
			cancel()
			server.server.Close()
		}
		server.handlerWg.Wait() // Wait for HTTP handlers to finish
		server.listenWg.Wait()  // Wait for listen goroutine to finish
		server.Storage.Close()
		server.watchWg.Wait() // Wait for watch goroutines to finish
		// PostShutdown hooks run after every internal teardown step;
		// they see storage, stream, and HTTP all torn down.
		runPhase(PostShutdown, "PostShutdown")
		// OnClose field is the legacy single-shot PostShutdown hook;
		// it runs after every registered PostShutdown hook.
		if budgetExceeded() {
			server.Console.Err("close: skipped OnClose callback; CloseCallbackBudget exceeded")
		} else {
			runOne(server.OnClose)
		}
		server.Console.Err("shutdown", sig)
		// Do not nil out Storage / Router / Console / Stream / filters here.
		// PR #63 made shutdown bounded by Deadline, which means a stuck
		// handler can outlive Close; nilling these fields with no
		// synchronisation would let that goroutine nil-deref. Leaving the
		// fields populated keeps post-close calls error-returning rather
		// than panicking.
	}
}

// WaitClose blocks until the process receives SIGINT, SIGTERM, or SIGHUP
// and then runs the graceful Close sequence with the received signal.
//
// SIGKILL is intentionally not listed. POSIX requires SIGKILL (and
// SIGSTOP) to be uncatchable: the kernel terminates the process with
// no opportunity for user code to run, so no defers, no Close, no
// OnClose, and no user callbacks execute. Anything sitting in
// in-process buffers (queued broadcasts, watcher events, writes that
// the storage backend has not yet flushed to durable media) is lost
// when SIGKILL is delivered. Durability across hard kill is the
// responsibility of the storage backend, not Server.Close.
//
// Operators should configure their orchestrator to send SIGTERM first
// and grant a grace window at least as long as Server.Deadline plus
// the worst-case runtime of any user-supplied preClose / proxy /
// OnClose callback. On Kubernetes this is `terminationGracePeriodSeconds`.
// SIGKILL should be the orchestrator's fallback for a stuck
// SIGTERM teardown, not the primary shutdown path.
func (server *Server) WaitClose() {
	server.Signal = make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(server.Signal, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		sig := <-server.Signal
		server.Close(sig)
		done <- true
	}()
	<-done
}
