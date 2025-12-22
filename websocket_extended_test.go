package ooo

import (
	"errors"
	"net/url"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/benitogf/ooo/meta"
	"github.com/goccy/go-json"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestWebSocketSubscriptionEvents(t *testing.T) {
	app := Server{}
	app.Silence = true

	subscribed := ""
	unsubscribed := ""
	var mu sync.Mutex

	app.OnSubscribe = func(key string) error {
		mu.Lock()
		subscribed = key
		mu.Unlock()
		return nil
	}

	app.OnUnsubscribe = func(key string) {
		mu.Lock()
		unsubscribed = key
		mu.Unlock()
	}

	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	// Connect to websocket
	u := url.URL{Scheme: "ws", Host: app.Address, Path: "/testkey"}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)

	// Give time for subscription event
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	require.Equal(t, "testkey", subscribed)
	mu.Unlock()

	// Close connection
	c.Close()

	// Give time for unsubscription event
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	require.Equal(t, "testkey", unsubscribed)
	mu.Unlock()
}

func TestWebSocketSubscriptionDenied(t *testing.T) {
	app := Server{}
	app.Silence = true

	app.OnSubscribe = func(key string) error {
		if key == "denied" {
			return errors.New("subscription denied")
		}
		return nil
	}

	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	// Try to connect to denied key
	u := url.URL{Scheme: "ws", Host: app.Address, Path: "/denied"}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.Error(t, err)
	require.Nil(t, c)
}

func TestWebSocketWithVersion(t *testing.T) {
	var wg sync.WaitGroup
	app := Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	// First connect to get initial version and wait for broadcast to update cache
	app.OpenFilter("versiontest")
	u := url.URL{Scheme: "ws", Host: app.Address, Path: "/versiontest"}
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

	_, err = app.Storage.Set("versiontest", json.RawMessage(`{"version":1}`))
	require.NoError(t, err)
	wg.Wait()
	c2.Close()

	// Get current version (after broadcast has updated it)
	entry, err := app.fetch("versiontest")
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
	app := Server{}
	app.Silence = true
	app.Static = true

	// Don't add any filters - route should be denied in static mode
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	// Try to connect to unfiltered route in static mode
	u := url.URL{Scheme: "ws", Host: app.Address, Path: "/filtered"}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.Error(t, err)
	require.Nil(t, c)
}

func TestWebSocketConcurrentConnections(t *testing.T) {
	app := Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	// Set initial data
	_, err := app.Storage.Set("concurrent", json.RawMessage(`{"test":true}`))
	require.NoError(t, err)

	var wg sync.WaitGroup
	numConnections := 5

	for i := 0; i < numConnections; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			u := url.URL{Scheme: "ws", Host: app.Address, Path: "/concurrent"}
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
	app := Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	// Set initial data
	_, err := app.Storage.Set("broadcast", json.RawMessage(`{"value":1}`))
	require.NoError(t, err)

	// Connect websocket
	u := url.URL{Scheme: "ws", Host: app.Address, Path: "/broadcast"}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer c.Close()

	// Read initial message
	_, message, err := c.ReadMessage()
	require.NoError(t, err)
	require.Contains(t, string(message), `"value":1`)

	// Update data (should trigger broadcast)
	_, err = app.Storage.Set("broadcast", json.RawMessage(`{"value":2}`))
	require.NoError(t, err)

	// Read broadcast message
	c.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, message, err = c.ReadMessage()
	require.NoError(t, err)
	require.Contains(t, string(message), `"value":2`)
}

func TestWebSocketGlobSubscription(t *testing.T) {
	app := Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	// Set data matching pattern
	_, err := app.Storage.Set("items/1", json.RawMessage(`{"id":1}`))
	require.NoError(t, err)
	_, err = app.Storage.Set("items/2", json.RawMessage(`{"id":2}`))
	require.NoError(t, err)

	// Connect to glob pattern
	u := url.URL{Scheme: "ws", Host: app.Address, Path: "/items/*"}
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
	app := Server{}
	app.Silence = true

	// Add read filter that modifies data (non-glob path uses ReadObjectFilter)
	app.ReadObjectFilter("filtered", func(key string, obj meta.Object) (meta.Object, error) {
		return meta.Object{Data: json.RawMessage(`{"filtered":true}`), Created: 1, Index: "1"}, nil
	})

	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	// Set original data
	_, err := app.Storage.Set("filtered", json.RawMessage(`{"original":true}`))
	require.NoError(t, err)

	// Connect websocket
	u := url.URL{Scheme: "ws", Host: app.Address, Path: "/filtered"}
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
