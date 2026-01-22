package client

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/benitogf/coat"
	"github.com/benitogf/ooo/key"
	"github.com/benitogf/ooo/messages"
	"github.com/benitogf/ooo/meta"
	"github.com/goccy/go-json"
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
// Optional fields: Header, HandshakeTimeout, Retry, Silence.
type SubscribeConfig struct {
	Ctx              context.Context
	Server           Server
	Header           http.Header
	HandshakeTimeout time.Duration
	Retry            RetryConfig
	Silence          bool          // if true, suppresses log output
	console          *coat.Console // internal console for logging
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

// subscribeState holds the mutable state for a single object subscription.
type subscribeState[T any] struct {
	cfg        SubscribeConfig
	path       string
	events     SubscribeEvents[T]
	wsURL      url.URL
	cache      json.RawMessage
	retryCount int
	closing    atomic.Bool
	muClient   sync.Mutex
	wsClient   *websocket.Conn
}

// logPrefix returns a consistent log prefix for this subscription.
func (s *subscribeState[T]) logPrefix() string {
	return "subscribe[" + s.cfg.Server.Host + "/" + s.path + "]"
}

// connect establishes a WebSocket connection.
// Returns true if connection was successful, false otherwise.
func (s *subscribeState[T]) connect() bool {
	dialer := &websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: s.cfg.HandshakeTimeout,
	}

	s.muClient.Lock()
	var err error
	s.wsClient, _, err = dialer.Dial(s.wsURL.String(), s.cfg.Header)
	if s.wsClient == nil || err != nil {
		s.muClient.Unlock()
		s.cfg.console.Err(s.logPrefix()+": failed websocket dial", err)
		if s.events.OnError != nil {
			s.events.OnError(err)
		}
		time.Sleep(2 * time.Second)
		return false
	}
	s.muClient.Unlock()

	s.cfg.console.Log(s.logPrefix() + ": connection established")
	return true
}

// handleMessage processes a single WebSocket message.
// Returns false if the read loop should break.
func (s *subscribeState[T]) handleMessage(message []byte) bool {
	var err error
	var obj meta.Object
	s.cache, obj, err = messages.Patch(message, s.cache)
	if err != nil {
		s.cfg.console.Err(s.logPrefix()+": failed to parse message", err)
		if s.events.OnError != nil {
			s.events.OnError(err)
		}
		return false
	}
	var item T
	err = json.Unmarshal(obj.Data, &item)
	if err != nil {
		s.cfg.console.Err(s.logPrefix()+": failed to unmarshal item", err)
		if s.events.OnError != nil {
			s.events.OnError(err)
		}
		return false
	}

	s.retryCount = 0
	s.events.OnMessage(Meta[T]{
		Created: obj.Created,
		Updated: obj.Updated,
		Index:   obj.Index,
		Data:    item,
	})
	return true
}

// readLoop reads messages from the WebSocket until an error occurs.
func (s *subscribeState[T]) readLoop() {
	for {
		_, message, err := s.wsClient.ReadMessage()
		if err != nil || message == nil {
			s.cfg.console.Err(s.logPrefix()+": read error", err)
			if s.events.OnError != nil {
				s.events.OnError(err)
			}
			s.wsClient.Close()
			return
		}
		if !s.handleMessage(message) {
			s.wsClient.Close()
			return
		}
	}
}

// waitRetry waits before reconnecting based on retry count.
func (s *subscribeState[T]) waitRetry() {
	s.retryCount++
	if s.retryCount < s.cfg.Retry.MediumThreshold {
		s.cfg.console.Log(s.logPrefix() + ": reconnecting...")
		time.Sleep(s.cfg.Retry.InitialDelay)
		return
	}
	if s.retryCount < s.cfg.Retry.MaxThreshold {
		s.cfg.console.Log(s.logPrefix()+": reconnecting in", s.cfg.Retry.MediumDelay)
		time.Sleep(s.cfg.Retry.MediumDelay)
		return
	}
	s.cfg.console.Log(s.logPrefix()+": reconnecting in", s.cfg.Retry.MaxDelay)
	time.Sleep(s.cfg.Retry.MaxDelay)
}

// startCloseWatcher starts a goroutine that closes the connection when context is done.
func (s *subscribeState[T]) startCloseWatcher() {
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

// Subscribe establishes a WebSocket subscription to a single object path (non-glob).
// It automatically reconnects on connection loss with exponential backoff.
// The subscription runs until the context is cancelled.
// For glob patterns (lists), use SubscribeList instead.
func Subscribe[T any](cfg SubscribeConfig, path string, events SubscribeEvents[T]) error {
	err := cfg.Validate()
	if err != nil {
		return err
	}
	if path == "" {
		return ErrPathRequired
	}
	if key.IsGlob(path) {
		return ErrGlobNotAllowed
	}
	err = events.Validate()
	if err != nil {
		return err
	}

	state := &subscribeState[T]{
		cfg:    cfg,
		path:   path,
		events: events,
		wsURL:  url.URL{Scheme: cfg.Server.Protocol, Host: cfg.Server.Host, Path: path},
	}

	state.startCloseWatcher()

	for {
		if state.closing.Load() {
			state.cfg.console.Log(state.logPrefix() + ": skip reconnection, closing...")
			break
		}

		if !state.connect() {
			continue
		}

		state.readLoop()

		if state.closing.Load() {
			state.cfg.console.Log(state.logPrefix() + ": skip reconnection, closing...")
			break
		}

		state.waitRetry()
	}
	return nil
}

// subscribeListState holds the mutable state for a list subscription.
type subscribeListState[T any] struct {
	cfg        SubscribeConfig
	path       string
	events     SubscribeListEvents[T]
	wsURL      url.URL
	cache      json.RawMessage
	retryCount int
	closing    atomic.Bool
	muClient   sync.Mutex
	wsClient   *websocket.Conn
}

// logPrefix returns a consistent log prefix for this subscription.
func (s *subscribeListState[T]) logPrefix() string {
	return "subscribeList[" + s.cfg.Server.Host + "/" + s.path + "]"
}

// connect establishes a WebSocket connection.
// Returns true if connection was successful, false otherwise.
func (s *subscribeListState[T]) connect() bool {
	dialer := &websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: s.cfg.HandshakeTimeout,
	}

	s.muClient.Lock()
	var err error
	s.wsClient, _, err = dialer.Dial(s.wsURL.String(), s.cfg.Header)
	if s.wsClient == nil || err != nil {
		s.muClient.Unlock()
		s.cfg.console.Err(s.logPrefix()+": failed websocket dial", err)
		if s.events.OnError != nil {
			s.events.OnError(err)
		}
		time.Sleep(2 * time.Second)
		return false
	}
	s.muClient.Unlock()

	s.cfg.console.Log(s.logPrefix() + ": connection established")
	return true
}

// handleMessage processes a single WebSocket message.
// Returns false if the read loop should break.
func (s *subscribeListState[T]) handleMessage(message []byte) bool {
	var err error
	var objs []meta.Object
	s.cache, objs, err = messages.PatchList(message, s.cache)
	if err != nil {
		s.cfg.console.Err(s.logPrefix()+": failed to parse list message", err)
		if s.events.OnError != nil {
			s.events.OnError(err)
		}
		return false
	}

	result := make([]Meta[T], 0, len(objs))
	for _, obj := range objs {
		var item T
		err = json.Unmarshal(obj.Data, &item)
		if err != nil {
			s.cfg.console.Err(s.logPrefix()+": failed to unmarshal list item", err)
			if s.events.OnError != nil {
				s.events.OnError(err)
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

	s.retryCount = 0
	s.events.OnMessage(result)
	return true
}

// readLoop reads messages from the WebSocket until an error occurs.
func (s *subscribeListState[T]) readLoop() {
	for {
		_, message, err := s.wsClient.ReadMessage()
		if err != nil || message == nil {
			s.cfg.console.Err(s.logPrefix()+": read error", err)
			if s.events.OnError != nil {
				s.events.OnError(err)
			}
			s.wsClient.Close()
			return
		}
		if !s.handleMessage(message) {
			s.wsClient.Close()
			return
		}
	}
}

// waitRetry waits before reconnecting based on retry count.
func (s *subscribeListState[T]) waitRetry() {
	s.retryCount++
	if s.retryCount < s.cfg.Retry.MediumThreshold {
		s.cfg.console.Log(s.logPrefix() + ": reconnecting...")
		time.Sleep(s.cfg.Retry.InitialDelay)
		return
	}
	if s.retryCount < s.cfg.Retry.MaxThreshold {
		s.cfg.console.Log(s.logPrefix()+": reconnecting in", s.cfg.Retry.MediumDelay)
		time.Sleep(s.cfg.Retry.MediumDelay)
		return
	}
	s.cfg.console.Log(s.logPrefix()+": reconnecting in", s.cfg.Retry.MaxDelay)
	time.Sleep(s.cfg.Retry.MaxDelay)
}

// startCloseWatcher starts a goroutine that closes the connection when context is done.
func (s *subscribeListState[T]) startCloseWatcher() {
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

// SubscribeList establishes a WebSocket subscription to a glob pattern path (list).
// It automatically reconnects on connection loss with exponential backoff.
// The subscription runs until the context is cancelled.
// For single object paths (non-glob), use Subscribe instead.
func SubscribeList[T any](cfg SubscribeConfig, path string, events SubscribeListEvents[T]) error {
	err := cfg.Validate()
	if err != nil {
		return err
	}
	if path == "" {
		return ErrPathRequired
	}
	if !key.IsGlob(path) {
		return ErrGlobRequired
	}
	err = events.Validate()
	if err != nil {
		return err
	}

	state := &subscribeListState[T]{
		cfg:    cfg,
		path:   path,
		events: events,
		wsURL:  url.URL{Scheme: cfg.Server.Protocol, Host: cfg.Server.Host, Path: path},
	}

	state.startCloseWatcher()

	for {
		if state.closing.Load() {
			state.cfg.console.Log(state.logPrefix() + ": skip reconnection, closing...")
			break
		}

		if !state.connect() {
			continue
		}

		state.readLoop()

		if state.closing.Load() {
			state.cfg.console.Log(state.logPrefix() + ": skip reconnection, closing...")
			break
		}

		state.waitRetry()
	}
	return nil
}
