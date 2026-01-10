package proxy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/benitogf/coat"
	"github.com/benitogf/ooo"
	"github.com/benitogf/ooo/client"
	"github.com/benitogf/ooo/meta"
	"github.com/goccy/go-json"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

var (
	ErrResolverRequired = errors.New("proxy: Resolver is required")
	ErrLocalPathEmpty   = errors.New("proxy: LocalPath is required")
	ErrResolveFailed    = errors.New("proxy: failed to resolve target")
)

// Resolver maps local path to remote server address and path.
// localPath example: "settings/device123"
// returns: address="192.168.1.100:8080", remotePath="settings"
type Resolver func(localPath string) (address, remotePath string, err error)

// Config for a proxy route.
type Config struct {
	Resolve   Resolver
	Subscribe *client.SubscribeConfig
}

// Validate checks that required fields are set.
func (c *Config) Validate() error {
	if c.Resolve == nil {
		return ErrResolverRequired
	}
	return nil
}

// proxyManager manages shared remote subscriptions.
type proxyManager struct {
	mu      sync.RWMutex
	states  map[string]*proxyState // key: "address/remotePath"
	console *coat.Console
}

// proxyState holds state for a shared remote subscription.
type proxyState struct {
	mu         sync.RWMutex
	address    string
	remotePath string
	cache      json.RawMessage
	localSubs  map[*websocket.Conn]chan []byte // local subscribers with their message channels
	cancel     context.CancelFunc
	console    *coat.Console
	cfg        *client.SubscribeConfig
	isList     bool
}

func newProxyManager(silence bool) *proxyManager {
	return &proxyManager{
		states:  make(map[string]*proxyState),
		console: coat.NewConsole("proxy", silence),
	}
}

func (pm *proxyManager) getOrCreateState(address, remotePath string, cfg *client.SubscribeConfig, isList bool) *proxyState {
	key := address + "/" + remotePath

	pm.mu.Lock()
	defer pm.mu.Unlock()

	state, exists := pm.states[key]
	if !exists {
		state = &proxyState{
			address:    address,
			remotePath: remotePath,
			localSubs:  make(map[*websocket.Conn]chan []byte),
			console:    pm.console,
			cfg:        cfg,
			isList:     isList,
		}
		pm.states[key] = state
	}
	return state
}

func (pm *proxyManager) removeState(address, remotePath string) {
	key := address + "/" + remotePath
	pm.mu.Lock()
	delete(pm.states, key)
	pm.mu.Unlock()
}

func (ps *proxyState) logPrefix() string {
	return "proxy[" + ps.address + "/" + ps.remotePath + "]"
}

func (ps *proxyState) addSubscriber(conn *websocket.Conn) chan []byte {
	msgChan := make(chan []byte, 10)
	ps.mu.Lock()
	wasEmpty := len(ps.localSubs) == 0
	ps.localSubs[conn] = msgChan

	// Send cached data immediately if available
	if len(ps.cache) > 0 {
		select {
		case msgChan <- ps.cache:
		default:
		}
	}
	ps.mu.Unlock()

	if wasEmpty {
		ps.startRemoteSubscription()
	}

	return msgChan
}

func (ps *proxyState) removeSubscriber(conn *websocket.Conn) bool {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if ch, ok := ps.localSubs[conn]; ok {
		close(ch)
		delete(ps.localSubs, conn)
	}

	// Return true if no more subscribers
	return len(ps.localSubs) == 0
}

func (ps *proxyState) broadcast(data []byte) {
	ps.mu.Lock()
	ps.cache = data
	for _, ch := range ps.localSubs {
		select {
		case ch <- data:
		default:
			// Channel full, skip this message
		}
	}
	ps.mu.Unlock()
}

func (ps *proxyState) broadcastRaw(_ int, data []byte) {
	ps.mu.Lock()
	ps.cache = data // Update cache so new subscribers get initial data
	for _, ch := range ps.localSubs {
		select {
		case ch <- data:
		default:
			// Channel full, skip this message
		}
	}
	ps.mu.Unlock()
}

func (ps *proxyState) closeAllSubscribers() {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	for conn, ch := range ps.localSubs {
		close(ch)
		conn.Close()
		delete(ps.localSubs, conn)
	}
}

func (ps *proxyState) startRemoteSubscription() {
	ctx, cancel := context.WithCancel(context.Background())
	ps.cancel = cancel

	go func() {
		wsScheme := "ws"
		if ps.cfg != nil && ps.cfg.Server.Protocol == "wss" {
			wsScheme = "wss"
		}

		wsURL := url.URL{
			Scheme: wsScheme,
			Host:   ps.address,
			Path:   ps.remotePath,
		}

		dialer := &websocket.Dialer{
			Proxy:            http.ProxyFromEnvironment,
			HandshakeTimeout: 2 * time.Second,
		}
		if ps.cfg != nil && ps.cfg.HandshakeTimeout > 0 {
			dialer.HandshakeTimeout = ps.cfg.HandshakeTimeout
		}

		var header http.Header
		if ps.cfg != nil {
			header = ps.cfg.Header
		}

		ps.console.Log(ps.logPrefix() + ": connecting to remote")

		conn, _, err := dialer.Dial(wsURL.String(), header)
		if err != nil {
			ps.console.Err(ps.logPrefix()+": failed to connect to remote", err)
			ps.closeAllSubscribers()
			return
		}

		// Set up close handler
		go func() {
			<-ctx.Done()
			conn.Close()
		}()

		ps.console.Log(ps.logPrefix() + ": connected to remote")

		// Read messages from remote and broadcast to local subscribers
		// Forward raw messages as-is - the ooo client expects the same format
		for {
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				ps.console.Err(ps.logPrefix()+": remote read error", err)
				ps.closeAllSubscribers()
				conn.Close()
				return
			}

			// Forward raw message to all local subscribers
			ps.broadcastRaw(messageType, message)
		}
	}()
}

func (ps *proxyState) stopRemoteSubscription() {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.cancel != nil {
		ps.cancel()
		ps.cancel = nil
	}
}

// Route registers a proxy for a single-key route pattern.
// localPath should be a pattern like "settings/*" where the wildcard is used for resolution.
func Route(server *ooo.Server, localPath string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if localPath == "" {
		return ErrLocalPathEmpty
	}

	// Initialize router if not already set
	if server.Router == nil {
		server.Router = mux.NewRouter()
	}

	silence := false
	if cfg.Subscribe != nil {
		silence = cfg.Subscribe.Silence
	}
	pm := newProxyManager(silence)

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	handler := func(w http.ResponseWriter, r *http.Request) {
		// Get the actual path from the request
		actualPath := strings.TrimPrefix(r.URL.Path, "/")

		address, remotePath, err := cfg.Resolve(actualPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Check if this is a WebSocket upgrade request
		if websocket.IsWebSocketUpgrade(r) {
			handleWebSocketProxy(w, r, pm, address, remotePath, cfg.Subscribe, upgrader, false)
			return
		}

		// Handle HTTP requests
		handleHTTPProxy(w, r, address, remotePath, cfg.Subscribe)
	}

	// Register the route
	server.Router.HandleFunc("/"+localPath, handler)

	return nil
}

// RouteList registers a proxy for a glob route pattern.
// localPath should be a pattern like "devices/*/things/*".
func RouteList(server *ooo.Server, localPath string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if localPath == "" {
		return ErrLocalPathEmpty
	}

	// Initialize router if not already set
	if server.Router == nil {
		server.Router = mux.NewRouter()
	}

	silence := false
	if cfg.Subscribe != nil {
		silence = cfg.Subscribe.Silence
	}
	pm := newProxyManager(silence)

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	// For list routes, we need to handle both the base path and specific items
	basePath := strings.TrimSuffix(localPath, "/*")

	// Handler for the base path (list)
	listHandler := func(w http.ResponseWriter, r *http.Request) {
		actualPath := strings.TrimPrefix(r.URL.Path, "/")

		address, remotePath, err := cfg.Resolve(actualPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if websocket.IsWebSocketUpgrade(r) {
			handleWebSocketProxy(w, r, pm, address, remotePath, cfg.Subscribe, upgrader, true)
			return
		}

		handleHTTPProxy(w, r, address, remotePath, cfg.Subscribe)
	}

	// Handler for specific items
	itemHandler := func(w http.ResponseWriter, r *http.Request) {
		actualPath := strings.TrimPrefix(r.URL.Path, "/")

		address, remotePath, err := cfg.Resolve(actualPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if websocket.IsWebSocketUpgrade(r) {
			handleWebSocketProxy(w, r, pm, address, remotePath, cfg.Subscribe, upgrader, false)
			return
		}

		handleHTTPProxy(w, r, address, remotePath, cfg.Subscribe)
	}

	// Register routes - specific path first, then base
	server.Router.HandleFunc("/"+localPath, itemHandler)
	server.Router.HandleFunc("/"+basePath, listHandler)

	return nil
}

func handleWebSocketProxy(w http.ResponseWriter, r *http.Request, pm *proxyManager, address, remotePath string, subCfg *client.SubscribeConfig, upgrader websocket.Upgrader, isList bool) {
	// Upgrade local connection
	localConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	state := pm.getOrCreateState(address, remotePath, subCfg, isList)
	msgChan := state.addSubscriber(localConn)

	// Forward messages to local subscriber
	go func() {
		defer func() {
			if state.removeSubscriber(localConn) {
				state.stopRemoteSubscription()
				pm.removeState(address, remotePath)
			}
			localConn.Close()
		}()

		for msg := range msgChan {
			if err := localConn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		}
	}()

	// Read from local connection (to detect close)
	for {
		_, _, err := localConn.ReadMessage()
		if err != nil {
			return
		}
	}
}

func handleHTTPProxy(w http.ResponseWriter, r *http.Request, address, remotePath string, subCfg *client.SubscribeConfig) {
	scheme := "http"
	if subCfg != nil && subCfg.Server.Protocol == "wss" {
		scheme = "https"
	}

	targetURL := scheme + "://" + address + "/" + remotePath

	var header http.Header
	if subCfg != nil {
		header = subCfg.Header
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}

	switch r.Method {
	case http.MethodGet:
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, targetURL, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for k, v := range header {
			req.Header[k] = v
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		// Copy response headers
		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)

	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, targetURL, bytes.NewReader(body))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		for k, v := range header {
			req.Header[k] = v
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)

	case http.MethodDelete:
		req, err := http.NewRequestWithContext(r.Context(), http.MethodDelete, targetURL, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for k, v := range header {
			req.Header[k] = v
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// RouteWithVars registers a proxy route using mux path variables.
// This is useful when the local path uses mux-style variables like "settings/{deviceID}".
func RouteWithVars(server *ooo.Server, localPath string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if localPath == "" {
		return ErrLocalPathEmpty
	}

	silence := false
	if cfg.Subscribe != nil {
		silence = cfg.Subscribe.Silence
	}
	pm := newProxyManager(silence)

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	handler := func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		actualPath := strings.TrimPrefix(r.URL.Path, "/")

		// Include vars in the path for resolution
		_ = vars // available for custom resolvers via closure

		address, remotePath, err := cfg.Resolve(actualPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if websocket.IsWebSocketUpgrade(r) {
			handleWebSocketProxy(w, r, pm, address, remotePath, cfg.Subscribe, upgrader, false)
			return
		}

		handleHTTPProxy(w, r, address, remotePath, cfg.Subscribe)
	}

	server.Router.HandleFunc("/"+localPath, handler)

	return nil
}

// getFullState retrieves the current full state from the remote server.
// This is used to get initial data for new subscribers.
func getFullState(address, remotePath string, header http.Header) (json.RawMessage, error) {
	scheme := "http"
	targetURL := scheme + "://" + address + "/" + remotePath

	httpClient := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range header {
		req.Header[k] = v
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, errors.New("failed to get initial state: " + resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Decode as meta.Object to get just the data
	obj, err := meta.Decode(body)
	if err != nil {
		// Try as list
		objs, err := meta.DecodeList(body)
		if err != nil {
			return nil, err
		}
		// Re-encode the list
		return json.Marshal(objs)
	}

	// Return the full object JSON for caching
	return json.Marshal(obj)
}
