package proxy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/benitogf/coat"
	"github.com/benitogf/ooo"
	"github.com/benitogf/ooo/client"
	"github.com/benitogf/ooo/meta"
	"github.com/benitogf/ooo/ui"
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

// Capabilities defines what operations are allowed on a proxy route.
// When nil in Config, all capabilities default to true.
type Capabilities struct {
	Read   bool
	Write  bool
	Delete bool
}

// Config for a proxy route.
type Config struct {
	Resolve   Resolver
	Subscribe *client.SubscribeConfig
	// Optional capability overrides. If nil, all capabilities are true.
	// Use &Capabilities{Read: true, Write: false, Delete: false} to restrict.
	Capabilities *Capabilities
}

// Validate checks that required fields are set.
func (c *Config) Validate() error {
	if c.Resolve == nil {
		return ErrResolverRequired
	}
	return nil
}

// canRead returns whether read is enabled (defaults to true)
func (c *Config) canRead() bool {
	if c.Capabilities == nil {
		return true
	}
	return c.Capabilities.Read
}

// canWrite returns whether write is enabled (defaults to true)
func (c *Config) canWrite() bool {
	if c.Capabilities == nil {
		return true
	}
	return c.Capabilities.Write
}

// canDelete returns whether delete is enabled (defaults to true)
func (c *Config) canDelete() bool {
	if c.Capabilities == nil {
		return true
	}
	return c.Capabilities.Delete
}

// proxyManager manages shared remote subscriptions.
type proxyManager struct {
	mu      sync.RWMutex
	states  map[string]*proxyState // key: "address/remotePath"
	console *coat.Console
}

// proxyState holds state for a shared remote subscription.
type wsMessage struct {
	msgType int
	data    []byte
}

type proxyState struct {
	mu              sync.Mutex
	wg              sync.WaitGroup // tracks remote subscription goroutine
	address         string
	remotePath      string
	localSubs       map[*websocket.Conn]chan wsMessage
	cache           wsMessage
	cancel          context.CancelFunc
	console         *coat.Console
	cfg             *client.SubscribeConfig
	isList          bool
	remoteConnected bool // true after remote subscription is established
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
			localSubs:  make(map[*websocket.Conn]chan wsMessage),
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

// CloseAll closes all proxy subscriptions managed by this proxy manager.
func (pm *proxyManager) CloseAll() {
	pm.mu.Lock()
	states := make([]*proxyState, 0, len(pm.states))
	for _, state := range pm.states {
		states = append(states, state)
	}
	pm.states = make(map[string]*proxyState)
	pm.mu.Unlock()

	for _, state := range states {
		state.stopRemoteSubscription()
		state.closeAllSubscribers()
	}
}

func (ps *proxyState) logPrefix() string {
	return "proxy[" + ps.address + "/" + ps.remotePath + "]"
}

func (ps *proxyState) addSubscriber(conn *websocket.Conn) chan wsMessage {
	msgChan := make(chan wsMessage, 10)
	ps.mu.Lock()
	wasEmpty := len(ps.localSubs) == 0
	remoteAlreadyConnected := ps.remoteConnected
	ps.localSubs[conn] = msgChan
	ps.mu.Unlock()

	if wasEmpty {
		// First subscriber - start remote subscription
		// The remote will send us the initial snapshot which we'll forward
		ps.startRemoteSubscription()
	} else if remoteAlreadyConnected {
		// New subscriber joining an active connection
		// Fetch fresh state via HTTP and send as snapshot
		go ps.sendInitialState(msgChan)
	}

	return msgChan
}

// sendInitialState fetches current state via HTTP and sends it as a snapshot to the subscriber
func (ps *proxyState) sendInitialState(msgChan chan wsMessage) {
	var header http.Header
	if ps.cfg != nil {
		header = ps.cfg.Header
	}

	scheme := "http"
	if ps.cfg != nil && ps.cfg.Server.Protocol == "wss" {
		scheme = "https"
	}

	targetURL := scheme + "://" + ps.address + "/" + ps.remotePath
	httpClient := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		ps.console.Err(ps.logPrefix()+": failed to create initial state request", err)
		return
	}
	for k, v := range header {
		req.Header[k] = v
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		ps.console.Err(ps.logPrefix()+": failed to fetch initial state", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Key doesn't exist yet - send empty snapshot matching what the ooo server sends:
		// - Lists (glob keys): empty array []
		// - Objects: empty meta object with created/updated = 0
		var emptyData []byte
		if ps.isList {
			emptyData = []byte("[]")
		} else {
			emptyData = meta.EmptyObject
		}
		snapshotMsg := buildSnapshotMessage(emptyData)
		select {
		case msgChan <- wsMessage{msgType: websocket.BinaryMessage, data: snapshotMsg}:
		default:
		}
		return
	}

	if resp.StatusCode >= 400 {
		ps.console.Err(ps.logPrefix()+": failed to fetch initial state", errors.New(resp.Status))
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		ps.console.Err(ps.logPrefix()+": failed to read initial state", err)
		return
	}

	// Build a snapshot message in the ooo format: {"snapshot":true,"version":"0","data":...}
	snapshotMsg := buildSnapshotMessage(body)

	select {
	case msgChan <- wsMessage{msgType: websocket.BinaryMessage, data: snapshotMsg}:
	default:
		// Channel full, skip
	}
}

// buildSnapshotMessage wraps data in the ooo WebSocket message format with snapshot=true
func buildSnapshotMessage(data []byte) []byte {
	// Format: {"snapshot":true,"version":"0","data":DATA}
	capacity := 40 + len(data)
	buf := make([]byte, 0, capacity)
	buf = append(buf, `{"snapshot":true,"version":"0","data":`...)
	buf = append(buf, data...)
	buf = append(buf, '}')
	return buf
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

func (ps *proxyState) broadcastRaw(msgType int, data []byte) {
	ps.mu.Lock()
	msg := wsMessage{msgType: msgType, data: data}
	ps.cache = msg // Update cache so new subscribers get initial data
	for _, ch := range ps.localSubs {
		select {
		case ch <- msg:
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

	ps.wg.Add(1)
	go func() {
		defer ps.wg.Done()
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

		// Mark as connected so new subscribers know to fetch via HTTP
		ps.mu.Lock()
		ps.remoteConnected = true
		ps.mu.Unlock()

		// Read messages from remote and broadcast to local subscribers
		// Forward raw messages as-is - the ooo client expects the same format
		for {
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				ps.console.Err(ps.logPrefix()+": remote read error", err)
				ps.mu.Lock()
				ps.remoteConnected = false
				ps.mu.Unlock()
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
	cancel := ps.cancel
	ps.cancel = nil
	ps.mu.Unlock()

	if cancel != nil {
		cancel()
		ps.wg.Wait() // Wait for remote subscription goroutine to finish
	}
}

// GlobToMuxPattern converts ooo-style glob pattern to mux-compatible pattern.
// "states/*" -> "states/{path:.*}"
// "devices/*/things/*" -> "devices/{path1}/things/{path2:.*}"
func GlobToMuxPattern(pattern string) string {
	parts := strings.Split(pattern, "/")
	varCount := 0
	for i, part := range parts {
		if part == "*" {
			varCount++
			// Last wildcard gets .* to match anything including slashes
			if i == len(parts)-1 {
				parts[i] = "{path" + string(rune('0'+varCount)) + ":.*}"
			} else {
				parts[i] = "{path" + string(rune('0'+varCount)) + "}"
			}
		}
	}
	return strings.Join(parts, "/")
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

	// Convert glob pattern to mux pattern
	muxPattern := GlobToMuxPattern(localPath)

	// Register the route
	server.Router.HandleFunc("/"+muxPattern, handler)

	// Register for UI visibility
	server.RegisterProxy(ui.ProxyInfo{
		LocalPath: localPath,
		Type:      "single",
		CanRead:   cfg.canRead(),
		CanWrite:  cfg.canWrite(),
		CanDelete: cfg.canDelete(),
	})

	// Register cleanup function to be called when server closes
	server.RegisterProxyCleanup(pm.CloseAll)

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

	// Convert glob pattern to mux pattern
	muxPattern := GlobToMuxPattern(localPath)

	// Register routes - specific path first, then base
	server.Router.HandleFunc("/"+muxPattern, itemHandler)
	server.Router.HandleFunc("/"+basePath, listHandler)

	// Register for UI visibility
	server.RegisterProxy(ui.ProxyInfo{
		LocalPath: localPath,
		Type:      "list",
		CanRead:   cfg.canRead(),
		CanWrite:  cfg.canWrite(),
		CanDelete: cfg.canDelete(),
	})

	// Register cleanup function to be called when server closes
	server.RegisterProxyCleanup(pm.CloseAll)

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
			if err := localConn.WriteMessage(msg.msgType, msg.data); err != nil {
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

	case http.MethodPatch:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		req, err := http.NewRequestWithContext(r.Context(), http.MethodPatch, targetURL, bytes.NewReader(body))
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

	// Register for UI visibility
	server.RegisterProxy(ui.ProxyInfo{
		LocalPath: localPath,
		Type:      "vars",
		CanRead:   cfg.canRead(),
		CanWrite:  cfg.canWrite(),
		CanDelete: cfg.canDelete(),
	})

	return nil
}

// Node represents a network node with IP and Port for proxy routing.
type Node struct {
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

// NodeFilterConfig configures RouteNodeFilter for routing requests to nodes.
type NodeFilterConfig struct {
	// NodesKey is the glob pattern to look up nodes (e.g., "devices/*")
	NodesKey string
	// LocalKey is the local path pattern to expose (e.g., "states/*")
	LocalKey string
	// RemoteKey is the path to access on the remote node (e.g., "state")
	RemoteKey string
	// Subscribe configures WebSocket subscription options
	Subscribe *client.SubscribeConfig
	// Capabilities overrides (if nil, all are true)
	Capabilities *Capabilities
}

// RouteNodeFilter sets up a proxy route that looks up nodes from storage and routes
// requests to them. This is a convenience wrapper for the common pattern of
// routing requests to nodes stored in a list.
//
// The node data must have "ip" and "port" JSON fields.
//
// Example:
//
//	proxy.RouteNodeFilter(server, proxy.NodeFilterConfig{
//	    NodesKey:  "devices/*",
//	    LocalKey:  "states/*",
//	    RemoteKey: "state",
//	})
func RouteNodeFilter(server *ooo.Server, cfg NodeFilterConfig) error {
	if cfg.NodesKey == "" || cfg.LocalKey == "" || cfg.RemoteKey == "" {
		return errors.New("proxy: NodesKey, LocalKey, and RemoteKey are required")
	}

	// Extract the base path from LocalKey (e.g., "states/*" -> "states/")
	basePath := strings.TrimSuffix(cfg.LocalKey, "*")

	proxyCfg := Config{
		Resolve: func(localPath string) (string, string, error) {
			// Extract the node ID from the local path
			nodeID := strings.TrimPrefix(localPath, basePath)

			// Look up nodes from storage
			nodes, err := ooo.GetList[Node](server, cfg.NodesKey)
			if err != nil {
				return "", "", err
			}

			// Find the matching node
			for _, node := range nodes {
				if node.Index == nodeID {
					return node.Data.IP + ":" + strconv.Itoa(node.Data.Port), cfg.RemoteKey, nil
				}
			}
			return "", "", errors.New("proxy: node not found: " + nodeID)
		},
		Subscribe:    cfg.Subscribe,
		Capabilities: cfg.Capabilities,
	}

	return Route(server, cfg.LocalKey, proxyCfg)
}

// NodeListFilterConfig configures RouteNodeListFilter for routing list requests to nodes.
type NodeListFilterConfig struct {
	// NodesKey is the glob pattern to look up nodes (e.g., "devices/*")
	NodesKey string
	// LocalKey is the local path pattern to expose (e.g., "device/logs/*/*")
	// The second-to-last wildcard is the node ID, the last wildcard is for list items
	LocalKey string
	// RemoteKey is the list path to access on the remote node (e.g., "logs/*")
	RemoteKey string
	// Subscribe configures WebSocket subscription options
	Subscribe *client.SubscribeConfig
	// Capabilities overrides (if nil, all are true)
	Capabilities *Capabilities
}

// RouteNodeListFilter sets up a proxy route that looks up nodes from storage and routes
// list requests to them. This is for proxying list data from nodes.
//
// The local path pattern should have at least two wildcards:
// - Second-to-last wildcard: node ID
// - Last wildcard: list item ID
//
// Example:
//
//	proxy.RouteNodeListFilter(server, proxy.NodeListFilterConfig{
//	    NodesKey:  "devices/*",
//	    LocalKey:  "device/logs/*/*",
//	    RemoteKey: "logs/*",
//	})
//
// This routes requests like "device/logs/node123/item456" to "logs/item456" on node123.
func RouteNodeListFilter(server *ooo.Server, cfg NodeListFilterConfig) error {
	if cfg.NodesKey == "" || cfg.LocalKey == "" || cfg.RemoteKey == "" {
		return errors.New("proxy: NodesKey, LocalKey, and RemoteKey are required")
	}

	// Parse the local key to find wildcard positions
	localParts := strings.Split(cfg.LocalKey, "/")
	wildcardPositions := []int{}
	for i, part := range localParts {
		if part == "*" {
			wildcardPositions = append(wildcardPositions, i)
		}
	}

	if len(wildcardPositions) < 2 {
		return errors.New("proxy: LocalKey must have at least two wildcards for node list filter")
	}

	// The node ID is at the second-to-last wildcard position
	nodeIDPos := wildcardPositions[len(wildcardPositions)-2]

	// Build base path up to (but not including) the node ID wildcard
	baseParts := localParts[:nodeIDPos]
	basePath := strings.Join(baseParts, "/")
	if basePath != "" {
		basePath += "/"
	}

	// Remote base path (without trailing wildcard)
	remoteBasePath := strings.TrimSuffix(cfg.RemoteKey, "*")

	proxyCfg := Config{
		Resolve: func(localPath string) (string, string, error) {
			// Parse the path to extract node ID and item path
			pathParts := strings.Split(localPath, "/")

			if len(pathParts) <= nodeIDPos {
				return "", "", errors.New("proxy: invalid path for node list filter")
			}

			nodeID := pathParts[nodeIDPos]

			// Build the remote path: everything after the node ID becomes the item path
			var remotePath string
			if len(pathParts) > nodeIDPos+1 {
				// Has item ID - route to specific item
				itemParts := pathParts[nodeIDPos+1:]
				remotePath = remoteBasePath + strings.Join(itemParts, "/")
			} else {
				// No item ID - route to list base (for subscriptions)
				remotePath = cfg.RemoteKey
			}

			// Look up nodes from storage
			nodes, err := ooo.GetList[Node](server, cfg.NodesKey)
			if err != nil {
				return "", "", err
			}

			// Find the matching node
			for _, node := range nodes {
				if node.Index == nodeID {
					return node.Data.IP + ":" + strconv.Itoa(node.Data.Port), remotePath, nil
				}
			}
			return "", "", errors.New("proxy: node not found: " + nodeID)
		},
		Subscribe:    cfg.Subscribe,
		Capabilities: cfg.Capabilities,
	}

	return RouteList(server, cfg.LocalKey, proxyCfg)
}
