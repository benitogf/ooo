package ooo

import (
	"errors"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/benitogf/ooo/meta"
	"github.com/goccy/go-json"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

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
