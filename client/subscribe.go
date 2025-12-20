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
)

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
// Required fields: Ctx, Protocol, Host.
// Optional fields: Header, HandshakeTimeout, Retry, Silence.
type SubscribeConfig struct {
	Ctx              context.Context
	Protocol         string
	Host             string
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
	if c.Protocol == "" {
		return ErrProtocolRequired
	}
	if c.Host == "" {
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
	c.console = coat.NewConsole(c.Host, c.Silence)
	return nil
}

// SubscribeEvents holds event callbacks for Subscribe.
// Required: OnMessage.
// Optional: OnError, OnOpen, OnClose.
type SubscribeEvents[T any] struct {
	OnMessage func([]Meta[T])
	OnError   func(error)
	OnOpen    func()
	OnClose   func()
}

// Validate checks that required callbacks are set.
func (e *SubscribeEvents[T]) Validate() error {
	if e.OnMessage == nil {
		return ErrOnMessageRequired
	}
	return nil
}

// subscribeState holds the mutable state for a subscription.
type subscribeState[T any] struct {
	cfg        SubscribeConfig
	path       string
	events     SubscribeEvents[T]
	isList     bool
	wsURL      url.URL
	cache      json.RawMessage
	retryCount int
	closing    atomic.Bool
	muClient   sync.Mutex
	wsClient   *websocket.Conn
}

// logPrefix returns a consistent log prefix for this subscription.
func (s *subscribeState[T]) logPrefix() string {
	return "subscribe[" + s.cfg.Host + "/" + s.path + "]"
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
	if s.events.OnOpen != nil {
		s.events.OnOpen()
	}
	return true
}

// handleMessage processes a single WebSocket message.
// Returns false if the read loop should break.
func (s *subscribeState[T]) handleMessage(message []byte) bool {
	var err error
	result := []Meta[T]{}

	if s.isList {
		var objs []meta.Object
		s.cache, objs, err = messages.PatchList(message, s.cache)
		if err != nil {
			s.cfg.console.Err(s.logPrefix()+": failed to parse list message", err)
			if s.events.OnError != nil {
				s.events.OnError(err)
			}
			return false
		}
		for _, obj := range objs {
			var item T
			if err = json.Unmarshal(obj.Data, &item); err != nil {
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
	} else {
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
		if err = json.Unmarshal(obj.Data, &item); err != nil {
			s.cfg.console.Err(s.logPrefix()+": failed to unmarshal item", err)
			if s.events.OnError != nil {
				s.events.OnError(err)
			}
			return false
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
		if s.events.OnClose != nil {
			s.events.OnClose()
		}
	}()
}

// Subscribe establishes a WebSocket subscription to the given path.
// It automatically reconnects on connection loss with exponential backoff.
// The subscription runs until the context is cancelled.
func Subscribe[T any](cfg SubscribeConfig, path string, events SubscribeEvents[T]) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if path == "" {
		return ErrPathRequired
	}
	if err := events.Validate(); err != nil {
		return err
	}

	state := &subscribeState[T]{
		cfg:    cfg,
		path:   path,
		events: events,
		isList: key.LastIndex(path) == "*",
		wsURL:  url.URL{Scheme: cfg.Protocol, Host: cfg.Host, Path: path},
	}

	state.startCloseWatcher()

	for {
		if !state.connect() {
			continue
		}

		state.readLoop()

		if state.closing.Load() {
			state.cfg.console.Log(state.logPrefix() + ": skip reconnection, closing...")
			break
		}

		if state.events.OnClose != nil {
			state.events.OnClose()
		}

		state.waitRetry()
	}
	return nil
}
