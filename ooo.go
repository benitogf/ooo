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
	"github.com/benitogf/ooo/stream"
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

// Server application
//
// Router: can be predefined with routes and passed to be extended
//
// NoBroadcastKeys: array of keys that should not broadcast on changes
//
// DbOpt: options for storage
//
// Audit: function to audit requests
//
// Workers: number of workers to use as readers of the storage->broadcast channel
//
// ForcePatch: flag to force patch operations even if the patch is bigger than the snapshot
//
// OnSubscribe: function to monitor subscribe events
//
// OnUnsubscribe: function to monitor unsubscribe events
//
// OnClose: function that triggers before closing the application
//
// Deadline: time duration of a request before timing out
//
// AllowedOrigins: list of allowed origins for cross domain access, defaults to ["*"]
//
// AllowedMethods: list of allowed methods for cross domain access, defaults to ["GET", "POST", "DELETE", "PUT"]
//
// AllowedHeaders: list of allowed headers for cross domain access, defaults to ["Authorization", "Content-Type"]
//
// ExposedHeaders: list of exposed headers for cross domain access, defaults to nil
//
// Storage: database interdace implementation
//
// Silence: output silence flag
//
// Static: static routing flag
//
// Tick: time interval between ticks on the clock websocket
//
// Signal: os signal channel
//
// Client: http client to make requests
type Server struct {
	wg                sync.WaitGroup
	server            *http.Server
	Router            *mux.Router
	Stream            stream.Stream
	filters           filters.Filters
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
	Storage           Database
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
	OnStorageEvent    StorageEventCallback
	BeforeRead        func(key string)
	startErr          chan error // channel for startup errors
}

// Validate checks the server configuration for common issues.
// Call this before Start() to catch configuration errors early.
func (app *Server) Validate() error {
	if app.ForcePatch && app.NoPatch {
		return ErrForcePatchConflict
	}
	if app.Workers < 0 {
		return ErrNegativeWorkers
	}
	if app.Deadline < 0 {
		return ErrNegativeDeadline
	}
	return nil
}

// tcpKeepAliveListener sets TCP keep-alive timeouts on accepted
// connections. It's used by ListenAndServe and ListenAndServeTLS so
// dead TCP connections (e.g. closing laptop mid-download) eventually
// go away.
type tcpKeepAliveListener struct {
	*net.TCPListener
}

func (app *Server) waitListen() {
	var err error
	storageOpt := StorageOpt{
		NoBroadcastKeys: app.NoBroadcastKeys,
	}

	if app.BeforeRead != nil {
		storageOpt.BeforeRead = app.BeforeRead
	}
	err = app.Storage.Start(storageOpt)
	if err != nil {
		app.startErr <- fmt.Errorf("ooo: storage start failed: %w", err)
		app.wg.Done()
		return
	}
	app.server = &http.Server{
		WriteTimeout:      app.WriteTimeout,
		ReadTimeout:       app.ReadTimeout,
		ReadHeaderTimeout: app.ReadHeaderTimeout,
		IdleTimeout:       app.IdleTimeout,
		Addr:              app.Address,
		Handler: cors.New(cors.Options{
			AllowedMethods: app.AllowedMethods,
			AllowedOrigins: app.AllowedOrigins,
			AllowedHeaders: app.AllowedHeaders,
			ExposedHeaders: app.ExposedHeaders,
			// AllowCredentials: true,
			// Debug:          true,
		}).Handler(handlers.CompressHandler(app.Router))}
	ln, err := net.Listen("tcp4", app.Address)
	if err != nil {
		app.startErr <- fmt.Errorf("ooo: failed to start tcp: %w", err)
		app.wg.Done()
		return
	}
	app.Address = ln.Addr().String()
	atomic.StoreInt64(&app.active, 1)
	app.wg.Done()
	err = app.server.Serve(tcpKeepAliveListener{ln.(*net.TCPListener)})
	if atomic.LoadInt64(&app.closing) != 1 && err != nil {
		app.Console.Err("server error", err)
	}
}

// Active check if the server is active
func (app *Server) Active() bool {
	return atomic.LoadInt64(&app.active) == 1 && atomic.LoadInt64(&app.closing) == 0
}

func (app *Server) waitStart() error {
	// Check for startup errors from waitListen
	select {
	case err := <-app.startErr:
		return err
	default:
	}

	if atomic.LoadInt64(&app.active) == 0 || !app.Storage.Active() {
		return ErrServerStartFailed
	}

	for i := 0; i < app.Workers; i++ {
		go app.watch(app.Storage.Watch())
	}

	app.Console.Log("glad to serve[" + app.Address + "]")
	return nil
}

// FetchResult holds the result of a fetch operation for initial WebSocket message
type FetchResult struct {
	Data    []byte
	Version int64
}

// Fetch data, update cache and apply filter
func (app *Server) fetch(path string) (FetchResult, error) {
	// Check if any filter exists for static mode validation
	if app.Static {
		hasFilter := app.filters.ReadObject.HasMatch(path) != -1 ||
			app.filters.ReadList.HasMatch(path) != -1
		if !hasFilter {
			return FetchResult{}, filters.ErrRouteNotDefined
		}
	}

	if key.HasGlob(path) {
		// List subscription
		objs, err := app.Storage.GetList(path)
		if err != nil {
			return FetchResult{}, err
		}
		filtered, err := app.filters.ReadList.Check(path, objs, app.Static)
		if err != nil {
			return FetchResult{}, err
		}
		// Initialize decoded cache (creates pool if needed)
		version := app.Stream.InitCacheObjectsWithVersion(path, filtered)
		// Encode for sending
		data, err := meta.Encode(filtered)
		if err != nil {
			return FetchResult{}, err
		}
		return FetchResult{Data: data, Version: version}, nil
	}

	// Single object subscription
	obj, err := app.Storage.Get(path)
	if err != nil {
		// Object not found - return empty object
		obj = meta.Object{}
	}
	filtered, err := app.filters.ReadObject.Check(path, obj, app.Static)
	if err != nil {
		return FetchResult{}, err
	}
	// Initialize decoded cache (creates pool if needed)
	version := app.Stream.InitCacheObjectWithVersion(path, &filtered)
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

func (app *Server) watch(sc StorageChan) {
	for {
		ev, ok := <-sc
		if !ok {
			// Channel closed
			break
		}
		if ev.Key != "" {
			app.Console.Log("broadcast[" + ev.Key + "]")
			app.Stream.Broadcast(ev.Key, stream.BroadcastOpt{
				Key:       ev.Key,
				Operation: ev.Operation,
				Object:    ev.Object,
				FilterObject: func(key string, obj meta.Object) (meta.Object, error) {
					return app.filters.ReadObject.Check(key, obj, app.Static)
				},
				FilterList: func(key string, objs []meta.Object) ([]meta.Object, error) {
					return app.filters.ReadList.Check(key, objs, app.Static)
				},
				Static: app.Static,
			})
			if app.OnStorageEvent != nil {
				app.OnStorageEvent(ev)
			}
		}
		if !app.Storage.Active() {
			break
		}
	}
}

// defaultCORS sets default CORS configuration.
func (app *Server) defaultCORS() {
	if len(app.AllowedOrigins) == 0 {
		app.AllowedOrigins = []string{"*"}
	}
	if len(app.AllowedMethods) == 0 {
		app.AllowedMethods = []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodDelete,
			http.MethodPut,
			http.MethodPatch,
		}
	}
	if len(app.AllowedHeaders) == 0 {
		app.AllowedHeaders = []string{"Authorization", "Content-Type"}
	}
}

// defaultTimeouts sets default timeout values.
func (app *Server) defaultTimeouts() {
	if app.Deadline.Nanoseconds() == 0 {
		app.Deadline = time.Second * 10
	}
	if app.Tick == 0 {
		app.Tick = 1 * time.Second
	}
	if app.ReadTimeout == 0 {
		app.ReadTimeout = 1 * time.Minute
	}
	if app.WriteTimeout == 0 {
		app.WriteTimeout = 1 * time.Minute
	}
	if app.ReadHeaderTimeout == 0 {
		app.ReadHeaderTimeout = 10 * time.Second
	}
	if app.IdleTimeout == 0 {
		app.IdleTimeout = 10 * time.Second
	}
}

// defaultCallbacks sets default callback functions.
func (app *Server) defaultCallbacks() {
	if app.OnClose == nil {
		app.OnClose = func() {}
	}
	if app.Audit == nil {
		app.Audit = func(r *http.Request) bool { return true }
	}
	if app.OnSubscribe == nil {
		app.OnSubscribe = func(key string) error { return nil }
	}
	if app.OnUnsubscribe == nil {
		app.OnUnsubscribe = func(key string) {}
	}
}

// defaultClient sets up the default HTTP client.
func (app *Server) defaultClient() {
	if app.Client == nil {
		app.Client = &http.Client{
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
func (app *Server) defaults() {
	if app.Router == nil {
		app.Router = mux.NewRouter()
	}
	if app.Console == nil {
		app.Console = coat.NewConsole(app.Address, app.Silence)
	}
	if app.Stream.Console == nil {
		app.Stream.Console = app.Console
	}
	if app.Storage == nil {
		app.Storage = &MemoryStorage{}
	}
	if app.Workers == 0 {
		app.Workers = 6
	}
	if app.NoBroadcastKeys == nil {
		app.NoBroadcastKeys = []string{}
	}

	app.defaultTimeouts()
	app.defaultCORS()
	app.defaultCallbacks()
	app.defaultClient()

	// Stream configuration
	if app.Stream.OnSubscribe == nil {
		app.Stream.OnSubscribe = app.OnSubscribe
	}
	if app.Stream.OnUnsubscribe == nil {
		app.Stream.OnUnsubscribe = app.OnUnsubscribe
	}
	app.Stream.ForcePatch = app.ForcePatch
	app.Stream.NoPatch = app.NoPatch
	if app.Stream.ForcePatch && app.Stream.NoPatch {
		app.Console.Err("both ForcePatch and NoPatch are enabled, only NoPatch will be used")
	}
	app.Stream.InitClock()
}

// setupRoutes configures the HTTP routes for the server.
func (app *Server) setupRoutes() {
	// https://ieftimov.com/post/make-resilient-golang-net-http-servers-using-timeouts-deadlines-context-cancellation/
	app.Router.HandleFunc("/", app.getStats).Methods("GET")
	// https://www.calhoun.io/why-cant-i-pass-this-function-as-an-http-handler/
	app.Router.Handle("/{key:[a-zA-Z\\*\\d\\/]+}", http.TimeoutHandler(
		http.HandlerFunc(app.unpublish), app.Deadline, deadlineMsg)).Methods("DELETE")
	app.Router.Handle("/{key:[a-zA-Z\\*\\d\\/]+}", http.TimeoutHandler(
		http.HandlerFunc(app.publish), app.Deadline, deadlineMsg)).Methods("POST")
	app.Router.Handle("/{key:[a-zA-Z\\*\\d\\/]+}", http.TimeoutHandler(
		http.HandlerFunc(app.republish), app.Deadline, deadlineMsg)).Methods("PUT")
	app.Router.Handle("/{key:[a-zA-Z\\*\\d\\/]+}", http.TimeoutHandler(
		http.HandlerFunc(app.patch), app.Deadline, deadlineMsg)).Methods("PATCH")
	app.Router.HandleFunc("/{key:[a-zA-Z\\*\\d\\/]+}", app.read).Methods("GET")
	app.Router.HandleFunc("/{key:[a-zA-Z\\*\\d\\/]+}", app.read).Queries("v", "{[\\d]}").Methods("GET")
}

// StartWithError initializes and starts the http server and database connection.
// Returns an error if startup fails instead of calling log.Fatal.
func (app *Server) StartWithError(address string) error {
	app.Address = address
	if atomic.LoadInt64(&app.active) == 1 {
		return ErrServerAlreadyActive
	}
	atomic.StoreInt64(&app.active, 0)
	atomic.StoreInt64(&app.closing, 0)
	app.startErr = make(chan error, 1)
	app.defaults()
	app.setupRoutes()
	// Preallocate stream pools for all registered filter paths
	app.Stream.PreallocatePools(app.filters.Paths())
	app.wg.Add(1)
	go app.waitListen()
	app.wg.Wait()
	if err := app.waitStart(); err != nil {
		return err
	}
	app.Console = coat.NewConsole(app.Address, app.Silence)
	go app.startClock()
	return nil
}

// Start initializes and starts the http server and database connection.
// Panics if startup fails. Use StartWithError for error handling.
// If the server is already active, this is a no-op (does not panic).
func (app *Server) Start(address string) {
	err := app.StartWithError(address)
	if err != nil && err != ErrServerAlreadyActive {
		log.Fatal(err)
	}
}

// Close : shutdown the http server and database connection
func (app *Server) Close(sig os.Signal) {
	if atomic.LoadInt64(&app.closing) != 1 {
		atomic.StoreInt64(&app.closing, 1)
		atomic.StoreInt64(&app.active, 0)
		app.Storage.Close()
		app.OnClose()
		app.Console.Err("shutdown", sig)
		if app.server != nil {
			app.server.Shutdown(context.Background())
		}
	}
}

// WaitClose : Blocks waiting for SIGINT, SIGTERM, SIGKILL, SIGHUP
func (app *Server) WaitClose() {
	app.Signal = make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(app.Signal, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		sig := <-app.Signal
		app.Close(sig)
		done <- true
	}()
	<-done
}
