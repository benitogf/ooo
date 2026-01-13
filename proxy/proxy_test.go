package proxy_test

import (
	"bytes"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/benitogf/ooo"
	"github.com/benitogf/ooo/client"
	"github.com/benitogf/ooo/proxy"
	"github.com/goccy/go-json"
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := proxy.GlobToMuxPattern(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
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
