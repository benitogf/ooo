package client_test

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/benitogf/ooo"
	"github.com/benitogf/ooo/client"
	"github.com/benitogf/ooo/key"
	"github.com/stretchr/testify/require"
)

type Device struct {
	Name string `json:"name"`
}

func createDevice(t *testing.T, server *ooo.Server, name string) {
	newKey := key.Build("devices/*")
	newDevice := Device{
		Name: name,
	}

	newDeviceData, err := json.Marshal(newDevice)
	require.NoError(t, err)

	server.Storage.Set(newKey, newDeviceData)
}

func TestClientList(t *testing.T) {
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	wg := sync.WaitGroup{}

	wg.Add(1)
	go client.SubscribeList(client.SubscribeConfig{
		Ctx:     t.Context(),
		Server:  client.Server{Protocol: "ws", Host: server.Address},
		Silence: true,
	}, "devices/*", client.SubscribeListEvents[Device]{
		OnMessage: func(devices []client.Meta[Device]) {
			if len(devices) > 0 {
				sort.Slice(devices, func(i, j int) bool {
					return devices[i].Created < devices[j].Created
				})
				require.Equal(t, "device "+strconv.Itoa(len(devices)-1), devices[len(devices)-1].Data.Name)
			}
			wg.Done()
		},
	})
	wg.Wait()

	for i := range 5 {
		wg.Add(1)
		createDevice(t, &server, "device "+strconv.Itoa(i))
		wg.Wait()
	}
}

func TestClientClose(t *testing.T) {
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	ctx, cancel := context.WithCancel(context.Background())

	var connected sync.WaitGroup
	connected.Add(1)
	var exited sync.WaitGroup
	exited.Add(1)

	go func() {
		defer exited.Done()
		client.SubscribeList(client.SubscribeConfig{
			Ctx:     ctx,
			Server:  client.Server{Protocol: "ws", Host: server.Address},
			Silence: true,
		}, "devices/*", client.SubscribeListEvents[Device]{
			OnMessage: func(devices []client.Meta[Device]) {
				connected.Done()
			},
		})
	}()

	connected.Wait()
	cancel()
	exited.Wait()
}

func TestClientCloseWhileReconnecting(t *testing.T) {
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	ctx, cancel := context.WithCancel(context.Background())

	var connected sync.WaitGroup
	connected.Add(1)
	var disconnected sync.WaitGroup
	disconnected.Add(1)
	var exited sync.WaitGroup
	exited.Add(1)

	go func() {
		defer exited.Done()
		client.SubscribeList(client.SubscribeConfig{
			Ctx:     ctx,
			Server:  client.Server{Protocol: "ws", Host: server.Address},
			Silence: true,
		}, "devices/*", client.SubscribeListEvents[Device]{
			OnMessage: func(devices []client.Meta[Device]) {
				connected.Done()
			},
			OnError: func(err error) {
				disconnected.Done()
			},
		})
	}()

	connected.Wait()
	server.Close(os.Interrupt)
	disconnected.Wait()
	cancel()
	exited.Wait()
}

func TestClientCloseWithoutConnection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var connectionFailed sync.WaitGroup
	connectionFailed.Add(1)
	var exited sync.WaitGroup
	exited.Add(1)
	messageReceived := false

	go func() {
		defer exited.Done()
		client.SubscribeList(client.SubscribeConfig{
			Ctx:              ctx,
			Server:           client.Server{Protocol: "ws", Host: "notAnIP"},
			HandshakeTimeout: 10 * time.Millisecond,
			Silence:          true,
		}, "devices/*", client.SubscribeListEvents[Device]{
			OnMessage: func(devices []client.Meta[Device]) {
				messageReceived = true
			},
			OnError: func(err error) {
				connectionFailed.Done()
			},
		})
	}()

	connectionFailed.Wait()
	cancel()
	exited.Wait()
	require.False(t, messageReceived, "OnMessage should not be called when connection fails")
}

func TestClientListCallbackCurry(t *testing.T) {
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	wg := sync.WaitGroup{}

	messagesCount := 0
	makeDevicesCallback := func() func([]client.Meta[Device]) {
		return func(devices []client.Meta[Device]) {
			messagesCount++
			if len(devices) > 0 {
				require.Equal(t, "device "+strconv.Itoa(len(devices)-1), devices[len(devices)-1].Data.Name)
			}
			wg.Done()
		}
	}

	wg.Add(1)
	go client.SubscribeList(client.SubscribeConfig{
		Ctx:     t.Context(),
		Server:  client.Server{Protocol: "ws", Host: server.Address},
		Silence: true,
	}, "devices/*", client.SubscribeListEvents[Device]{
		OnMessage: makeDevicesCallback(),
	})

	wg.Wait()

	const NUM_DEVICES = 5
	for i := range NUM_DEVICES {
		wg.Add(1)
		createDevice(t, &server, "device "+strconv.Itoa(i))
		wg.Wait()
	}

	require.Equal(t, NUM_DEVICES+1, messagesCount)
}
