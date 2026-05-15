package ooo

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/benitogf/go-json"
	"github.com/benitogf/ooo/client"
	"github.com/benitogf/ooo/meta"
	"github.com/benitogf/ooo/stream"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// waitForPoolConnections waits until the pool with the given key has at least n
// connections, so tests can synchronize ordering of dial → conn registration.
func waitForPoolConnections(t *testing.T, sm *stream.Stream, key string, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, p := range sm.GetState() {
			if p.Key == key && p.Connections >= n {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("pool %q did not reach %d connections in time", key, n)
}

// TestStreamBroadcastSlowSubscriberDoesNotStallPool asserts that a slow
// WebSocket peer (whose TCP receive buffer is full) does not delay broadcast
// delivery to other peers on the same pool. Pre-fix Stream.Broadcast iterates
// connections sequentially under pool.mutex, so a single slow peer blocks the
// fast peer for up to WriteTimeout per broadcast and stalls the watcher worker
// that fans out events.
func TestStreamBroadcastSlowSubscriberDoesNotStallPool(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("test relies on Linux TCP buffer behavior")
	}

	server := Server{}
	server.Silence = true
	// Long enough that a stall is unmistakable, short enough to keep the
	// failure mode bounded.
	server.Stream.WriteTimeout = 5 * time.Second
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	u := url.URL{Scheme: "ws", Host: server.Address, Path: "/items/*"}

	// Slow subscriber dialed first so it lands at pool.connections[0].
	// Shrink its TCP receive buffer so the server's writes back-pressure
	// after a small payload, then never read from it.
	slow, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer slow.Close()
	tc, ok := slow.UnderlyingConn().(*net.TCPConn)
	require.True(t, ok, "underlying conn must be *net.TCPConn")
	require.NoError(t, tc.SetReadBuffer(2048))

	waitForPoolConnections(t, &server.Stream, "items/*", 1)

	// Fast subscriber dialed second.
	fast, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer fast.Close()
	waitForPoolConnections(t, &server.Stream, "items/*", 2)

	// Drain the initial empty-list snapshot from fast.
	fast.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = fast.ReadMessage()
	require.NoError(t, err)

	// 1 MiB payload definitely exceeds the default kernel send buffer, so the
	// server's write to slow blocks once slow's tiny recv buffer fills.
	bigPayload := strings.Repeat("x", 1024*1024)

	start := time.Now()
	_, err = server.Storage.Set("items/probe", json.RawMessage(`{"v":"`+bigPayload+`"}`))
	require.NoError(t, err)

	fast.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, err = fast.ReadMessage()
	elapsed := time.Since(start)
	require.NoError(t, err)

	// Pre-fix: slow at index 0 blocks ~WriteTimeout, fast waits behind it.
	// Post-fix: each conn has its own writer, fast gets the message in ms.
	require.Less(t, elapsed, 1*time.Second,
		"fast subscriber waited %v — slow peer is starving the pool", elapsed)
}

// TestStreamReadReapsHalfClosedConn asserts that a websocket client which
// stops responding to server pings is reaped within bounded time. Pre-fix
// Stream.Read had no read deadline and no server-side pings, so a quiet pool
// with a half-closed conn would never notice the peer was gone.
func TestStreamReadReapsHalfClosedConn(t *testing.T) {
	t.Parallel()
	server := Server{}
	server.Silence = true
	server.Stream.PingInterval = 50 * time.Millisecond
	server.Stream.PongTimeout = 200 * time.Millisecond

	var unsubscribed sync.WaitGroup
	unsubscribed.Add(1)
	var unsubOnce sync.Once
	server.OnUnsubscribe = func(key string) {
		if key == "deadconn" {
			unsubOnce.Do(unsubscribed.Done)
		}
	}

	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	u := url.URL{Scheme: "ws", Host: server.Address, Path: "/deadconn"}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer c.Close()

	// Override the client's ping handler so it never replies with a pong.
	// Combined with not calling ReadMessage, the server's pong-resetting
	// deadline expires and Stream.Read should error and Close the conn.
	c.SetPingHandler(func(appData string) error { return nil })

	done := make(chan struct{})
	go func() {
		unsubscribed.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not reap dead websocket conn within bounded time")
	}
}

// TestWsTime tests clock websocket connections
// Note: Uses raw websocket because clock endpoint sends raw timestamp strings, not JSON objects
func TestWsTime(t *testing.T) {
	t.Parallel()
	var wg sync.WaitGroup
	server := Server{}
	server.Silence = true
	server.Tick = 1 * time.Millisecond
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	u := url.URL{Scheme: "ws", Host: server.Address, Path: "/"}
	c1, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer c1.Close()
	c2, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer c2.Close()

	// Wait for 2 messages on c1 and 1 message on c2
	c1Count := 0
	wg.Add(1) // For c1 goroutine completion
	go func() {
		defer wg.Done()
		for {
			_, message, err := c1.ReadMessage()
			if err != nil {
				server.Console.Err("read c1", err)
				break
			}
			server.Console.Log("time c1", string(message))
			c1Count++
			if c1Count >= 2 {
				break
			}
		}
	}()

	// Read one message from c2
	_, message, err := c2.ReadMessage()
	require.NoError(t, err)
	server.Console.Log("time c2", string(message))

	wg.Wait()
}

// TestWebSocketSubscriptionEvents tests OnSubscribe/OnUnsubscribe callbacks
// Note: Uses raw websocket to test subscription lifecycle events
func TestWebSocketSubscriptionEvents(t *testing.T) {
	server := Server{}
	server.Silence = true

	var subscribedWg sync.WaitGroup
	subscribedWg.Add(1)
	var unsubscribedWg sync.WaitGroup
	unsubscribedWg.Add(1)

	subscribed := ""
	unsubscribed := ""

	server.OnSubscribe = func(key string) error {
		subscribed = key
		subscribedWg.Done()
		return nil
	}

	server.OnUnsubscribe = func(key string) {
		unsubscribed = key
		unsubscribedWg.Done()
	}

	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Connect to websocket
	u := url.URL{Scheme: "ws", Host: server.Address, Path: "/testkey"}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)

	subscribedWg.Wait()
	require.Equal(t, "testkey", subscribed)

	// Close connection - this triggers OnUnsubscribe
	c.Close()

	unsubscribedWg.Wait()
	require.Equal(t, "testkey", unsubscribed)
}

// TestFetchUpgradeFailDoesNotLeakPool asserts that when a subscription
// request reaches the ws handler — populating the stream pool's cache
// via fetch — and the subsequent WebSocket upgrade fails, the pool is
// pruned so an orphan entry cannot linger in sm.pools / poolIndex.
// Pre-fix InitCache eagerly registered the pool and a failed upgrade
// never invoked Close, so the entry leaked forever for any
// non-persistent (filter-less) subscription path.
func TestFetchUpgradeFailDoesNotLeakPool(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true
	server.AllowedOrigins = []string{"http://allowed.example.com"}
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	require.Zero(t, server.Stream.PoolCount(), "fixture: no filters registered, no pools expected")

	u := url.URL{Scheme: "ws", Host: server.Address, Path: "/ephemeral"}
	hdr := http.Header{}
	hdr.Set("Origin", "http://evil.example.com")
	c, _, err := websocket.DefaultDialer.Dial(u.String(), hdr)
	require.Error(t, err, "cross-origin upgrade must be rejected")
	require.Nil(t, c)

	require.Zero(t, server.Stream.PoolCount(),
		"the orphan pool from a failed upgrade must be pruned by PruneIfEmpty")
}

// TestWebSocketCheckOriginRejectsCrossOrigin asserts the WebSocket upgrader
// rejects connections whose Origin is not in the server's AllowedOrigins.
// Pre-fix the upgrader's CheckOrigin was a no-op (returned true whenever the
// Upgrade header was present, which gorilla always sets before the hook), so
// a browser at evil.example.com could open a session on behalf of a
// logged-in user (CSWSH). The upgrader now honors AllowedOrigins, same-origin
// requests, and missing Origin (non-browser clients).
func TestWebSocketCheckOriginRejectsCrossOrigin(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true
	server.AllowedOrigins = []string{"http://allowed.example.com"}
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	u := url.URL{Scheme: "ws", Host: server.Address, Path: "/test"}

	hdr := http.Header{}
	hdr.Set("Origin", "http://evil.example.com")
	c, _, err := websocket.DefaultDialer.Dial(u.String(), hdr)
	require.Error(t, err)
	require.Nil(t, c)
}

// TestWebSocketCheckOriginAcceptsConfigured asserts an origin listed in
// AllowedOrigins is accepted.
func TestWebSocketCheckOriginAcceptsConfigured(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true
	server.AllowedOrigins = []string{"http://allowed.example.com"}
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	u := url.URL{Scheme: "ws", Host: server.Address, Path: "/test"}

	hdr := http.Header{}
	hdr.Set("Origin", "http://allowed.example.com")
	c, _, err := websocket.DefaultDialer.Dial(u.String(), hdr)
	require.NoError(t, err)
	defer c.Close()
}

// TestWebSocketCheckOriginAcceptsSameOrigin asserts a request whose Origin
// matches the request Host is accepted regardless of AllowedOrigins. This is
// the legitimate browser case where the page is served from the same host.
func TestWebSocketCheckOriginAcceptsSameOrigin(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true
	server.AllowedOrigins = []string{"http://allowed.example.com"}
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	u := url.URL{Scheme: "ws", Host: server.Address, Path: "/test"}

	hdr := http.Header{}
	hdr.Set("Origin", "http://"+server.Address)
	c, _, err := websocket.DefaultDialer.Dial(u.String(), hdr)
	require.NoError(t, err)
	defer c.Close()
}

// TestWebSocketCheckOriginAcceptsNoOrigin asserts a request with no Origin
// header is accepted. Non-browser clients (Go ws dialers, CLIs) do not always
// send Origin and breaking them would regress every existing programmatic
// subscriber.
func TestWebSocketCheckOriginAcceptsNoOrigin(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true
	server.AllowedOrigins = []string{"http://allowed.example.com"}
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	u := url.URL{Scheme: "ws", Host: server.Address, Path: "/test"}

	// gorilla's default dialer omits Origin unless we set it.
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer c.Close()
}

// TestWebSocketCheckOriginWildcardAcceptsAny asserts AllowedOrigins=["*"]
// (the default after defaultCORS) preserves the historical wide-open behavior
// so existing deployments are not broken by the upgrade.
func TestWebSocketCheckOriginWildcardAcceptsAny(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true
	// AllowedOrigins unset → defaultCORS sets to ["*"]
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	u := url.URL{Scheme: "ws", Host: server.Address, Path: "/test"}

	hdr := http.Header{}
	hdr.Set("Origin", "http://any.example.com")
	c, _, err := websocket.DefaultDialer.Dial(u.String(), hdr)
	require.NoError(t, err)
	defer c.Close()
}

func TestWebSocketSubscriptionDenied(t *testing.T) {
	server := Server{}
	server.Silence = true

	server.OnSubscribe = func(key string) error {
		if key == "denied" {
			return errors.New("subscription denied")
		}
		return nil
	}

	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Try to connect to denied key
	u := url.URL{Scheme: "ws", Host: server.Address, Path: "/denied"}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.Error(t, err)
	require.Nil(t, c)
}

// TestWebSocketWithVersion tests version query parameter behavior
// Note: Uses raw websocket to test ?v= query parameter which client package doesn't expose
func TestWebSocketWithVersion(t *testing.T) {
	var wg sync.WaitGroup
	server := Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// First connect to get initial version and wait for broadcast to update cache
	server.OpenFilter("versiontest")
	u := url.URL{Scheme: "ws", Host: server.Address, Path: "/versiontest"}
	c1, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)

	wg.Add(1)
	go func() {
		_, _, _ = c1.ReadMessage() // Wait for initial snapshot
		wg.Done()
	}()
	wg.Wait()
	c1.Close()

	// Set data - this will trigger a broadcast
	wg.Add(1)
	c2, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)

	go func() {
		_, _, _ = c2.ReadMessage() // Initial snapshot
		wg.Done()
	}()
	wg.Wait()

	wg.Add(1)
	go func() {
		_, _, _ = c2.ReadMessage() // Wait for broadcast from Set
		wg.Done()
	}()

	_, err = server.Storage.Set("versiontest", json.RawMessage(`{"version":1}`))
	require.NoError(t, err)
	wg.Wait()
	c2.Close()

	// Get current version (after broadcast has updated it)
	entry, err := server.fetch("versiontest")
	require.NoError(t, err)

	// Connect with matching version (should not receive initial data)
	q := u.Query()
	q.Set("v", strconv.FormatInt(entry.Version, 16))
	u.RawQuery = q.Encode()

	c3, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer c3.Close()

	// Set read deadline to avoid blocking
	c3.SetReadDeadline(time.Now().Add(10 * time.Millisecond))

	// Should not receive initial message due to version match
	_, _, err = c3.ReadMessage()
	require.Error(t, err) // Should timeout
}

func TestWebSocketFilteredRoute(t *testing.T) {
	server := Server{}
	server.Silence = true
	server.Static = true

	// Don't add any filters - route should be denied in static mode
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Try to connect to unfiltered route in static mode
	u := url.URL{Scheme: "ws", Host: server.Address, Path: "/filtered"}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.Error(t, err)
	require.Nil(t, c)
}

// TestWebSocketConcurrentConnections tests multiple simultaneous connections
// Note: Uses raw websocket to test concurrent connection handling
func TestWebSocketConcurrentConnections(t *testing.T) {
	server := Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Set initial data
	_, err := server.Storage.Set("concurrent", json.RawMessage(`{"test":true}`))
	require.NoError(t, err)

	var wg sync.WaitGroup
	numConnections := 5

	for i := range numConnections {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			u := url.URL{Scheme: "ws", Host: server.Address, Path: "/concurrent"}
			c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
			require.NoError(t, err)
			defer c.Close()

			// Read initial message
			_, message, err := c.ReadMessage()
			require.NoError(t, err)
			require.Contains(t, string(message), "test")
		}(i)
	}

	wg.Wait()
}

func TestWebSocketBroadcastUpdate(t *testing.T) {
	server := Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Set initial data
	_, err := server.Storage.Set("broadcast", json.RawMessage(`{"value":1}`))
	require.NoError(t, err)

	// Connect websocket
	u := url.URL{Scheme: "ws", Host: server.Address, Path: "/broadcast"}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer c.Close()

	// Read initial message
	_, message, err := c.ReadMessage()
	require.NoError(t, err)
	require.Contains(t, string(message), `"value":1`)

	// Update data (should trigger broadcast)
	_, err = server.Storage.Set("broadcast", json.RawMessage(`{"value":2}`))
	require.NoError(t, err)

	// Read broadcast message
	c.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, message, err = c.ReadMessage()
	require.NoError(t, err)
	require.Contains(t, string(message), `"value":2`)
}

func TestWebSocketGlobSubscription(t *testing.T) {
	server := Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Set data matching pattern
	_, err := server.Storage.Set("items/1", json.RawMessage(`{"id":1}`))
	require.NoError(t, err)
	_, err = server.Storage.Set("items/2", json.RawMessage(`{"id":2}`))
	require.NoError(t, err)

	// Connect to glob pattern
	u := url.URL{Scheme: "ws", Host: server.Address, Path: "/items/*"}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer c.Close()

	// Read initial message (should contain both items)
	c.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, message, err := c.ReadMessage()
	require.NoError(t, err)

	// Should contain array of items
	require.Contains(t, string(message), `"id":1`)
	require.Contains(t, string(message), `"id":2`)
}

func TestWebSocketReadFilter(t *testing.T) {
	server := Server{}
	server.Silence = true

	// Add read filter that modifies data (non-glob path uses ReadObjectFilter)
	server.ReadObjectFilter("filtered", func(key string, obj meta.Object) (meta.Object, error) {
		return meta.Object{Data: json.RawMessage(`{"filtered":true}`), Created: 1, Index: "1"}, nil
	})

	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Set original data
	_, err := server.Storage.Set("filtered", json.RawMessage(`{"original":true}`))
	require.NoError(t, err)

	// Connect websocket
	u := url.URL{Scheme: "ws", Host: server.Address, Path: "/filtered"}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer c.Close()

	// Read message (should be filtered)
	c.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, message, err := c.ReadMessage()
	require.NoError(t, err)
	require.Contains(t, string(message), "filtered")
	require.NotContains(t, string(message), "original")
}

// TestWebSocketGlobKey tests glob pattern in websocket path
// Note: Uses raw websocket to test path pattern handling
func TestWebSocketGlobKey(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	u := url.URL{Scheme: "ws", Host: server.Address, Path: "/ws/test/*"}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	server.Console.Err(err)
	require.NotNil(t, c)
	require.NoError(t, err)
	defer c.Close()
}

func TestWebSocketInvalidKey(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	u := url.URL{Scheme: "ws", Host: server.Address, Path: "/sa//test"}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.Nil(t, c)
	server.Console.Err(err)
	require.Error(t, err)
	u = url.URL{Scheme: "ws", Host: server.Address, Path: "/sa/test//1"}
	c, _, err = websocket.DefaultDialer.Dial(u.String(), nil)
	require.Nil(t, c)
	server.Console.Err(err)
	require.Error(t, err)
	u = url.URL{Scheme: "ws", Host: server.Address, Path: "/sa/test/1/"}
	c, _, err = websocket.DefaultDialer.Dial(u.String(), nil)
	require.Nil(t, c)
	server.Console.Err(err)
	require.Error(t, err)
}

func TestWebSocketReadListFilterAllowsIndividualSubscribe(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true
	server.Static = true

	// ReadListFilter for glob path should allow subscribing to both list and individual items
	// This is the fix for: logs/* should allow subscribing to logs/123
	server.ReadListFilter("logs/*", func(key string, objs []meta.Object) ([]meta.Object, error) {
		return objs, nil
	})

	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	type LogEntry struct {
		Level   string `json:"level"`
		Message string `json:"message"`
	}

	// Create an item directly in storage
	index, err := server.Storage.Set("logs/testitem", json.RawMessage(`{"level":"info","message":"test log"}`))
	require.NoError(t, err)
	require.NotEmpty(t, index)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := client.SubscribeConfig{
		Ctx:     ctx,
		Server:  client.Server{Protocol: "ws", Host: server.Address},
		Silence: true,
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Subscribe to the list - should work
	var listReceived bool
	go client.SubscribeList(cfg, "logs/*", client.SubscribeListEvents[LogEntry]{
		OnMessage: func(items []client.Meta[LogEntry]) {
			if len(items) > 0 && items[0].Data.Message == "test log" {
				listReceived = true
				wg.Done()
			}
		},
		OnError: func(err error) {
			if !listReceived {
				t.Errorf("list subscription error: %v", err)
				wg.Done()
			}
		},
	})

	// Subscribe to the individual item - this was the bug, should now work
	var itemReceived bool
	go client.Subscribe(cfg, "logs/testitem", client.SubscribeEvents[LogEntry]{
		OnMessage: func(item client.Meta[LogEntry]) {
			if item.Data.Message == "test log" {
				itemReceived = true
				wg.Done()
			}
		},
		OnError: func(err error) {
			if !itemReceived {
				t.Errorf("individual subscription error (ReadListFilter must allow individual subscriptions): %v", err)
				wg.Done()
			}
		},
	})

	wg.Wait()

	require.True(t, listReceived, "list subscription should receive the item")
	require.True(t, itemReceived, "individual subscription should receive the item - ReadListFilter must also allow individual subscriptions")
}
