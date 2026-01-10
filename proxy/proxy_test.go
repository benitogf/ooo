package proxy_test

import (
	"bytes"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"

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
