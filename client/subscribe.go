package client

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/benitogf/coat"
	"github.com/benitogf/go-json"
	"github.com/benitogf/ooo/key"
	"github.com/benitogf/ooo/messages"
	"github.com/gorilla/websocket"
)

var (
	DefaultHandshakeTimeout = time.Second * 2
	// Default retry delays
	DefaultInitialRetryDelay = 300 * time.Millisecond
	DefaultMediumRetryDelay  = 2 * time.Second
	DefaultMaxRetryDelay     = 10 * time.Second
	// Default retry thresholds
	DefaultMediumRetryThreshold = 30
	DefaultMaxRetryThreshold    = 100
)

var (
	ErrCtxRequired       = errors.New("client: Ctx is required")
	ErrProtocolRequired  = errors.New("client: Protocol is required")
	ErrHostRequired      = errors.New("client: Host is required")
	ErrPathRequired      = errors.New("client: path is required")
	ErrOnMessageRequired = errors.New("client: OnMessage callback is required")
	ErrGlobNotAllowed    = errors.New("client: glob pattern not allowed for Subscribe, use SubscribeList")
	ErrGlobRequired      = errors.New("client: glob pattern required for SubscribeList, use Subscribe")
)

// Server holds the protocol and host for WebSocket connections.
// This type is shared across all subscription functions.
type Server struct {
	Protocol string
	Host     string
}

type Meta[T any] struct {
	Created int64  `json:"created"`
	Updated int64  `json:"updated"`
	Index   string `json:"index"`
	Data    T      `json:"data"`
}

// RetryConfig holds retry configuration for Subscribe.
type RetryConfig struct {
	// InitialDelay is the delay for the first retries (default: 300ms)
	InitialDelay time.Duration
	// MediumDelay is the delay after MediumThreshold retries (default: 2s)
	MediumDelay time.Duration
	// MaxDelay is the delay after MaxThreshold retries (default: 10s)
	MaxDelay time.Duration
	// MediumThreshold is the retry count to switch to MediumDelay (default: 30)
	MediumThreshold int
	// MaxThreshold is the retry count to switch to MaxDelay (default: 100)
	MaxThreshold int
}

// SubscribeConfig holds connection configuration for Subscribe.
// Required fields: Ctx, Server.
// Optional fields: Header, HandshakeTimeout, Retry, Silence, TLSConfig.
type SubscribeConfig struct {
	Ctx              context.Context
	Server           Server
	Header           http.Header
	HandshakeTimeout time.Duration
	Retry            RetryConfig
	Silence          bool // if true, suppresses log output
	// TLSConfig, when set, is used for the "wss" websocket handshake. Leave
	// nil to use the system cert pool (the default). Set RootCAs to trust an
	// internal/self-signed CA (e.g. via x509.SystemCertPool + AppendCertsFromPEM,
	// or a test CA) when the peer serves TLS with a non-public certificate.
	TLSConfig *tls.Config
	console   *coat.Console // internal console for logging
}

// Validate checks that required fields are set and applies defaults.
func (c *SubscribeConfig) Validate() error {
	if c.Ctx == nil {
		return ErrCtxRequired
	}
	if c.Server.Protocol == "" {
		return ErrProtocolRequired
	}
	if c.Server.Host == "" {
		return ErrHostRequired
	}
	if c.HandshakeTimeout == 0 {
		c.HandshakeTimeout = DefaultHandshakeTimeout
	}
	// Apply retry defaults
	if c.Retry.InitialDelay == 0 {
		c.Retry.InitialDelay = DefaultInitialRetryDelay
	}
	if c.Retry.MediumDelay == 0 {
		c.Retry.MediumDelay = DefaultMediumRetryDelay
	}
	if c.Retry.MaxDelay == 0 {
		c.Retry.MaxDelay = DefaultMaxRetryDelay
	}
	if c.Retry.MediumThreshold == 0 {
		c.Retry.MediumThreshold = DefaultMediumRetryThreshold
	}
	if c.Retry.MaxThreshold == 0 {
		c.Retry.MaxThreshold = DefaultMaxRetryThreshold
	}
	// Initialize console with host as identifier
	c.console = coat.NewConsole(c.Server.Host, c.Silence)
	return nil
}

// SubscribeEvents holds event callbacks for Subscribe (single object).
// Required: OnMessage.
// Optional: OnError.
type SubscribeEvents[T any] struct {
	OnMessage func(Meta[T])
	OnError   func(error)
}

// Validate checks that required callbacks are set.
func (e *SubscribeEvents[T]) Validate() error {
	if e.OnMessage == nil {
		return ErrOnMessageRequired
	}
	return nil
}

// SubscribeListEvents holds event callbacks for SubscribeList (glob patterns).
// Required: OnMessage.
// Optional: OnError.
type SubscribeListEvents[T any] struct {
	OnMessage func([]Meta[T])
	OnError   func(error)
}

// Validate checks that required callbacks are set.
func (e *SubscribeListEvents[T]) Validate() error {
	if e.OnMessage == nil {
		return ErrOnMessageRequired
	}
	return nil
}

// subscribeCore holds the mutable state shared between Subscribe and
// SubscribeList. Everything that varies between the two entry points —
// wire decode, per-item unmarshal policy, OnMessage signature — is
// injected via the handle closure. The connect/read/retry/close
// machinery is identical for both variants and lives here in one
// place. No type parameters are needed on the core itself: the
// payload type T is captured inside each entry point's handle
// closure (for the var item T, json.Unmarshal, and typed
// OnMessage(Meta[T]) call), which is sufficient for type-safe
// delivery. onError is a plain func(error) and needs no T.
type subscribeCore struct {
	cfg        SubscribeConfig
	path       string
	label      string // "subscribe" or "subscribeList" — feeds logPrefix
	wsURL      url.URL
	cache      json.RawMessage
	retryCount int
	closing    atomic.Bool
	muClient   sync.Mutex
	wsClient   *websocket.Conn

	onError func(error)
	// handle decodes one wire message + delivers it to OnMessage,
	// firing onError per the variant's contract (fatal-on-error for
	// single, fire-and-continue per item for list). Returns false to
	// break the read loop (catastrophic decode), true to continue.
	handle func(message []byte) bool
}

// logPrefix returns a consistent log prefix for this subscription.
func (s *subscribeCore) logPrefix() string {
	return s.label + "[" + s.cfg.Server.Host + "/" + s.path + "]"
}

// isClosing reports whether the subscription is being torn down. The
// startCloseWatcher goroutine flips closing on ctx.Done, but it is a
// separate goroutine — between ctx cancellation and the watcher running,
// the retry loop could observe closing=false and dial again. Falling back
// to ctx.Err() closes that window: the context has happens-before with
// itself, so any goroutine that reads ctx.Err() after cancel sees the
// non-nil error immediately.
func (s *subscribeCore) isClosing() bool {
	return s.closing.Load() || s.cfg.Ctx.Err() != nil
}

// connect establishes a WebSocket connection.
// Returns true if connection was successful, false otherwise.
// Backoff on failure is the caller's responsibility (see waitRetry).
func (s *subscribeCore) connect() bool {
	dialer := &websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: s.cfg.HandshakeTimeout,
		TLSClientConfig:  s.cfg.TLSConfig,
	}

	s.muClient.Lock()
	var err error
	s.wsClient, _, err = dialer.DialContext(s.cfg.Ctx, s.wsURL.String(), s.cfg.Header)
	if s.wsClient == nil || err != nil {
		s.muClient.Unlock()
		s.cfg.console.Err(s.logPrefix()+": failed websocket dial", err)
		// Suppress OnError when the subscription is being torn down: a
		// cancelled ctx (whether observed by us or by the dialer) means
		// the caller asked us to stop, so the dial failure is expected
		// and not actionable. Firing OnError here races with test
		// cleanup — callbacks see a "fail in goroutine after test has
		// completed" panic when the retry loop dials a server that the
		// caller has already begun shutting down.
		if !s.isClosing() && s.onError != nil {
			s.onError(err)
		}
		return false
	}
	s.muClient.Unlock()

	s.cfg.console.Log(s.logPrefix() + ": connection established")
	return true
}

// readLoop reads messages from the WebSocket until an error occurs.
func (s *subscribeCore) readLoop() {
	for {
		_, message, err := s.wsClient.ReadMessage()
		if err != nil || message == nil {
			s.cfg.console.Err(s.logPrefix()+": read error", err)
			// Same rationale as connect(): a read error during teardown
			// is the expected consequence of the caller cancelling ctx
			// (the close watcher closes the conn), not something the
			// caller wants to hear about.
			if !s.isClosing() && s.onError != nil {
				s.onError(err)
			}
			s.wsClient.Close()
			return
		}
		if !s.handle(message) {
			s.wsClient.Close()
			return
		}
	}
}

// waitRetry waits before reconnecting based on retry count.
// Returns immediately if the subscription context is cancelled.
func (s *subscribeCore) waitRetry() {
	s.retryCount++
	var delay time.Duration
	switch {
	case s.retryCount < s.cfg.Retry.MediumThreshold:
		s.cfg.console.Log(s.logPrefix() + ": reconnecting...")
		delay = s.cfg.Retry.InitialDelay
	case s.retryCount < s.cfg.Retry.MaxThreshold:
		s.cfg.console.Log(s.logPrefix()+": reconnecting in", s.cfg.Retry.MediumDelay)
		delay = s.cfg.Retry.MediumDelay
	default:
		s.cfg.console.Log(s.logPrefix()+": reconnecting in", s.cfg.Retry.MaxDelay)
		delay = s.cfg.Retry.MaxDelay
	}
	select {
	case <-time.After(delay):
	case <-s.cfg.Ctx.Done():
	}
}

// startCloseWatcher starts a goroutine that closes the connection when context is done.
func (s *subscribeCore) startCloseWatcher() {
	go func() {
		<-s.cfg.Ctx.Done()
		s.closing.Store(true)
		s.muClient.Lock()
		defer s.muClient.Unlock()
		if s.wsClient == nil {
			s.cfg.console.Log(s.logPrefix() + ": closing but no connection")
			return
		}
		s.cfg.console.Err(s.logPrefix()+": closing", s.cfg.Ctx.Err())
		s.wsClient.Close()
	}()
}

// run drives the connect/read/retry loop until the context is cancelled.
func (s *subscribeCore) run() error {
	s.startCloseWatcher()
	for {
		if s.isClosing() {
			s.cfg.console.Log(s.logPrefix() + ": skip reconnection, closing...")
			break
		}
		if !s.connect() {
			s.waitRetry()
			continue
		}
		s.readLoop()
		if s.isClosing() {
			s.cfg.console.Log(s.logPrefix() + ": skip reconnection, closing...")
			break
		}
		s.waitRetry()
	}
	return nil
}

// Subscribe establishes a WebSocket subscription to a single object path (non-glob).
// It automatically reconnects on connection loss with exponential backoff.
// The subscription runs until the context is cancelled.
// For glob patterns (lists), use SubscribeList instead.
func Subscribe[T any](cfg SubscribeConfig, path string, events SubscribeEvents[T]) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if path == "" {
		return ErrPathRequired
	}
	if key.IsGlob(path) {
		return ErrGlobNotAllowed
	}
	if err := events.Validate(); err != nil {
		return err
	}

	core := &subscribeCore{
		cfg:     cfg,
		path:    path,
		label:   "subscribe",
		wsURL:   url.URL{Scheme: cfg.Server.Protocol, Host: cfg.Server.Host, Path: path},
		onError: events.OnError,
	}
	// Single-object decode: any error is fatal (terminates the read
	// loop and triggers a reconnect after backoff). Mirrors the
	// pre-consolidation Subscribe semantics exactly.
	core.handle = func(message []byte) bool {
		next, obj, err := messages.Patch(message, core.cache)
		core.cache = next
		if err != nil {
			core.cfg.console.Err(core.logPrefix()+": failed to parse message", err)
			if core.onError != nil {
				core.onError(err)
			}
			return false
		}
		var item T
		if err := json.Unmarshal(obj.Data, &item); err != nil {
			core.cfg.console.Err(core.logPrefix()+": failed to unmarshal item", err)
			if core.onError != nil {
				core.onError(err)
			}
			return false
		}
		core.retryCount = 0
		events.OnMessage(Meta[T]{
			Created: obj.Created,
			Updated: obj.Updated,
			Index:   obj.Index,
			Data:    item,
		})
		return true
	}
	return core.run()
}

// SubscribeList establishes a WebSocket subscription to a glob pattern path (list).
// It automatically reconnects on connection loss with exponential backoff.
// The subscription runs until the context is cancelled.
// For single object paths (non-glob), use Subscribe instead.
func SubscribeList[T any](cfg SubscribeConfig, path string, events SubscribeListEvents[T]) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if path == "" {
		return ErrPathRequired
	}
	if !key.IsGlob(path) {
		return ErrGlobRequired
	}
	if err := events.Validate(); err != nil {
		return err
	}

	core := &subscribeCore{
		cfg:     cfg,
		path:    path,
		label:   "subscribeList",
		wsURL:   url.URL{Scheme: cfg.Server.Protocol, Host: cfg.Server.Host, Path: path},
		onError: events.OnError,
	}
	// List decode: a wire-level (PatchList) failure terminates the
	// read loop, but per-item unmarshal failures fire OnError and
	// continue so a single bad item does not kill the subscription.
	// Mirrors the pre-consolidation SubscribeList semantics exactly.
	core.handle = func(message []byte) bool {
		next, objs, err := messages.PatchList(message, core.cache)
		core.cache = next
		if err != nil {
			core.cfg.console.Err(core.logPrefix()+": failed to parse list message", err)
			if core.onError != nil {
				core.onError(err)
			}
			return false
		}
		result := make([]Meta[T], 0, len(objs))
		for _, obj := range objs {
			var item T
			if err := json.Unmarshal(obj.Data, &item); err != nil {
				core.cfg.console.Err(core.logPrefix()+": failed to unmarshal list item", err)
				if core.onError != nil {
					core.onError(err)
				}
				continue
			}
			result = append(result, Meta[T]{
				Created: obj.Created,
				Updated: obj.Updated,
				Index:   obj.Index,
				Data:    item,
			})
		}
		core.retryCount = 0
		events.OnMessage(result)
		return true
	}
	return core.run()
}
