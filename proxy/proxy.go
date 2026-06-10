package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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

// teardownPauseHook is a test seam invoked inside the WS handler's
// teardown between stopRemoteSubscription and removeState. afterTeardownHook
// fires immediately after removeState returns. Production callers leave
// both nil so the calls are zero-cost branches. Tests set them to
// barrier-based callbacks so the lifecycle race window between "state
// is being torn down" and "new subscriber arrives for the same path"
// can be exercised — and verified to have settled — deterministically.
//
// Wrapped in atomic.Pointer so the WS handler can read them racelessly
// while a test goroutine concurrently installs or clears via the
// SetTeardown*ForTest setters.
var (
	teardownPauseHook atomic.Pointer[func()]
	afterTeardownHook atomic.Pointer[func()]
)

// SetTeardownPauseHookForTest installs a callback invoked inside the
// WS handler's teardown between stopRemoteSubscription and removeState.
// Tests only — production callers leave the hook nil. Pass nil to
// clear. Single-shot: tests must not assume the hook fires more than
// once per install. Not safe for concurrent test execution; tests
// using this hook must not run with t.Parallel.
func SetTeardownPauseHookForTest(fn func()) {
	if fn == nil {
		teardownPauseHook.Store(nil)
		return
	}
	teardownPauseHook.Store(&fn)
}

// SetAfterTeardownHookForTest installs a callback invoked inside the
// WS handler's teardown after removeState returns. Tests use this to
// know when a subscriber teardown has fully settled before introducing
// the next subscriber. Same constraints as SetTeardownPauseHookForTest.
func SetAfterTeardownHookForTest(fn func()) {
	if fn == nil {
		afterTeardownHook.Store(nil)
		return
	}
	afterTeardownHook.Store(&fn)
}

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
	// Description for UI display
	Description string
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

// enforceCapability writes 403 + returns false when the request method
// is denied by the route's Capabilities. WebSocket upgrade requests are
// gated by Read (a proxied WS subscription is a read on this codebase).
// Unknown methods are passed through and rejected as 405 by the
// downstream handler. The check fires before Resolve and before any
// upstream contact, so denied requests never reach the target.
func (c *Config) enforceCapability(w http.ResponseWriter, r *http.Request) bool {
	if c.Capabilities == nil {
		return true
	}
	if websocket.IsWebSocketUpgrade(r) {
		if c.canRead() {
			return true
		}
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return false
	}
	allowed := true
	switch r.Method {
	case http.MethodGet:
		allowed = c.canRead()
	case http.MethodPost, http.MethodPatch:
		allowed = c.canWrite()
	case http.MethodDelete:
		allowed = c.canDelete()
	}
	if !allowed {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return false
	}
	return true
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
	mu sync.Mutex
	wg sync.WaitGroup // tracks remote subscription goroutine
	// dying flips to true under mu as soon as the last subscriber
	// leaves, so a concurrent addSubscriber on the same state pointer
	// can detect the in-progress teardown and ask the caller to
	// refetch from the manager. getOrCreateState also consults it so
	// the registry hands out a fresh state instead of the dying one.
	dying           bool
	address         string
	remotePath      string
	localSubs       map[*websocket.Conn]chan wsMessage
	cache           wsMessage
	cancel          context.CancelFunc
	console         *coat.Console
	cfg             *client.SubscribeConfig
	isList          bool
	remoteConnected bool   // true after remote subscription is established
	clientProtocol  string // Sec-Websocket-Protocol from first connecting client
}

func newProxyManager(silence bool) *proxyManager {
	return &proxyManager{
		states:  make(map[string]*proxyState),
		console: coat.NewConsole("proxy", silence),
	}
}

func (pm *proxyManager) getOrCreateState(address, remotePath string, cfg *client.SubscribeConfig, isList bool, clientProtocol string) *proxyState {
	key := address + "/" + remotePath

	pm.mu.Lock()
	defer pm.mu.Unlock()

	state, exists := pm.states[key]
	if exists {
		state.mu.Lock()
		if state.dying {
			// A teardown for this state is mid-flight and has not yet
			// reached removeState. Replace it in the registry with a
			// fresh state so this new subscriber starts its own
			// remote subscription instead of attaching to the
			// half-torn-down one. The dying state's removeState will
			// see the pointer mismatch and skip its delete.
			state.mu.Unlock()
			exists = false
		} else {
			// Reusable state: update protocol for the new client
			// (auth credentials may differ from the previous client).
			state.clientProtocol = clientProtocol
			state.mu.Unlock()
		}
	}
	if !exists {
		state = &proxyState{
			address:        address,
			remotePath:     remotePath,
			localSubs:      make(map[*websocket.Conn]chan wsMessage),
			console:        pm.console,
			cfg:            cfg,
			isList:         isList,
			clientProtocol: clientProtocol,
		}
		pm.states[key] = state
	}
	return state
}

// removeState deletes the state for (address, remotePath) from the
// registry, but only if the registered pointer still matches the one
// the caller observed. After a teardown stops the remote subscription,
// a concurrent getOrCreateState may have already replaced the dying
// state with a fresh one; compare-and-delete prevents the teardown
// from evicting that fresh state.
func (pm *proxyManager) removeState(address, remotePath string, expected *proxyState) {
	key := address + "/" + remotePath
	pm.mu.Lock()
	if pm.states[key] == expected {
		delete(pm.states, key)
	}
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

// addSubscriber registers conn with the state and returns the
// per-subscriber outbox. Returns nil if the state is being torn down
// — the caller must refetch a fresh state from the manager and try
// again.
func (ps *proxyState) addSubscriber(conn *websocket.Conn) chan wsMessage {
	msgChan := make(chan wsMessage, 10)
	ps.mu.Lock()
	if ps.dying {
		ps.mu.Unlock()
		return nil
	}
	wasEmpty := len(ps.localSubs) == 0
	remoteAlreadyConnected := ps.remoteConnected
	ps.localSubs[conn] = msgChan
	ps.mu.Unlock()

	if wasEmpty {
		// Wait for any in-flight remote subscription goroutine to finish
		// before starting a new one — prevents using stale state.
		ps.wg.Wait()
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
	// Recover from send on closed channel - subscriber may disconnect during HTTP fetch
	defer func() {
		recover()
	}()

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

	// Flip dying as soon as the state empties so a concurrent
	// addSubscriber on the same pointer (one that observed the state
	// before getOrCreateState had a chance to replace it) can detect
	// the in-progress teardown. Pairs with the dying check in
	// addSubscriber and the registry replacement in getOrCreateState.
	if len(ps.localSubs) == 0 {
		ps.dying = true
		return true
	}
	return false
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
		// Forward client's Sec-Websocket-Protocol header to remote for auth
		ps.mu.Lock()
		protocol := ps.clientProtocol
		ps.mu.Unlock()
		if protocol != "" {
			if header == nil {
				header = http.Header{}
			}
			header.Set("Sec-Websocket-Protocol", protocol)
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
				parts[i] = "{path" + strconv.Itoa(varCount) + ":.*}"
			} else {
				parts[i] = "{path" + strconv.Itoa(varCount) + "}"
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
		CheckOrigin:  server.Stream.CheckOrigin,
		Subprotocols: []string{"bearer"},
	}

	handler := func(w http.ResponseWriter, r *http.Request) {
		if !cfg.enforceCapability(w, r) {
			return
		}
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
		handleHTTPProxy(server, w, r, address, remotePath, cfg.Subscribe)
	}

	// Convert glob pattern to mux pattern
	muxPattern := GlobToMuxPattern(localPath)

	// Register the route and mirror onto the route oracle so the data
	// wildcard defers to this proxy regardless of registration order.
	// Auth gating is handled by middleware registered via
	// server.Router.Use(...); gorilla/mux's Match applies the chain
	// before dispatch, so the proxy handler runs after middleware just
	// like built-in REST handlers. RouterMutate serializes this with
	// the syncRouter wrapper's Match.
	server.RouterMutate(func() {
		server.Router.HandleFunc("/"+muxPattern, handler)
	})
	server.RegisterOracleRoute("/"+muxPattern, nil)

	// Register for UI visibility
	server.RegisterProxy(ui.ProxyInfo{
		LocalPath:   localPath,
		Type:        "single",
		CanRead:     cfg.canRead(),
		CanWrite:    cfg.canWrite(),
		CanDelete:   cfg.canDelete(),
		Description: cfg.Description,
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
		CheckOrigin:  server.Stream.CheckOrigin,
		Subprotocols: []string{"bearer"},
	}

	// For list routes, we need to handle both the base path and specific items
	basePath := strings.TrimSuffix(localPath, "/*")

	// Handler for the base path (list)
	listHandler := func(w http.ResponseWriter, r *http.Request) {
		if !cfg.enforceCapability(w, r) {
			return
		}
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

		handleHTTPProxy(server, w, r, address, remotePath, cfg.Subscribe)
	}

	// Handler for specific items
	itemHandler := func(w http.ResponseWriter, r *http.Request) {
		if !cfg.enforceCapability(w, r) {
			return
		}
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

		handleHTTPProxy(server, w, r, address, remotePath, cfg.Subscribe)
	}

	// Convert glob pattern to mux pattern
	muxPattern := GlobToMuxPattern(localPath)

	// Register routes - specific path first, then base. Mirror both onto
	// the route oracle so the data wildcard defers to them regardless of
	// registration order. Auth gating is handled by middleware registered
	// via server.Router.Use(...); gorilla/mux's Match applies the chain
	// before dispatch, so proxy handlers run after middleware like
	// built-in REST handlers. RouterMutate serializes both mutations
	// with the syncRouter wrapper's Match.
	server.RouterMutate(func() {
		server.Router.HandleFunc("/"+muxPattern, itemHandler)
		server.Router.HandleFunc("/"+basePath, listHandler)
	})
	server.RegisterOracleRoute("/"+muxPattern, nil)
	server.RegisterOracleRoute("/"+basePath, nil)

	// Register for UI visibility
	server.RegisterProxy(ui.ProxyInfo{
		LocalPath:   localPath,
		Type:        "list",
		CanRead:     cfg.canRead(),
		CanWrite:    cfg.canWrite(),
		CanDelete:   cfg.canDelete(),
		Description: cfg.Description,
	})

	// Register cleanup function to be called when server closes
	server.RegisterProxyCleanup(pm.CloseAll)

	return nil
}

func handleWebSocketProxy(w http.ResponseWriter, r *http.Request, pm *proxyManager, address, remotePath string, subCfg *client.SubscribeConfig, upgrader websocket.Upgrader, isList bool) {
	// Get client's Sec-Websocket-Protocol header - used for auth and needs to be forwarded to remote
	clientProtocol := r.Header.Get("Sec-Websocket-Protocol")

	// Build response headers - echo back subprotocol if client requested one
	// This is required for browser WebSocket connections that use Sec-Websocket-Protocol for auth
	var responseHeader http.Header
	if clientProtocol != "" {
		responseHeader = http.Header{}
		responseHeader.Set("Sec-Websocket-Protocol", clientProtocol)
	}

	// Upgrade local connection
	localConn, err := upgrader.Upgrade(w, r, responseHeader)
	if err != nil {
		return
	}

	// Retry loop: if getOrCreateState returned a state that flipped to
	// dying between the registry lookup and the addSubscriber call,
	// fetch again. The dying state will have been replaced by then
	// (either by our retry triggering replacement, or by the teardown
	// completing). The retry is bounded — each iteration either
	// attaches or hands back to the manager for a fresh state — so
	// it cannot livelock.
	var state *proxyState
	var msgChan chan wsMessage
	for {
		state = pm.getOrCreateState(address, remotePath, subCfg, isList, clientProtocol)
		msgChan = state.addSubscriber(localConn)
		if msgChan != nil {
			break
		}
	}

	// Forward messages to local subscriber
	go func() {
		defer func() {
			if state.removeSubscriber(localConn) {
				state.stopRemoteSubscription()
				if hook := teardownPauseHook.Load(); hook != nil {
					(*hook)()
				}
				pm.removeState(address, remotePath, state)
				if hook := afterTeardownHook.Load(); hook != nil {
					(*hook)()
				}
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

// proxyWrite streams a POST/PATCH body to upstream under the server's
// MaxRequestBodyBytes cap. The body is passed directly to the upstream
// request instead of buffered into memory, so a runaway client cannot
// force the proxy to allocate arbitrary bytes. If the cap trips during
// upstream transport, the client gets 413 instead of a generic 502.
//
// Wire framing is preserved from the client: req.ContentLength is forwarded
// so upstreams that reject chunked POST/PATCH (older non-Go servers, some
// content-inspection middleware) continue to receive Content-Length when
// the client sent one. MaxBytesReader is stateful and cannot be replayed,
// so req.GetBody is set to a sentinel error rather than left nil — that
// turns a 307/308 redirect into an unambiguous failure instead of the
// stdlib's "http: cannot redirect with body" message.
func proxyWrite(server *ooo.Server, w http.ResponseWriter, r *http.Request, method, targetURL string, header http.Header, httpClient *http.Client) {
	// Honest clients that declare an oversize Content-Length can be
	// rejected before we open the upstream TCP connection. A dishonest
	// client that lies about its length still trips the cap mid-stream
	// via the LimitBody wrap below, so this guard is an optimization,
	// not the correctness boundary.
	if server.MaxRequestBodyBytes > 0 && r.ContentLength > server.MaxRequestBodyBytes {
		http.Error(w, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
		return
	}
	body, probe := server.LimitBody(w, r)

	req, err := http.NewRequestWithContext(r.Context(), method, targetURL, body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.ContentLength = r.ContentLength
	req.GetBody = func() (io.ReadCloser, error) {
		return nil, errors.New("proxy: request body not replayable after streaming")
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range header {
		req.Header[k] = v
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		// MaxBytesReader trips during upstream transport. Distinguish
		// "client sent too many bytes" (413) from "client hung up mid
		// body" (400) and from generic transport failures (502).
		// The probe holds the underlying Read error when transport
		// surfaces a less-specific one.
		if ooo.IsRequestBodyTooLargeErr(err) || (probe != nil && ooo.IsRequestBodyTooLargeErr(probe.Last())) {
			http.Error(w, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
			return
		}
		// probe.Last() captures io.EOF on a clean read of the full body,
		// so only treat non-EOF read errors as a client-side fault.
		// ErrUnexpectedEOF and friends fall into this branch.
		if probe != nil && probe.Last() != nil && !errors.Is(probe.Last(), io.EOF) {
			http.Error(w, probe.Last().Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func handleHTTPProxy(server *ooo.Server, w http.ResponseWriter, r *http.Request, address, remotePath string, subCfg *client.SubscribeConfig) {
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
		proxyWrite(server, w, r, http.MethodPost, targetURL, header, httpClient)

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
		proxyWrite(server, w, r, http.MethodPatch, targetURL, header, httpClient)

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
		CheckOrigin:  server.Stream.CheckOrigin,
		Subprotocols: []string{"bearer"},
	}

	handler := func(w http.ResponseWriter, r *http.Request) {
		if !cfg.enforceCapability(w, r) {
			return
		}
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

		handleHTTPProxy(server, w, r, address, remotePath, cfg.Subscribe)
	}

	// Auth gating is handled by middleware registered via
	// server.Router.Use(...); gorilla/mux's Match applies the chain
	// before dispatch, so this proxy handler runs after middleware
	// like built-in REST handlers.
	server.RouterMutate(func() {
		server.Router.HandleFunc("/"+localPath, handler)
	})
	server.RegisterOracleRoute("/"+localPath, nil)

	// Register for UI visibility
	server.RegisterProxy(ui.ProxyInfo{
		LocalPath:   localPath,
		Type:        "vars",
		CanRead:     cfg.canRead(),
		CanWrite:    cfg.canWrite(),
		CanDelete:   cfg.canDelete(),
		Description: cfg.Description,
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
	// Description for UI display
	Description string
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
		Description:  cfg.Description,
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
	// Description for UI display
	Description string
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
		Description:  cfg.Description,
	}

	return RouteList(server, cfg.LocalKey, proxyCfg)
}
