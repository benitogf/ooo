package proxy_test

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/benitogf/go-json"
	"github.com/benitogf/ooo"
	"github.com/benitogf/ooo/client"
	"github.com/benitogf/ooo/proxy"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

type Settings struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

type Thing struct {
	ID   string `json:"id"`
	Data string `json:"data"`
}

func TestGlobToMuxPattern(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple glob", "states/*", "states/{path1:.*}"},
		{"no glob", "settings", "settings"},
		{"middle glob", "devices/*/things", "devices/{path1}/things"},
		{"multiple globs", "devices/*/things/*", "devices/{path1}/things/{path2:.*}"},
		{"nested path no glob", "api/v1/users", "api/v1/users"},
		{
			"ten wildcards",
			"a/*/b/*/c/*/d/*/e/*/f/*/g/*/h/*/i/*/j/*",
			"a/{path1}/b/{path2}/c/{path3}/d/{path4}/e/{path5}/f/{path6}/g/{path7}/h/{path8}/i/{path9}/j/{path10:.*}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := proxy.GlobToMuxPattern(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

// TestProxyCheckOriginRejectsCrossOrigin asserts the proxy ws upgrader
// honors AllowedOrigins. Pre-fix the proxy upgraders returned true
// unconditionally, so any browser origin could ride a logged-in user's
// session through the proxy and into the upstream.
func TestProxyCheckOriginRejectsCrossOrigin(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	remote := &ooo.Server{}
	remote.Silence = true
	remote.Start("localhost:0")
	defer remote.Close(os.Interrupt)

	proxyServer := &ooo.Server{}
	proxyServer.Silence = true
	proxyServer.AllowedOrigins = []string{"http://allowed.example.com"}

	err := proxy.Route(proxyServer, "settings/{deviceID}", proxy.Config{
		Resolve: func(_ string) (string, string, error) {
			return remote.Address, "settings", nil
		},
	})
	require.NoError(t, err)
	proxyServer.Start("localhost:0")
	defer proxyServer.Close(os.Interrupt)

	u := url.URL{Scheme: "ws", Host: proxyServer.Address, Path: "/settings/device1"}
	hdr := http.Header{}
	hdr.Set("Origin", "http://evil.example.com")
	c, _, err := websocket.DefaultDialer.Dial(u.String(), hdr)
	require.Error(t, err)
	require.Nil(t, c)
}

func TestRouteWithGlobPattern(t *testing.T) {
	remote := &ooo.Server{}
	remote.Silence = true
	remote.Static = true
	remote.OpenFilter("state")
	remote.Start("localhost:0")
	defer remote.Close(os.Interrupt)

	proxyServer := &ooo.Server{}
	proxyServer.Silence = true

	err := proxy.Route(proxyServer, "states/*", proxy.Config{
		Resolve: func(localPath string) (string, string, error) {
			// localPath is "states/device123", extract device ID
			deviceID := strings.TrimPrefix(localPath, "states/")
			_ = deviceID // In real use, look up device address
			return remote.Address, "state", nil
		},
	})
	require.NoError(t, err)

	proxyServer.Start("localhost:0")
	defer proxyServer.Close(os.Interrupt)

	// Set data on remote
	data, _ := json.Marshal(Settings{Name: "test", Value: 42})
	_, err = remote.Storage.Set("state", data)
	require.NoError(t, err)

	// GET through proxy with glob pattern
	resp, err := http.Get("http://" + proxyServer.Address + "/states/device123")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		Data Settings `json:"data"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	require.Equal(t, "test", result.Data.Name)
}

func TestConfig_Validate(t *testing.T) {
	cfg := &proxy.Config{}
	err := cfg.Validate()
	require.ErrorIs(t, err, proxy.ErrResolverRequired)

	cfg = &proxy.Config{
		Resolve: func(_ string) (string, string, error) {
			return "localhost:8080", "settings", nil
		},
	}
	err = cfg.Validate()
	require.NoError(t, err)
}

func TestRoute_EmptyLocalPath(t *testing.T) {
	server := &ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	err := proxy.Route(server, "", proxy.Config{
		Resolve: func(_ string) (string, string, error) {
			return "localhost:8080", "settings", nil
		},
	})
	require.ErrorIs(t, err, proxy.ErrLocalPathEmpty)
}

func TestProxy(t *testing.T) {
	var wsWg sync.WaitGroup
	var proxySettings []client.Meta[Settings]
	var mu sync.Mutex

	remote := &ooo.Server{}
	remote.Silence = true
	remote.Static = true
	remote.OpenFilter("settings")
	remote.OpenFilter("things/*")
	remote.Start("localhost:0")
	defer remote.Close(os.Interrupt)

	proxyServer := &ooo.Server{}
	proxyServer.Silence = true

	err := proxy.Route(proxyServer, "settings/{deviceID}", proxy.Config{
		Resolve: func(_ string) (string, string, error) {
			return remote.Address, "settings", nil
		},
	})
	require.NoError(t, err)

	err = proxy.RouteList(proxyServer, "devices/{deviceID}/things/{itemID}", proxy.Config{
		Resolve: func(localPath string) (string, string, error) {
			parts := strings.Split(localPath, "/")
			if len(parts) >= 4 {
				return remote.Address, "things/" + parts[3], nil
			}
			return remote.Address, "things", nil
		},
	})
	require.NoError(t, err)

	proxyServer.Start("localhost:0")
	defer proxyServer.Close(os.Interrupt)

	ctx := t.Context()

	// Subscribe to settings through proxy - expect 1 initial message (empty)
	wsWg.Add(1)
	go client.Subscribe(client.SubscribeConfig{
		Ctx:     ctx,
		Server:  client.Server{Protocol: "ws", Host: proxyServer.Address},
		Silence: true,
	}, "settings/device1", client.SubscribeEvents[Settings]{
		OnMessage: func(m client.Meta[Settings]) {
			mu.Lock()
			proxySettings = append(proxySettings, m)
			mu.Unlock()
			wsWg.Done()
		},
	})
	wsWg.Wait()

	// Test HTTP POST through proxy
	data, _ := json.Marshal(Settings{Name: "test", Value: 42})
	wsWg.Add(1)
	resp, err := http.Post("http://"+proxyServer.Address+"/settings/device1", "application/json", bytes.NewReader(data))
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	wsWg.Wait()

	// Verify data on remote
	obj, err := remote.Storage.Get("settings")
	require.NoError(t, err)
	var settings Settings
	err = json.Unmarshal(obj.Data, &settings)
	require.NoError(t, err)
	require.Equal(t, "test", settings.Name)
	require.Equal(t, 42, settings.Value)

	// Verify WebSocket received update
	mu.Lock()
	require.GreaterOrEqual(t, len(proxySettings), 2)
	require.Equal(t, "test", proxySettings[1].Data.Name)
	mu.Unlock()

	// Test HTTP GET through proxy
	resp, err = http.Get("http://" + proxyServer.Address + "/settings/device1")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result struct {
		Data Settings `json:"data"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	require.Equal(t, "test", result.Data.Name)
	require.Equal(t, 42, result.Data.Value)

	// Test HTTP POST update through proxy
	data, _ = json.Marshal(Settings{Name: "updated", Value: 100})
	wsWg.Add(1)
	resp, err = http.Post("http://"+proxyServer.Address+"/settings/device1", "application/json", bytes.NewReader(data))
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	wsWg.Wait()

	// Verify update
	mu.Lock()
	require.GreaterOrEqual(t, len(proxySettings), 3)
	require.Equal(t, "updated", proxySettings[2].Data.Name)
	require.Equal(t, 100, proxySettings[2].Data.Value)
	mu.Unlock()

	// Test HTTP DELETE through proxy
	wsWg.Add(1)
	req, err := http.NewRequest(http.MethodDelete, "http://"+proxyServer.Address+"/settings/device1", nil)
	require.NoError(t, err)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent)
	wsWg.Wait()

	// Verify data deleted on remote
	_, err = remote.Storage.Get("settings")
	require.Error(t, err)

	// Test RouteList - set data on remote
	data1, _ := json.Marshal(Thing{ID: "1", Data: "first"})
	_, err = remote.Storage.Set("things/item1", data1)
	require.NoError(t, err)

	// Get through RouteList proxy
	resp, err = http.Get("http://" + proxyServer.Address + "/devices/dev1/things/item1")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestProxyMultipleSubscribers(t *testing.T) {
	var wsWg sync.WaitGroup
	var received1, received2 []client.Meta[Settings]
	var mu sync.Mutex

	remote := &ooo.Server{}
	remote.Silence = true
	remote.Static = true
	remote.OpenFilter("settings")
	remote.Start("localhost:0")
	defer remote.Close(os.Interrupt)

	proxyServer := &ooo.Server{}
	proxyServer.Silence = true

	err := proxy.Route(proxyServer, "settings/{deviceID}", proxy.Config{
		Resolve: func(_ string) (string, string, error) {
			return remote.Address, "settings", nil
		},
	})
	require.NoError(t, err)

	proxyServer.Start("localhost:0")
	defer proxyServer.Close(os.Interrupt)

	ctx := t.Context()

	// Start both subscribers - expect 2 initial messages (empty)
	wsWg.Add(2)
	go client.Subscribe(client.SubscribeConfig{
		Ctx:     ctx,
		Server:  client.Server{Protocol: "ws", Host: proxyServer.Address},
		Silence: true,
	}, "settings/device1", client.SubscribeEvents[Settings]{
		OnMessage: func(m client.Meta[Settings]) {
			mu.Lock()
			received1 = append(received1, m)
			mu.Unlock()
			wsWg.Done()
		},
	})

	go client.Subscribe(client.SubscribeConfig{
		Ctx:     ctx,
		Server:  client.Server{Protocol: "ws", Host: proxyServer.Address},
		Silence: true,
	}, "settings/device1", client.SubscribeEvents[Settings]{
		OnMessage: func(m client.Meta[Settings]) {
			mu.Lock()
			received2 = append(received2, m)
			mu.Unlock()
			wsWg.Done()
		},
	})
	wsWg.Wait()

	// Set data - both should receive (2 messages)
	wsWg.Add(2)
	data, _ := json.Marshal(Settings{Name: "shared", Value: 1})
	_, err = remote.Storage.Set("settings", data)
	require.NoError(t, err)
	wsWg.Wait()

	mu.Lock()
	require.GreaterOrEqual(t, len(received1), 2, "subscriber 1 should receive messages")
	require.GreaterOrEqual(t, len(received2), 2, "subscriber 2 should receive messages")
	require.Equal(t, "shared", received1[1].Data.Name)
	require.Equal(t, "shared", received2[1].Data.Name)
	mu.Unlock()
}

func TestProxyResolverError(t *testing.T) {
	server := &ooo.Server{}
	server.Silence = true

	err := proxy.Route(server, "settings/{deviceID}", proxy.Config{
		Resolve: func(_ string) (string, string, error) {
			return "", "", proxy.ErrResolveFailed
		},
	})
	require.NoError(t, err)

	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	resp, err := http.Get("http://" + server.Address + "/settings/unknown")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestProxyLateSubscriberGetsSnapshot tests that a subscriber joining an already-active
// proxy connection receives a proper snapshot instead of a patch message.
func TestProxyLateSubscriberGetsSnapshot(t *testing.T) {
	var wsWg sync.WaitGroup
	var received1, received2 []client.Meta[Settings]
	var mu sync.Mutex

	remote := &ooo.Server{}
	remote.Silence = true
	remote.Static = true
	remote.OpenFilter("settings")
	remote.Start("localhost:0")
	defer remote.Close(os.Interrupt)

	proxyServer := &ooo.Server{}
	proxyServer.Silence = true

	err := proxy.Route(proxyServer, "settings/{deviceID}", proxy.Config{
		Resolve: func(_ string) (string, string, error) {
			return remote.Address, "settings", nil
		},
	})
	require.NoError(t, err)

	proxyServer.Start("localhost:0")
	defer proxyServer.Close(os.Interrupt)

	ctx := t.Context()

	// Set initial data on remote
	data, _ := json.Marshal(Settings{Name: "initial", Value: 1})
	_, err = remote.Storage.Set("settings", data)
	require.NoError(t, err)

	// First subscriber connects - should receive initial snapshot
	wsWg.Add(1)
	go client.Subscribe(client.SubscribeConfig{
		Ctx:     ctx,
		Server:  client.Server{Protocol: "ws", Host: proxyServer.Address},
		Silence: true,
	}, "settings/device1", client.SubscribeEvents[Settings]{
		OnMessage: func(m client.Meta[Settings]) {
			mu.Lock()
			received1 = append(received1, m)
			mu.Unlock()
			wsWg.Done()
		},
	})
	wsWg.Wait()

	// Verify first subscriber got the initial data
	mu.Lock()
	require.Len(t, received1, 1, "first subscriber should receive initial snapshot")
	require.Equal(t, "initial", received1[0].Data.Name)
	require.Equal(t, 1, received1[0].Data.Value)
	mu.Unlock()

	// Update data on remote - first subscriber receives the update
	wsWg.Add(1)
	data, _ = json.Marshal(Settings{Name: "updated", Value: 42})
	_, err = remote.Storage.Set("settings", data)
	require.NoError(t, err)
	wsWg.Wait()

	mu.Lock()
	require.Len(t, received1, 2, "first subscriber should receive update")
	require.Equal(t, "updated", received1[1].Data.Name)
	mu.Unlock()

	// Now a second subscriber joins AFTER the proxy connection is already active
	// This subscriber should receive the current state as a snapshot, not a patch
	wsWg.Add(1)
	go client.Subscribe(client.SubscribeConfig{
		Ctx:     ctx,
		Server:  client.Server{Protocol: "ws", Host: proxyServer.Address},
		Silence: true,
	}, "settings/device1", client.SubscribeEvents[Settings]{
		OnMessage: func(m client.Meta[Settings]) {
			mu.Lock()
			received2 = append(received2, m)
			mu.Unlock()
			wsWg.Done()
		},
	})
	wsWg.Wait()

	// Verify second subscriber got the current state as a snapshot
	mu.Lock()
	require.Len(t, received2, 1, "late subscriber should receive snapshot")
	require.Equal(t, "updated", received2[0].Data.Name, "late subscriber should get current state")
	require.Equal(t, 42, received2[0].Data.Value, "late subscriber should get current state")
	mu.Unlock()

	// Update data again - both subscribers should receive
	wsWg.Add(2)
	data, _ = json.Marshal(Settings{Name: "final", Value: 100})
	_, err = remote.Storage.Set("settings", data)
	require.NoError(t, err)
	wsWg.Wait()

	// Verify both received the update
	mu.Lock()
	require.Len(t, received1, 3, "first subscriber should receive all updates")
	require.Equal(t, "final", received1[2].Data.Name)
	require.Len(t, received2, 2, "late subscriber should receive updates after joining")
	require.Equal(t, "final", received2[1].Data.Name)
	mu.Unlock()
}

// TestProxyEmptyObjectSubscription tests that when subscribing to a non-existent object key,
// the proxy returns an empty meta object (created=0, updated=0) matching ooo server behavior.
func TestProxyEmptyObjectSubscription(t *testing.T) {
	var wsWg sync.WaitGroup
	var received []client.Meta[Settings]
	var mu sync.Mutex

	remote := &ooo.Server{}
	remote.Silence = true
	remote.Static = true
	remote.OpenFilter("settings")
	remote.Start("localhost:0")
	defer remote.Close(os.Interrupt)

	proxyServer := &ooo.Server{}
	proxyServer.Silence = true

	err := proxy.Route(proxyServer, "settings/{deviceID}", proxy.Config{
		Resolve: func(_ string) (string, string, error) {
			return remote.Address, "settings", nil
		},
	})
	require.NoError(t, err)

	proxyServer.Start("localhost:0")
	defer proxyServer.Close(os.Interrupt)

	ctx := t.Context()

	// Subscribe to non-existent key - should receive empty object with created=0
	wsWg.Add(1)
	go client.Subscribe(client.SubscribeConfig{
		Ctx:     ctx,
		Server:  client.Server{Protocol: "ws", Host: proxyServer.Address},
		Silence: true,
	}, "settings/device1", client.SubscribeEvents[Settings]{
		OnMessage: func(m client.Meta[Settings]) {
			mu.Lock()
			received = append(received, m)
			mu.Unlock()
			wsWg.Done()
		},
	})
	wsWg.Wait()

	// Verify we received empty object (created=0, updated=0)
	mu.Lock()
	require.Len(t, received, 1, "should receive initial empty object")
	require.Equal(t, int64(0), received[0].Created, "empty object should have created=0")
	require.Equal(t, int64(0), received[0].Updated, "empty object should have updated=0")
	mu.Unlock()

	// Now set data - subscriber should receive the update
	wsWg.Add(1)
	data, _ := json.Marshal(Settings{Name: "test", Value: 42})
	_, err = remote.Storage.Set("settings", data)
	require.NoError(t, err)
	wsWg.Wait()

	mu.Lock()
	require.Len(t, received, 2, "should receive update after data is set")
	require.Equal(t, "test", received[1].Data.Name)
	require.NotZero(t, received[1].Created, "object with data should have created > 0")
	mu.Unlock()
}

// TestProxyEmptyListSubscription tests that when subscribing to a non-existent list key,
// the proxy returns an empty array [] matching ooo server behavior.
func TestProxyEmptyListSubscription(t *testing.T) {
	var wsWg sync.WaitGroup
	var received [][]client.Meta[Thing]
	var mu sync.Mutex

	remote := &ooo.Server{}
	remote.Silence = true
	remote.Static = true
	remote.OpenFilter("things/*")
	remote.Start("localhost:0")
	defer remote.Close(os.Interrupt)

	proxyServer := &ooo.Server{}
	proxyServer.Silence = true

	err := proxy.RouteList(proxyServer, "devices/{deviceID}/things/{itemID}", proxy.Config{
		Resolve: func(localPath string) (string, string, error) {
			parts := strings.Split(localPath, "/")
			if len(parts) >= 4 {
				return remote.Address, "things/" + parts[3], nil
			}
			return remote.Address, "things/*", nil
		},
	})
	require.NoError(t, err)

	proxyServer.Start("localhost:0")
	defer proxyServer.Close(os.Interrupt)

	ctx := t.Context()

	// Subscribe to non-existent list - should receive empty array
	wsWg.Add(1)
	go client.SubscribeList(client.SubscribeConfig{
		Ctx:     ctx,
		Server:  client.Server{Protocol: "ws", Host: proxyServer.Address},
		Silence: true,
	}, "devices/dev1/things/*", client.SubscribeListEvents[Thing]{
		OnMessage: func(items []client.Meta[Thing]) {
			mu.Lock()
			received = append(received, items)
			mu.Unlock()
			wsWg.Done()
		},
	})
	wsWg.Wait()

	// Verify we received empty list
	mu.Lock()
	require.Len(t, received, 1, "should receive initial empty list")
	require.Empty(t, received[0], "empty list should have no items")
	mu.Unlock()

	// Now add an item - subscriber should receive the update
	wsWg.Add(1)
	data, _ := json.Marshal(Thing{ID: "1", Data: "first"})
	_, err = remote.Storage.Set("things/item1", data)
	require.NoError(t, err)
	wsWg.Wait()

	mu.Lock()
	require.Len(t, received, 2, "should receive update after item is added")
	require.Len(t, received[1], 1, "list should have 1 item")
	require.Equal(t, "1", received[1][0].Data.ID)
	mu.Unlock()
}

// TestProxyLateSubscriberEmptyObject tests that a late subscriber to an empty object key
// receives the correct empty meta object format.
func TestProxyLateSubscriberEmptyObject(t *testing.T) {
	var wsWg sync.WaitGroup
	var received1, received2 []client.Meta[Settings]
	var mu sync.Mutex

	remote := &ooo.Server{}
	remote.Silence = true
	remote.Static = true
	remote.OpenFilter("settings")
	remote.Start("localhost:0")
	defer remote.Close(os.Interrupt)

	proxyServer := &ooo.Server{}
	proxyServer.Silence = true

	err := proxy.Route(proxyServer, "settings/{deviceID}", proxy.Config{
		Resolve: func(_ string) (string, string, error) {
			return remote.Address, "settings", nil
		},
	})
	require.NoError(t, err)

	proxyServer.Start("localhost:0")
	defer proxyServer.Close(os.Interrupt)

	ctx := t.Context()

	// First subscriber connects to empty key
	wsWg.Add(1)
	go client.Subscribe(client.SubscribeConfig{
		Ctx:     ctx,
		Server:  client.Server{Protocol: "ws", Host: proxyServer.Address},
		Silence: true,
	}, "settings/device1", client.SubscribeEvents[Settings]{
		OnMessage: func(m client.Meta[Settings]) {
			mu.Lock()
			received1 = append(received1, m)
			mu.Unlock()
			wsWg.Done()
		},
	})
	wsWg.Wait()

	// Verify first subscriber got empty object
	mu.Lock()
	require.Len(t, received1, 1)
	require.Equal(t, int64(0), received1[0].Created, "first subscriber should get empty object")
	mu.Unlock()

	// Second subscriber joins while key is still empty (late subscriber via HTTP fetch returning 404)
	wsWg.Add(1)
	go client.Subscribe(client.SubscribeConfig{
		Ctx:     ctx,
		Server:  client.Server{Protocol: "ws", Host: proxyServer.Address},
		Silence: true,
	}, "settings/device1", client.SubscribeEvents[Settings]{
		OnMessage: func(m client.Meta[Settings]) {
			mu.Lock()
			received2 = append(received2, m)
			mu.Unlock()
			wsWg.Done()
		},
	})
	wsWg.Wait()

	// Verify second subscriber also got empty object (not null or error)
	mu.Lock()
	require.Len(t, received2, 1, "late subscriber should receive empty object")
	require.Equal(t, int64(0), received2[0].Created, "late subscriber should get empty object with created=0")
	require.Equal(t, int64(0), received2[0].Updated, "late subscriber should get empty object with updated=0")
	mu.Unlock()
}

// TestProxyLateSubscriberEmptyList tests that a late subscriber to an empty list key
// receives the correct empty array format.
func TestProxyLateSubscriberEmptyList(t *testing.T) {
	var wsWg sync.WaitGroup
	var received1, received2 [][]client.Meta[Thing]
	var mu sync.Mutex

	remote := &ooo.Server{}
	remote.Silence = true
	remote.Static = true
	remote.OpenFilter("things/*")
	remote.Start("localhost:0")
	defer remote.Close(os.Interrupt)

	proxyServer := &ooo.Server{}
	proxyServer.Silence = true

	err := proxy.RouteList(proxyServer, "devices/{deviceID}/things/{itemID}", proxy.Config{
		Resolve: func(localPath string) (string, string, error) {
			parts := strings.Split(localPath, "/")
			if len(parts) >= 4 {
				return remote.Address, "things/" + parts[3], nil
			}
			return remote.Address, "things/*", nil
		},
	})
	require.NoError(t, err)

	proxyServer.Start("localhost:0")
	defer proxyServer.Close(os.Interrupt)

	ctx := t.Context()

	// First subscriber connects to empty list
	wsWg.Add(1)
	go client.SubscribeList(client.SubscribeConfig{
		Ctx:     ctx,
		Server:  client.Server{Protocol: "ws", Host: proxyServer.Address},
		Silence: true,
	}, "devices/dev1/things/*", client.SubscribeListEvents[Thing]{
		OnMessage: func(items []client.Meta[Thing]) {
			mu.Lock()
			received1 = append(received1, items)
			mu.Unlock()
			wsWg.Done()
		},
	})
	wsWg.Wait()

	// Verify first subscriber got empty list
	mu.Lock()
	require.Len(t, received1, 1)
	require.Empty(t, received1[0], "first subscriber should get empty list")
	mu.Unlock()

	// Second subscriber joins while list is still empty (late subscriber via HTTP fetch returning 404)
	wsWg.Add(1)
	go client.SubscribeList(client.SubscribeConfig{
		Ctx:     ctx,
		Server:  client.Server{Protocol: "ws", Host: proxyServer.Address},
		Silence: true,
	}, "devices/dev1/things/*", client.SubscribeListEvents[Thing]{
		OnMessage: func(items []client.Meta[Thing]) {
			mu.Lock()
			received2 = append(received2, items)
			mu.Unlock()
			wsWg.Done()
		},
	})
	wsWg.Wait()

	// Verify second subscriber also got empty list (not null or error)
	mu.Lock()
	require.Len(t, received2, 1, "late subscriber should receive empty list")
	require.Empty(t, received2[0], "late subscriber should get empty list []")
	mu.Unlock()
}

// TestProxyWebSocketWithBearerToken tests that a WebSocket connection with a bearer token
// in the Sec-Websocket-Protocol header works through the proxy.
// The remote has NO auth - the issue is the proxy rejecting the connection when client sends token.
// This test verifies that the proxy echoes back the subprotocol to complete the handshake.
func TestProxyWebSocketWithBearerToken(t *testing.T) {
	// Use an arbitrary JWT-like token as shown in the browser screenshot
	testToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IlRlc3QifQ.fake"

	// Remote server - NO authentication required
	remote := &ooo.Server{}
	remote.Silence = true
	remote.Static = true
	remote.OpenFilter("settings")
	remote.Start("localhost:0")
	defer remote.Close(os.Interrupt)

	// Set initial data on remote
	data, _ := json.Marshal(Settings{Name: "test", Value: 42})
	_, err := remote.Storage.Set("settings", data)
	require.NoError(t, err)

	// Proxy server - no auth required at proxy level either
	proxyServer := &ooo.Server{}
	proxyServer.Silence = true

	err = proxy.Route(proxyServer, "settings/{deviceID}", proxy.Config{
		Resolve: func(_ string) (string, string, error) {
			return remote.Address, "settings", nil
		},
	})
	require.NoError(t, err)

	proxyServer.Start("localhost:0")
	defer proxyServer.Close(os.Interrupt)

	// Test WebSocket connection with Sec-Websocket-Protocol header
	// Browsers require the server to echo back the selected subprotocol
	dialer := &websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 2 * time.Second,
		Subprotocols:     []string{"bearer", testToken},
	}

	wsURL := "ws://" + proxyServer.Address + "/settings/device1"
	conn, resp, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err, "WebSocket connection should succeed with bearer token subprotocol")
	defer conn.Close()

	// Verify the server echoed back the subprotocol (required for browser compatibility)
	// CRITICAL: The server must respond with exactly "bearer" (one of the requested subprotocols),
	// NOT the full header value "bearer, <token>". Browsers reject responses that don't match
	// one of the requested subprotocols exactly.
	// This requires the upgrader to have Subprotocols: []string{"bearer"} configured.
	require.NotNil(t, resp)
	negotiatedProtocol := resp.Header.Get("Sec-Websocket-Protocol")
	require.Equal(t, "bearer", negotiatedProtocol, "proxy must negotiate exactly 'bearer' subprotocol - not the full header value")

	// Read the initial message
	_, message, err := conn.ReadMessage()
	require.NoError(t, err)
	require.NotEmpty(t, message)
}

// TestProxyWebSocketWithBearerTokenAuthRequired tests that the proxy forwards
// the bearer token to a remote server that requires authentication.
func TestProxyWebSocketWithBearerTokenAuthRequired(t *testing.T) {
	// Use an arbitrary JWT-like token
	testToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IlRlc3QifQ.valid"

	// Remote server - REQUIRES authentication via Sec-Websocket-Protocol header
	remote := &ooo.Server{}
	remote.Silence = true
	remote.Static = true
	remote.OpenFilter("settings")
	remote.Router = mux.NewRouter()
	remote.Router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check for bearer token in Sec-Websocket-Protocol header
			protocols := r.Header.Get("Sec-Websocket-Protocol")
			if protocols == "" || !strings.Contains(protocols, testToken) {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	})
	remote.Start("localhost:0")
	defer remote.Close(os.Interrupt)

	// Set initial data on remote (direct storage access bypasses middleware)
	data, _ := json.Marshal(Settings{Name: "protected", Value: 42})
	_, err := remote.Storage.Set("settings", data)
	require.NoError(t, err)

	// Proxy server - no auth at proxy level, should forward client headers to remote
	proxyServer := &ooo.Server{}
	proxyServer.Silence = true

	err = proxy.Route(proxyServer, "settings/{deviceID}", proxy.Config{
		Resolve: func(_ string) (string, string, error) {
			return remote.Address, "settings", nil
		},
	})
	require.NoError(t, err)

	proxyServer.Start("localhost:0")
	defer proxyServer.Close(os.Interrupt)

	wsURL := "ws://" + proxyServer.Address + "/settings/device1"

	// Test 1: INVALID token - should fail
	invalidToken := "invalid.token.here"
	invalidDialer := &websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 2 * time.Second,
		Subprotocols:     []string{"bearer", invalidToken},
	}

	invalidConn, _, err := invalidDialer.Dial(wsURL, nil)
	if err == nil && invalidConn != nil {
		// Connection established at proxy level, but remote should reject
		_, _, readErr := invalidConn.ReadMessage()
		require.Error(t, readErr, "should fail to read from remote with invalid token")
		invalidConn.Close()
	}
	// If dial fails, that's also acceptable - auth rejected

	// Test 2: VALID token - should succeed
	// Previous invalid connection failed, so proxy state was cleaned up
	validDialer := &websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 2 * time.Second,
		Subprotocols:     []string{"bearer", testToken},
	}

	conn, resp, err := validDialer.Dial(wsURL, nil)
	require.NoError(t, err, "WebSocket connection should succeed - proxy must forward token to authenticated remote")
	defer conn.Close()

	// Verify subprotocol was echoed back
	require.NotNil(t, resp)
	negotiatedProtocol := resp.Header.Get("Sec-Websocket-Protocol")
	require.NotEmpty(t, negotiatedProtocol, "proxy must echo back subprotocol")

	// Read the initial message - should contain the protected data
	_, message, err := conn.ReadMessage()
	require.NoError(t, err, "should receive message from authenticated remote")
	require.NotEmpty(t, message)
	require.Contains(t, string(message), "protected", "should receive protected data after successful auth")
}

// TestProxyPostBodyTooLarge asserts a proxied POST whose body exceeds
// the proxy server's MaxRequestBodyBytes is rejected with 413 instead of
// being buffered into memory. Pre-fix handleHTTPProxy did io.ReadAll on
// r.Body with no cap, so a runaway client could force the proxy to
// allocate arbitrary bytes per request.
func TestProxyPostBodyTooLarge(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	remote := &ooo.Server{}
	remote.Silence = true
	remote.Start("localhost:0")
	defer remote.Close(os.Interrupt)

	proxyServer := &ooo.Server{}
	proxyServer.Silence = true
	proxyServer.MaxRequestBodyBytes = 64

	err := proxy.Route(proxyServer, "settings/{deviceID}", proxy.Config{
		Resolve: func(_ string) (string, string, error) {
			return remote.Address, "settings", nil
		},
	})
	require.NoError(t, err)
	proxyServer.Start("localhost:0")
	defer proxyServer.Close(os.Interrupt)

	body := []byte(`{"data":"` + strings.Repeat("x", 256) + `"}`)
	resp, err := http.Post("http://"+proxyServer.Address+"/settings/device1", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
}

// TestProxyPatchBodyTooLarge is the PATCH variant of body-cap enforcement.
func TestProxyPatchBodyTooLarge(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	remote := &ooo.Server{}
	remote.Silence = true
	remote.Start("localhost:0")
	defer remote.Close(os.Interrupt)

	// Seed remote so the upstream path is reachable; the 413 should fire
	// before the upstream ever sees the body.
	_, err := remote.Storage.Set("settings", []byte(`{"name":"seed","value":1}`))
	require.NoError(t, err)

	proxyServer := &ooo.Server{}
	proxyServer.Silence = true
	proxyServer.MaxRequestBodyBytes = 64

	err = proxy.Route(proxyServer, "settings/{deviceID}", proxy.Config{
		Resolve: func(_ string) (string, string, error) {
			return remote.Address, "settings", nil
		},
	})
	require.NoError(t, err)
	proxyServer.Start("localhost:0")
	defer proxyServer.Close(os.Interrupt)

	body := []byte(`{"data":"` + strings.Repeat("x", 256) + `"}`)
	req, err := http.NewRequest(http.MethodPatch, "http://"+proxyServer.Address+"/settings/device1", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
}

// TestProxyPostBodyUnderLimit asserts a POST whose body fits under the
// proxy cap is forwarded successfully to the upstream.
func TestProxyPostBodyUnderLimit(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	remote := &ooo.Server{}
	remote.Silence = true
	remote.Start("localhost:0")
	defer remote.Close(os.Interrupt)

	proxyServer := &ooo.Server{}
	proxyServer.Silence = true
	proxyServer.MaxRequestBodyBytes = 4096

	err := proxy.Route(proxyServer, "settings/{deviceID}", proxy.Config{
		Resolve: func(_ string) (string, string, error) {
			return remote.Address, "settings", nil
		},
	})
	require.NoError(t, err)
	proxyServer.Start("localhost:0")
	defer proxyServer.Close(os.Interrupt)

	body := []byte(`{"name":"small","value":1}`)
	resp, err := http.Post("http://"+proxyServer.Address+"/settings/device1", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestProxyForwardsContentLength asserts the streamed-body proxy path
// preserves the client's Content-Length on the upstream request instead
// of switching to chunked Transfer-Encoding. Upstreams that reject
// chunked POST/PATCH (older non-Go servers, certain content-inspection
// middleware) rely on Content-Length being set; pre-fix the buffered
// path produced this for free via bytes.NewReader.
func TestProxyForwardsContentLength(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}

	var (
		mu                sync.Mutex
		seenContentLength int64
		seenTransferEnc   []string
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seenContentLength = r.ContentLength
		seenTransferEnc = append([]string(nil), r.TransferEncoding...)
		mu.Unlock()
	}))
	defer upstream.Close()
	upstreamHost := strings.TrimPrefix(upstream.URL, "http://")

	proxyServer := &ooo.Server{}
	proxyServer.Silence = true
	proxyServer.MaxRequestBodyBytes = 4096

	err := proxy.Route(proxyServer, "settings/{deviceID}", proxy.Config{
		Resolve: func(_ string) (string, string, error) {
			return upstreamHost, "", nil
		},
	})
	require.NoError(t, err)
	proxyServer.Start("localhost:0")
	defer proxyServer.Close(os.Interrupt)

	body := []byte(`{"name":"small","value":1}`)
	resp, err := http.Post("http://"+proxyServer.Address+"/settings/device1", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, int64(len(body)), seenContentLength, "upstream should see Content-Length matching client body")
	require.Empty(t, seenTransferEnc, "upstream should not see Transfer-Encoding: chunked")
}

// TestProxyForwardsChunkedFromClient asserts the streamed-body proxy
// preserves the client's chunked transfer-encoding when the client did
// not declare a Content-Length. The complement to
// TestProxyForwardsContentLength: that test guards against the streamed
// path silently switching to chunked for Content-Length clients; this
// test guards against a future change that would clamp r.ContentLength
// (or otherwise force Content-Length) and silently break chunked
// clients.
func TestProxyForwardsChunkedFromClient(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}

	var (
		mu                sync.Mutex
		seenContentLength int64
		seenTransferEnc   []string
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seenContentLength = r.ContentLength
		seenTransferEnc = append([]string(nil), r.TransferEncoding...)
		mu.Unlock()
	}))
	defer upstream.Close()
	upstreamHost := strings.TrimPrefix(upstream.URL, "http://")

	proxyServer := &ooo.Server{}
	proxyServer.Silence = true
	proxyServer.MaxRequestBodyBytes = 4096

	err := proxy.Route(proxyServer, "settings/{deviceID}", proxy.Config{
		Resolve: func(_ string) (string, string, error) {
			return upstreamHost, "", nil
		},
	})
	require.NoError(t, err)
	proxyServer.Start("localhost:0")
	defer proxyServer.Close(os.Interrupt)

	body := []byte(`{"name":"small","value":1}`)
	req, err := http.NewRequest(http.MethodPost, "http://"+proxyServer.Address+"/settings/device1", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	// Force chunked by hiding the length from the transport: stdlib
	// only auto-derives Content-Length for *bytes.Reader / *bytes.Buffer
	// / *strings.Reader bodies, so wrapping in io.NopCloser produces a
	// chunked request.
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = -1
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, int64(-1), seenContentLength, "upstream should see no Content-Length when client uses chunked")
	require.Equal(t, []string{"chunked"}, seenTransferEnc, "upstream should see chunked Transfer-Encoding")
}

// TestProxyPostBodyTooLargeDeclared asserts an honest client that
// declares an oversize Content-Length is rejected with 413 before the
// proxy opens an upstream TCP connection. The cap should be a
// pre-flight check, not just a mid-stream trip.
func TestProxyPostBodyTooLargeDeclared(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}

	var upstreamHit bool
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		upstreamHit = true
	}))
	defer upstream.Close()
	upstreamHost := strings.TrimPrefix(upstream.URL, "http://")

	proxyServer := &ooo.Server{}
	proxyServer.Silence = true
	proxyServer.MaxRequestBodyBytes = 64

	err := proxy.Route(proxyServer, "settings/{deviceID}", proxy.Config{
		Resolve: func(_ string) (string, string, error) {
			return upstreamHost, "", nil
		},
	})
	require.NoError(t, err)
	proxyServer.Start("localhost:0")
	defer proxyServer.Close(os.Interrupt)

	body := []byte(`{"data":"` + strings.Repeat("x", 256) + `"}`)
	resp, err := http.Post("http://"+proxyServer.Address+"/settings/device1", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
	require.False(t, upstreamHit, "upstream should not be contacted for a declared-oversize request")
}

// TestProxyPostBodyDisabledLimit asserts MaxRequestBodyBytes <= 0 opts the
// proxy out of body capping entirely (escape hatch for trusted callers).
func TestProxyPostBodyDisabledLimit(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	remote := &ooo.Server{}
	remote.Silence = true
	remote.Start("localhost:0")
	defer remote.Close(os.Interrupt)

	proxyServer := &ooo.Server{}
	proxyServer.Silence = true
	proxyServer.MaxRequestBodyBytes = -1

	err := proxy.Route(proxyServer, "settings/{deviceID}", proxy.Config{
		Resolve: func(_ string) (string, string, error) {
			return remote.Address, "settings", nil
		},
	})
	require.NoError(t, err)
	proxyServer.Start("localhost:0")
	defer proxyServer.Close(os.Interrupt)

	body := []byte(`{"data":"` + strings.Repeat("x", 1024) + `"}`)
	resp, err := http.Post("http://"+proxyServer.Address+"/settings/device1", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestProxyCapabilitiesWriteDeniedRejectsPost asserts a route declared
// with Capabilities{Write: false} refuses proxied POST requests with
// 403 Forbidden instead of forwarding them. Pre-fix Capabilities was
// reflected in the UI surface only and the handlers did not consult
// it, so the documented restriction was a no-op.
func TestProxyCapabilitiesWriteDeniedRejectsPost(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}

	var upstreamHit bool
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		upstreamHit = true
	}))
	defer upstream.Close()
	upstreamHost := strings.TrimPrefix(upstream.URL, "http://")

	proxyServer := &ooo.Server{}
	proxyServer.Silence = true

	err := proxy.Route(proxyServer, "settings/{deviceID}", proxy.Config{
		Capabilities: &proxy.Capabilities{Read: true, Write: false, Delete: true},
		Resolve: func(_ string) (string, string, error) {
			return upstreamHost, "", nil
		},
	})
	require.NoError(t, err)
	proxyServer.Start("localhost:0")
	defer proxyServer.Close(os.Interrupt)

	resp, err := http.Post("http://"+proxyServer.Address+"/settings/device1", "application/json", bytes.NewReader([]byte(`{"x":1}`)))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.False(t, upstreamHit, "upstream must not see denied requests")
}

// TestProxyCapabilitiesWriteDeniedRejectsPatch is the PATCH variant.
func TestProxyCapabilitiesWriteDeniedRejectsPatch(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}

	var upstreamHit bool
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		upstreamHit = true
	}))
	defer upstream.Close()
	upstreamHost := strings.TrimPrefix(upstream.URL, "http://")

	proxyServer := &ooo.Server{}
	proxyServer.Silence = true

	err := proxy.Route(proxyServer, "settings/{deviceID}", proxy.Config{
		Capabilities: &proxy.Capabilities{Read: true, Write: false, Delete: true},
		Resolve: func(_ string) (string, string, error) {
			return upstreamHost, "", nil
		},
	})
	require.NoError(t, err)
	proxyServer.Start("localhost:0")
	defer proxyServer.Close(os.Interrupt)

	req, err := http.NewRequest(http.MethodPatch, "http://"+proxyServer.Address+"/settings/device1", bytes.NewReader([]byte(`{"x":1}`)))
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.False(t, upstreamHit)
}

// TestProxyCapabilitiesReadDeniedRejectsGet asserts Read: false blocks
// GET requests through the proxy.
func TestProxyCapabilitiesReadDeniedRejectsGet(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}

	var upstreamHit bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamHit = true
		w.Write([]byte(`{}`))
	}))
	defer upstream.Close()
	upstreamHost := strings.TrimPrefix(upstream.URL, "http://")

	proxyServer := &ooo.Server{}
	proxyServer.Silence = true

	err := proxy.Route(proxyServer, "settings/{deviceID}", proxy.Config{
		Capabilities: &proxy.Capabilities{Read: false, Write: true, Delete: true},
		Resolve: func(_ string) (string, string, error) {
			return upstreamHost, "", nil
		},
	})
	require.NoError(t, err)
	proxyServer.Start("localhost:0")
	defer proxyServer.Close(os.Interrupt)

	resp, err := http.Get("http://" + proxyServer.Address + "/settings/device1")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.False(t, upstreamHit)
}

// TestProxyCapabilitiesDeleteDeniedRejectsDelete asserts Delete: false
// blocks DELETE requests through the proxy.
func TestProxyCapabilitiesDeleteDeniedRejectsDelete(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}

	var upstreamHit bool
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		upstreamHit = true
	}))
	defer upstream.Close()
	upstreamHost := strings.TrimPrefix(upstream.URL, "http://")

	proxyServer := &ooo.Server{}
	proxyServer.Silence = true

	err := proxy.Route(proxyServer, "settings/{deviceID}", proxy.Config{
		Capabilities: &proxy.Capabilities{Read: true, Write: true, Delete: false},
		Resolve: func(_ string) (string, string, error) {
			return upstreamHost, "", nil
		},
	})
	require.NoError(t, err)
	proxyServer.Start("localhost:0")
	defer proxyServer.Close(os.Interrupt)

	req, err := http.NewRequest(http.MethodDelete, "http://"+proxyServer.Address+"/settings/device1", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.False(t, upstreamHit)
}

// TestProxyCapabilitiesReadDeniedRejectsWebSocket asserts Read: false
// blocks the WebSocket upgrade — a WS subscription is a read operation
// on this codebase, so it must be gated by canRead.
func TestProxyCapabilitiesReadDeniedRejectsWebSocket(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	remote := &ooo.Server{}
	remote.Silence = true
	remote.Start("localhost:0")
	defer remote.Close(os.Interrupt)

	proxyServer := &ooo.Server{}
	proxyServer.Silence = true

	err := proxy.Route(proxyServer, "settings/{deviceID}", proxy.Config{
		Capabilities: &proxy.Capabilities{Read: false, Write: true, Delete: true},
		Resolve: func(_ string) (string, string, error) {
			return remote.Address, "settings", nil
		},
	})
	require.NoError(t, err)
	proxyServer.Start("localhost:0")
	defer proxyServer.Close(os.Interrupt)

	u := url.URL{Scheme: "ws", Host: proxyServer.Address, Path: "/settings/device1"}
	c, resp, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.Error(t, err)
	if c != nil {
		c.Close()
	}
	require.NotNil(t, resp)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// TestProxyCapabilitiesDefaultAllowsAll asserts nil Capabilities (the
// default) permits all methods. Pinning this so a future "deny by
// default" change is an explicit decision and not an accident.
func TestProxyCapabilitiesDefaultAllowsAll(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	remote := &ooo.Server{}
	remote.Silence = true
	remote.Start("localhost:0")
	defer remote.Close(os.Interrupt)

	proxyServer := &ooo.Server{}
	proxyServer.Silence = true

	err := proxy.Route(proxyServer, "settings/{deviceID}", proxy.Config{
		Resolve: func(_ string) (string, string, error) {
			return remote.Address, "settings", nil
		},
	})
	require.NoError(t, err)
	proxyServer.Start("localhost:0")
	defer proxyServer.Close(os.Interrupt)

	// POST
	resp, err := http.Post("http://"+proxyServer.Address+"/settings/device1", "application/json", bytes.NewReader([]byte(`{"name":"x","value":1}`)))
	require.NoError(t, err)
	resp.Body.Close()
	require.NotEqual(t, http.StatusForbidden, resp.StatusCode)

	// GET
	resp, err = http.Get("http://" + proxyServer.Address + "/settings/device1")
	require.NoError(t, err)
	resp.Body.Close()
	require.NotEqual(t, http.StatusForbidden, resp.StatusCode)

	// DELETE
	req, err := http.NewRequest(http.MethodDelete, "http://"+proxyServer.Address+"/settings/device1", nil)
	require.NoError(t, err)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.NotEqual(t, http.StatusForbidden, resp.StatusCode)
}

// TestProxyCapabilitiesEnforcedViaNodeFilter pins that Capabilities
// declared on a NodeFilterConfig is enforced by the underlying Route.
// NodeFilter is a convenience wrapper around Route, so the enforcement
// flows transitively — this test guards against a future refactor that
// changes that delegation.
func TestProxyCapabilitiesEnforcedViaNodeFilter(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}

	var upstreamHit bool
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		upstreamHit = true
	}))
	defer upstream.Close()
	upstreamHost := strings.TrimPrefix(upstream.URL, "http://")

	host, portStr, err := net.SplitHostPort(upstreamHost)
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	directory := &ooo.Server{}
	directory.Silence = true
	directory.Start("localhost:0")
	defer directory.Close(os.Interrupt)

	nodeData, _ := json.Marshal(proxy.Node{IP: host, Port: port})
	_, err = directory.Storage.Push("nodes/*", nodeData)
	require.NoError(t, err)
	nodes, err := ooo.GetList[proxy.Node](directory, "nodes/*")
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	nodeID := nodes[0].Index

	err = proxy.RouteNodeFilter(directory, proxy.NodeFilterConfig{
		NodesKey:     "nodes/*",
		LocalKey:     "states/*",
		RemoteKey:    "state",
		Capabilities: &proxy.Capabilities{Read: true, Write: false, Delete: true},
	})
	require.NoError(t, err)

	resp, err := http.Post("http://"+directory.Address+"/states/"+nodeID, "application/json", bytes.NewReader([]byte(`{"x":1}`)))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.False(t, upstreamHit, "upstream must not see denied requests via NodeFilter")
}

// TestProxyCapabilitiesEnforcedViaRouteWithVars pins that Capabilities
// declared on a Config passed to RouteWithVars is enforced — this
// registrar uses mux path variables and a separate handler closure
// from Route/RouteList, so it gets its own regression test.
func TestProxyCapabilitiesEnforcedViaRouteWithVars(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}

	var (
		mu          sync.Mutex
		upstreamHit bool
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		upstreamHit = true
		mu.Unlock()
	}))
	defer upstream.Close()
	upstreamHost := strings.TrimPrefix(upstream.URL, "http://")

	proxyServer := &ooo.Server{}
	proxyServer.Silence = true

	// RouteWithVars assumes Router is already initialized. Use Start
	// first to populate it (Start no-ops the listen on :0 closure
	// timing, so we register routes after Start in this path).
	proxyServer.Start("localhost:0")
	err := proxy.RouteWithVars(proxyServer, "settings/{deviceID}", proxy.Config{
		Capabilities: &proxy.Capabilities{Read: true, Write: false, Delete: true},
		Resolve: func(_ string) (string, string, error) {
			return upstreamHost, "", nil
		},
	})
	require.NoError(t, err)
	defer proxyServer.Close(os.Interrupt)

	resp, err := http.Post("http://"+proxyServer.Address+"/settings/device1", "application/json", bytes.NewReader([]byte(`{"x":1}`)))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	mu.Lock()
	defer mu.Unlock()
	require.False(t, upstreamHit, "upstream must not see denied requests via RouteWithVars")
}

// TestProxyCapabilitiesEnforcedViaNodeListFilter pins that Capabilities
// declared on a NodeListFilterConfig is enforced by the underlying
// RouteList. Mirrors TestProxyCapabilitiesEnforcedViaNodeFilter for
// the list-shaped wrapper.
func TestProxyCapabilitiesEnforcedViaNodeListFilter(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}

	var (
		mu          sync.Mutex
		upstreamHit bool
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		upstreamHit = true
		mu.Unlock()
	}))
	defer upstream.Close()
	upstreamHost := strings.TrimPrefix(upstream.URL, "http://")

	host, portStr, err := net.SplitHostPort(upstreamHost)
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	directory := &ooo.Server{}
	directory.Silence = true
	directory.Start("localhost:0")
	defer directory.Close(os.Interrupt)

	nodeData, _ := json.Marshal(proxy.Node{IP: host, Port: port})
	_, err = directory.Storage.Push("nodes/*", nodeData)
	require.NoError(t, err)
	nodes, err := ooo.GetList[proxy.Node](directory, "nodes/*")
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	nodeID := nodes[0].Index

	err = proxy.RouteNodeListFilter(directory, proxy.NodeListFilterConfig{
		NodesKey:     "nodes/*",
		LocalKey:     "device/logs/*/*",
		RemoteKey:    "logs/*",
		Capabilities: &proxy.Capabilities{Read: true, Write: false, Delete: true},
	})
	require.NoError(t, err)

	resp, err := http.Post("http://"+directory.Address+"/device/logs/"+nodeID+"/item1", "application/json", bytes.NewReader([]byte(`{"x":1}`)))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	mu.Lock()
	defer mu.Unlock()
	require.False(t, upstreamHit, "upstream must not see denied requests via NodeListFilter")
}

// denyAllMiddleware rejects every request with 401 Unauthorized.
// Used by the proxy gating tests below to assert middleware fans
// out across every shape of proxy registration.
func denyAllMiddleware() mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		})
	}
}

// TestProxyMiddlewareGatesRoute asserts that a proxied request
// through Route runs inside the middleware chain registered via
// Router.Use(). Pre-refactor proxy handlers were wrapped in an
// opinionated AuditHandler; now the gorilla/mux middleware chain is
// the single extension point and the proxy handler is registered
// naked.
func TestProxyMiddlewareGatesRoute(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}

	var upstreamHit atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		upstreamHit.Store(true)
	}))
	defer upstream.Close()
	upstreamHost := strings.TrimPrefix(upstream.URL, "http://")

	proxyServer := &ooo.Server{}
	proxyServer.Silence = true
	proxyServer.Router = mux.NewRouter()
	proxyServer.Router.Use(denyAllMiddleware())

	err := proxy.Route(proxyServer, "settings/{deviceID}", proxy.Config{
		Resolve: func(_ string) (string, string, error) {
			return upstreamHost, "", nil
		},
	})
	require.NoError(t, err)
	proxyServer.Start("localhost:0")
	defer proxyServer.Close(os.Interrupt)

	resp, err := http.Get("http://" + proxyServer.Address + "/settings/device1")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"middleware returning 401 must reject proxied Route requests")
	require.False(t, upstreamHit.Load(), "upstream must not be contacted when middleware denies")
}

// TestProxyMiddlewareGatesRouteList asserts the same for the
// list-shaped registrar — exercises BOTH closures it installs:
// itemHandler at the glob-expanded path and listHandler at the
// trimmed base path. A glob-suffix localPath ("items/*") is required
// so basePath strips to "items" and the request to /items actually
// matches the listHandler route instead of falling through to the
// data wildcard.
func TestProxyMiddlewareGatesRouteList(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}

	var upstreamHit atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		upstreamHit.Store(true)
	}))
	defer upstream.Close()
	upstreamHost := strings.TrimPrefix(upstream.URL, "http://")

	proxyServer := &ooo.Server{}
	proxyServer.Silence = true
	proxyServer.Router = mux.NewRouter()
	proxyServer.Router.Use(denyAllMiddleware())

	err := proxy.RouteList(proxyServer, "items/*", proxy.Config{
		Resolve: func(_ string) (string, string, error) {
			return upstreamHost, "", nil
		},
	})
	require.NoError(t, err)
	proxyServer.Start("localhost:0")
	defer proxyServer.Close(os.Interrupt)

	// itemHandler — registered at /items/{path1:.*}.
	resp, err := http.Get("http://" + proxyServer.Address + "/items/i1")
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"itemHandler must be gated by middleware")

	// listHandler — registered at /items (basePath). This request
	// matches the registered route directly (not the data wildcard),
	// so the 401 must come from the middleware chain.
	resp, err = http.Get("http://" + proxyServer.Address + "/items")
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"listHandler must be gated by middleware")

	require.False(t, upstreamHit.Load(), "upstream must not be contacted when middleware denies")
}

// TestProxyMiddlewareGatesWebSocketUpgrade asserts that the
// Router.Use() middleware chain runs BEFORE the proxy closure's
// websocket.IsWebSocketUpgrade branch. A WebSocket dial against a
// deny-all middleware must receive a clean HTTP 401 before any
// upgrade is attempted, so the client never enters a half-upgraded
// state. gorilla/mux's Match builds the middleware chain into the
// matched handler, so this ordering is structural; this test locks
// it against future refactors.
func TestProxyMiddlewareGatesWebSocketUpgrade(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}

	var upstreamHit atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		upstreamHit.Store(true)
	}))
	defer upstream.Close()
	upstreamHost := strings.TrimPrefix(upstream.URL, "http://")

	proxyServer := &ooo.Server{}
	proxyServer.Silence = true
	proxyServer.Router = mux.NewRouter()
	proxyServer.Router.Use(denyAllMiddleware())

	err := proxy.Route(proxyServer, "settings/{deviceID}", proxy.Config{
		Resolve: func(_ string) (string, string, error) {
			return upstreamHost, "", nil
		},
	})
	require.NoError(t, err)
	proxyServer.Start("localhost:0")
	defer proxyServer.Close(os.Interrupt)

	u := url.URL{Scheme: "ws", Host: proxyServer.Address, Path: "/settings/device1"}
	c, resp, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.Error(t, err, "deny-all middleware must reject the upgrade")
	if c != nil {
		c.Close()
	}
	require.NotNil(t, resp)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"client must see HTTP 401, not a half-upgraded WS connection")
	require.False(t, upstreamHit.Load())
}

// TestProxyMiddlewareGatesRouteWithVars asserts the same for
// RouteWithVars.
func TestProxyMiddlewareGatesRouteWithVars(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}

	var upstreamHit atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		upstreamHit.Store(true)
	}))
	defer upstream.Close()
	upstreamHost := strings.TrimPrefix(upstream.URL, "http://")

	proxyServer := &ooo.Server{}
	proxyServer.Silence = true
	proxyServer.Router = mux.NewRouter()
	proxyServer.Router.Use(denyAllMiddleware())
	// RouteWithVars assumes Router is initialized.
	proxyServer.Start("localhost:0")
	defer proxyServer.Close(os.Interrupt)

	err := proxy.RouteWithVars(proxyServer, "settings/{deviceID}", proxy.Config{
		Resolve: func(_ string) (string, string, error) {
			return upstreamHost, "", nil
		},
	})
	require.NoError(t, err)

	resp, err := http.Get("http://" + proxyServer.Address + "/settings/device1")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	require.False(t, upstreamHit.Load())
}

// TestProxySubscriberTeardownRaceDoesNotOrphanNewSubscriber
// reproduces the race the audit flagged: between the last subscriber
// emptying a proxyState and removeState deleting it from the
// registry, a new subscriber for the same path could observe the
// dying state, attach to it, and lose its upstream subscription.
// The fix flips a dying flag inside removeSubscriber, makes
// getOrCreateState replace dying states, and lets addSubscriber
// refuse a dying state so the WS handler retries. removeState then
// compare-and-deletes so it cannot evict the fresh state that took
// the dying one's place.
//
// The test uses the package-level teardownPauseHook to deterministic-
// ally hold A's teardown in the race window while B opens. Pre-fix
// B's state has no remote subscription (A's was stopped while B's
// was building on the same pointer) so the subsequent remote Set
// never reaches B. Post-fix B gets a fresh state with its own
// remote subscription and receives the update.
func TestProxySubscriberTeardownRaceDoesNotOrphanNewSubscriber(t *testing.T) {
	// Don't run in parallel — teardownPauseHook is package-level and
	// parallel tests would clobber each other.

	resume := make(chan struct{})
	hookFired := make(chan struct{}, 1)
	teardownDone := make(chan struct{}, 1)
	proxy.SetTeardownPauseHookForTest(func() {
		select {
		case hookFired <- struct{}{}:
		default:
		}
		<-resume
	})
	defer proxy.SetTeardownPauseHookForTest(nil)
	proxy.SetAfterTeardownHookForTest(func() {
		select {
		case teardownDone <- struct{}{}:
		default:
		}
	})
	defer proxy.SetAfterTeardownHookForTest(nil)

	remote := &ooo.Server{}
	remote.Silence = true
	remote.OpenFilter("settings")
	remote.Start("localhost:0")
	defer remote.Close(os.Interrupt)
	_, err := remote.Storage.Set("settings", []byte(`{"name":"seed","value":0}`))
	require.NoError(t, err)

	proxyServer := &ooo.Server{}
	proxyServer.Silence = true
	err = proxy.Route(proxyServer, "settings/{deviceID}", proxy.Config{
		Resolve: func(_ string) (string, string, error) {
			return remote.Address, "settings", nil
		},
	})
	require.NoError(t, err)
	proxyServer.Start("localhost:0")
	defer proxyServer.Close(os.Interrupt)

	u := url.URL{Scheme: "ws", Host: proxyServer.Address, Path: "/settings/device1"}

	// Subscriber A: connects, reads initial snapshot, then forces the
	// TCP socket closed so the proxy's forwarder fails on its next
	// WriteMessage and triggers the teardown defer. cA.Close() sends a
	// graceful WS close which gorilla may absorb without failing the
	// next write; UnderlyingConn().Close() is the deterministic
	// hammer.
	cA, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	_, _, err = cA.ReadMessage()
	require.NoError(t, err)
	require.NoError(t, cA.UnderlyingConn().Close())

	// Fan a few Sets at the upstream — each one broadcasts through
	// the proxy and pushes a wsMessage onto A's msgChan, eventually
	// driving A's forwarder into a failed WriteMessage that fires
	// the teardown defer.
	fired := false
	for i := range 20 {
		_, err = remote.Storage.Set("settings", []byte(fmt.Sprintf(`{"name":"wake-A","value":%d}`, i)))
		require.NoError(t, err)
		select {
		case <-hookFired:
			fired = true
		case <-time.After(100 * time.Millisecond):
		}
		if fired {
			break
		}
	}
	if !fired {
		close(resume)
		t.Fatal("teardown hook did not fire after fanning 20 Sets")
	}

	// Subscriber B: opens while A's state is still in the registry.
	// Pre-fix B attaches to the dying state — its addSubscriber sees
	// wasEmpty (A removed) and kicks off a fresh remote sub on the
	// SAME state pointer that A's teardown is about to evict from
	// the registry. Result: B's state survives but is orphaned.
	cB, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer cB.Close()
	_, _, err = cB.ReadMessage() // initial snapshot
	require.NoError(t, err)

	// Release A's teardown so removeState runs.
	close(resume)

	// Wait for A's afterTeardownHook to fire, signalling removeState
	// has returned. Pre-fix it evicts state1 (the one B is attached
	// to) from the registry; post-fix the compare-and-delete sees a
	// fresh state in the slot and skips the eviction. Either way,
	// C's subsequent dial must happen after A's removeState has
	// fully settled or the test races with the teardown.
	select {
	case <-teardownDone:
	case <-time.After(2 * time.Second):
		t.Fatal("A's teardown did not settle (afterTeardownHook never fired)")
	}

	// Subscriber C: opens for the SAME path. Pre-fix B's state was
	// evicted by A's removeState, so C creates a brand-new state3
	// with its own upstream connection — and the proxy now holds
	// TWO upstream subs to remote for the same path. Post-fix B's
	// state was preserved (either via dying-flag replacement at
	// add time or via compare-and-delete in removeState), and C
	// reuses it — proxy holds ONE upstream sub.
	cC, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer cC.Close()
	_, _, err = cC.ReadMessage() // initial snapshot
	require.NoError(t, err)

	// The remote sees the proxy's upstream subscriptions. With the
	// audit-flagged race, two parallel states each hold their own
	// upstream — connection count = 2. With the fix, B and C share
	// one state and one upstream — connection count = 1.
	deadline := time.Now().Add(1 * time.Second)
	var connCount int
	for time.Now().Before(deadline) {
		connCount = 0
		for _, pool := range remote.Stream.GetState() {
			connCount += pool.Connections
		}
		if connCount == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.Equal(t, 1, connCount,
		"proxy must share one upstream subscription across B and C; "+
			"pre-fix the teardown race leaves B on a stranded state with its own upstream "+
			"while C creates a fresh state with another, doubling the upstream conn count")
}

func TestProxyServerCloseTearsDownSubscriptions(t *testing.T) {
	var clientWg sync.WaitGroup
	var mu sync.Mutex
	var received []client.Meta[Settings]

	remote := &ooo.Server{}
	remote.Silence = true
	remote.Static = true
	remote.OpenFilter("settings")
	remote.Start("localhost:0")
	defer remote.Close(os.Interrupt)

	// Set initial data on remote
	data, _ := json.Marshal(Settings{Name: "initial", Value: 1})
	_, err := remote.Storage.Set("settings", data)
	require.NoError(t, err)

	proxyServer := &ooo.Server{}
	proxyServer.Silence = true

	err = proxy.Route(proxyServer, "settings/{deviceID}", proxy.Config{
		Resolve: func(_ string) (string, string, error) {
			return remote.Address, "settings", nil
		},
	})
	require.NoError(t, err)

	proxyServer.Start("localhost:0")

	// Subscribe to settings through proxy - use test context
	ctx := t.Context()
	clientWg.Add(1)
	go client.Subscribe(client.SubscribeConfig{
		Ctx:     ctx,
		Server:  client.Server{Protocol: "ws", Host: proxyServer.Address},
		Silence: true,
	}, "settings/device1", client.SubscribeEvents[Settings]{
		OnMessage: func(m client.Meta[Settings]) {
			mu.Lock()
			received = append(received, m)
			count := len(received)
			mu.Unlock()
			if count == 1 {
				clientWg.Done()
			}
		},
	})
	clientWg.Wait()

	// Verify we received initial data
	mu.Lock()
	require.Len(t, received, 1, "should receive initial snapshot")
	require.Equal(t, "initial", received[0].Data.Name)
	mu.Unlock()

	// Get remote's subscriber count before close using GetState()
	remoteStateBefore := remote.Stream.GetState()
	totalConnsBefore := 0
	for _, pool := range remoteStateBefore {
		totalConnsBefore += pool.Connections
	}
	require.Equal(t, 1, totalConnsBefore, "remote should have 1 subscriber from proxy")

	// Close the proxy server - this should tear down all subscriptions including remote
	proxyServer.Close(os.Interrupt)

	// Wait for remote subscription to be cleaned up (async on remote side)
	// Poll with timeout instead of sleep
	deadline := time.Now().Add(500 * time.Millisecond)
	var totalConnsAfter int
	for time.Now().Before(deadline) {
		remoteStateAfter := remote.Stream.GetState()
		totalConnsAfter = 0
		for _, pool := range remoteStateAfter {
			totalConnsAfter += pool.Connections
		}
		if totalConnsAfter == 0 {
			break
		}
		// Yield to allow remote cleanup goroutine to run
		runtime.Gosched()
	}
	require.Equal(t, 0, totalConnsAfter, "remote should have 0 subscribers after proxy server closes")
}
