package ooo

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
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

// audit requests function
// will define approval or denial by the return value
// r: the request to be audited
// returns
// true: approve the request
// false: rejects the request
type audit func(r *http.Request) bool

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
// OnClose: function that triggers before closing the application
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
type Server struct {
	wg                sync.WaitGroup
	watchWg           sync.WaitGroup
	listenWg          sync.WaitGroup
	handlerWg         sync.WaitGroup
	clockWg           sync.WaitGroup
	server            *http.Server
	Name              string
	Router            *mux.Router
	Stream            stream.Stream
	filters           filters.Filters
	limitFilters      map[string]int // tracks limit filter paths and their limits
	NoBroadcastKeys   []string
	Audit             audit
	Workers           int
	ForcePatch        bool
	NoPatch           bool
	OnSubscribe       stream.Subscribe
	OnUnsubscribe     stream.Unsubscribe
	OnClose           func()
	Deadline          time.Duration
	AllowedOrigins    []string
	AllowedMethods    []string
	AllowedHeaders    []string
	ExposedHeaders    []string
	Storage           storage.Database
	Address           string
	closing           int64
	active            int64
	Silence           bool
	Static            bool
	Tick              time.Duration
	Console           *coat.Console
	Signal            chan os.Signal
	Client            *http.Client
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	ReadHeaderTimeout time.Duration
	IdleTimeout       time.Duration
	OnStorageEvent    storage.EventCallback
	BeforeRead        func(key string)
	startErr          chan error    // channel for startup errors
	clockStop         chan struct{} // channel to signal clock goroutine to stop
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
	}
}

// getFiltersInfo returns detailed filter information for the explorer
func (server *Server) getFiltersInfo() []ui.FilterInfo {
	filtersInfo := server.filters.PathsInfo(server.limitFilters)
	result := make([]ui.FilterInfo, len(filtersInfo))
	for i, f := range filtersInfo {
		result[i] = ui.FilterInfo{
			Path:      f.Path,
			Type:      f.Type,
			CanRead:   f.CanRead,
			CanWrite:  f.CanWrite,
			CanDelete: f.CanDelete,
			IsGlob:    f.IsGlob,
			Limit:     f.Limit,
		}
	}
	return result
}

// RegisterLimitFilter registers a limit filter and tracks it for the ui.
// The LimitFilter should already be created and its filters added to the server.
// This method just tracks the limit value for display in the ui.
func (server *Server) RegisterLimitFilter(lf *filters.LimitFilter) {
	if server.limitFilters == nil {
		server.limitFilters = make(map[string]int)
	}
	server.limitFilters[lf.Path()] = lf.Limit()
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
	err = server.Storage.Start(storageOpt)
	if err != nil {
		server.startErr <- fmt.Errorf("ooo: storage start failed: %w", err)
		server.wg.Done()
		return
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
		}).Handler(handlers.CompressHandler(server.Router))}
	ln, err := net.Listen("tcp4", server.Address)
	if err != nil {
		server.startErr <- fmt.Errorf("ooo: failed to start tcp: %w", err)
		server.wg.Done()
		return
	}
	server.Address = ln.Addr().String()
	atomic.StoreInt64(&server.active, 1)
	server.wg.Done()
	err = server.server.Serve(tcpKeepAliveListener{ln.(*net.TCPListener)})
	if atomic.LoadInt64(&server.closing) != 1 && err != nil {
		server.Console.Err("server error", err)
	}
}

// Active check if the server is active
func (server *Server) Active() bool {
	return atomic.LoadInt64(&server.active) == 1 && atomic.LoadInt64(&server.closing) == 0
}

func (server *Server) waitStart() error {
	// Check for startup errors from waitListen
	select {
	case err := <-server.startErr:
		return err
	default:
	}

	if atomic.LoadInt64(&server.active) == 0 || !server.Storage.Active() {
		return ErrServerStartFailed
	}

	// Start workers for sharded watcher (per-key ordering)
	shardedWatcher := server.Storage.WatchSharded()
	for i := 0; i < shardedWatcher.Count(); i++ {
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
	filtered, err := server.filters.ReadObject.Check(path, obj, server.Static)
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
		if ev.Key != "" {
			server.Console.Log("broadcast[" + ev.Key + "]")
			server.Stream.Broadcast(ev.Key, stream.BroadcastOpt{
				Key:       ev.Key,
				Operation: ev.Operation,
				Object:    ev.Object,
				FilterObject: func(key string, obj meta.Object) (meta.Object, error) {
					return server.filters.ReadObject.Check(key, obj, server.Static)
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
		if !server.Storage.Active() {
			break
		}
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
}

// defaultCallbacks sets default callback functions.
func (server *Server) defaultCallbacks() {
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
func (server *Server) defaultClient() {
	if server.Client == nil {
		server.Client = &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
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
			},
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
	if server.Console == nil {
		server.Console = coat.NewConsole(server.Address, server.Silence)
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
	if server.Stream.ForcePatch && server.Stream.NoPatch {
		server.Console.Err("both ForcePatch and NoPatch are enabled, only NoPatch will be used")
	}
	server.Stream.InitClock()
}

// setupRoutes configures the HTTP routes for the server.
func (server *Server) setupRoutes() {
	// https://ieftimov.com/post/make-resilient-golang-net-http-servers-using-timeouts-deadlines-context-cancellation/
	explorerHandler := &ui.Handler{
		GetKeys:        server.Storage.Keys,
		GetInfo:        server.getServerInfo,
		GetFilters:     server.filters.Paths,
		GetFiltersInfo: server.getFiltersInfo,
		GetState:       server.getStreamState,
		AuditFunc:      server.Audit,
		ClockFunc:      server.clock,
	}
	server.Router.Handle("/", explorerHandler).Methods("GET")
	server.Router.Handle("/vanilla-jsoneditor.js", explorerHandler).Methods("GET")
	server.Router.Handle("/react.min.js", explorerHandler).Methods("GET")
	server.Router.Handle("/react-dom.min.js", explorerHandler).Methods("GET")
	server.Router.Handle("/babel.min.js", explorerHandler).Methods("GET")
	server.Router.Handle("/styles.css", explorerHandler).Methods("GET")
	server.Router.Handle("/ooo-client.js", explorerHandler).Methods("GET")
	server.Router.Handle("/react-json-view.js", explorerHandler).Methods("GET")
	server.Router.Handle("/api.js", explorerHandler).Methods("GET")
	server.Router.Handle("/favicon.ico", explorerHandler).Methods("GET")
	server.Router.Handle("/favicon.png", explorerHandler).Methods("GET")
	server.Router.Handle("/logo.jpg", explorerHandler).Methods("GET")
	server.Router.Handle("/logo.png", explorerHandler).Methods("GET")
	server.Router.PathPrefix("/components/").Handler(explorerHandler).Methods("GET")
	// https://www.calhoun.io/why-cant-i-pass-this-function-as-an-http-handler/
	server.Router.Handle("/{key:[a-zA-Z\\*\\d\\/]+}", http.TimeoutHandler(
		http.HandlerFunc(server.unpublish), server.Deadline, deadlineMsg)).Methods("DELETE")
	server.Router.Handle("/{key:[a-zA-Z\\*\\d\\/]+}", http.TimeoutHandler(
		http.HandlerFunc(server.publish), server.Deadline, deadlineMsg)).Methods("POST")
	server.Router.Handle("/{key:[a-zA-Z\\*\\d\\/]+}", http.TimeoutHandler(
		http.HandlerFunc(server.patch), server.Deadline, deadlineMsg)).Methods("PATCH")
	server.Router.HandleFunc("/{key:[a-zA-Z\\*\\d\\/]+}", server.read).Methods("GET")
	server.Router.HandleFunc("/{key:[a-zA-Z\\*\\d\\/]+}", server.read).Queries("v", "{[\\d]}").Methods("GET")
}

// StartWithError initializes and starts the http server and database connection.
// Returns an error if startup fails instead of calling log.Fatal.
func (server *Server) StartWithError(address string) error {
	server.Address = address
	if atomic.LoadInt64(&server.active) == 1 {
		return ErrServerAlreadyActive
	}
	atomic.StoreInt64(&server.active, 0)
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
	server.Console = coat.NewConsole(server.Address, server.Silence)
	server.clockStop = make(chan struct{})
	server.clockWg.Add(1)
	go server.startClock()
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

// Close : shutdown the http server and database connection
func (server *Server) Close(sig os.Signal) {
	if atomic.LoadInt64(&server.closing) != 1 {
		atomic.StoreInt64(&server.closing, 1)
		atomic.StoreInt64(&server.active, 0)
		close(server.clockStop) // Signal clock goroutine to stop immediately
		server.clockWg.Wait()   // Wait for clock goroutine to exit before touching Stream
		// Force close all stream connections first
		server.Stream.CloseAll()
		// Shutdown HTTP server to stop accepting new connections
		if server.server != nil {
			server.server.Shutdown(context.Background())
		}
		server.handlerWg.Wait() // Wait for HTTP handlers to finish
		server.listenWg.Wait()  // Wait for listen goroutine to finish
		server.Storage.Close()
		server.watchWg.Wait() // Wait for watch goroutines to finish
		server.OnClose()
		server.Console.Err("shutdown", sig)
		// Clear server state to allow restarting
		server.server = nil
		server.Router = nil
		server.Stream = stream.Stream{}
		server.Storage = nil
		server.Console = nil
		server.filters = filters.Filters{}
		server.limitFilters = nil
		server.startErr = nil
		server.clockStop = nil
	}
}

// WaitClose : Blocks waiting for SIGINT, SIGTERM, SIGKILL, SIGHUP
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
