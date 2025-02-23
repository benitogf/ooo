package client_test

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/benitogf/ooo"
	"github.com/benitogf/ooo/client"
	"github.com/benitogf/ooo/key"
	"github.com/pkg/expect"
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg := sync.WaitGroup{}

	wg.Add(1)
	go client.Subscribe(ctx, "ws", server.Address, "devices/*",
		func(devices []client.Meta[Device]) {
			if len(devices) > 0 {
				sort.Slice(devices, func(i, j int) bool {
					return devices[i].Created < devices[j].Created
				})
				log.Println("len", len(devices))
				require.Equal(t, "device "+strconv.Itoa(len(devices)-1), devices[len(devices)-1].Data.Name)
			}
			wg.Done()
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

	wg := sync.WaitGroup{}
	wg.Add(1)
	go client.Subscribe(ctx, "ws", server.Address, "devices/*",
		func(devices []client.Meta[Device]) {
			wg.Done()
		})
	wg.Wait()

	cancel()
	time.Sleep(10 * time.Millisecond) // wait for the connection to be closed
	createDevice(t, &server, "device null")
	time.Sleep(100 * time.Millisecond) // wait to verify that the update is not received
}

func TestClientCloseWhileReconnecting(t *testing.T) {
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	ctx, cancel := context.WithCancel(context.Background())

	wg := sync.WaitGroup{}
	wg.Add(1)
	// client.DEBUG = false
	go client.Subscribe(ctx, "ws", server.Address, "devices/*",
		func(devices []client.Meta[Device]) {
			wg.Done()
		})
	wg.Wait()

	// close server and wait for the client to start reconnection attempts
	server.Close(os.Interrupt)
	time.Sleep(1 * time.Second)

	// cancel while reconnecting in progress
	cancel()
	createDevice(t, &server, "device null")
	time.Sleep(1 * time.Second)
}

func TestClientCloseWithoutConnection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	client.HandshakeTimeout = 10 * time.Millisecond
	go client.Subscribe(ctx, "ws", "notAnIP", "devices/*",
		func(devices []client.Meta[Device]) {
			expect.True(false)
		})

	// wait for retries to stablish connection
	time.Sleep(200 * time.Millisecond)

	// cancel while trying to connect
	cancel()
	time.Sleep(200 * time.Millisecond)
}

func TestClientListCallbackCurry(t *testing.T) {
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg := sync.WaitGroup{}

	messagesCount := 0
	makeDevicesCallback := func() client.OnMessageCallback[Device] {
		return func(devices []client.Meta[Device]) {
			messagesCount++
			if len(devices) > 0 {
				require.Equal(t, "device "+strconv.Itoa(len(devices)-1), devices[len(devices)-1].Data.Name)
			}
			wg.Done()
		}
	}

	wg.Add(1)
	go client.Subscribe(ctx, "ws", server.Address, "devices/*", makeDevicesCallback())

	wg.Wait()

	const NUM_DEVICES = 5
	for i := range NUM_DEVICES {
		wg.Add(1)
		createDevice(t, &server, "device "+strconv.Itoa(i))
		wg.Wait()
	}

	require.Equal(t, NUM_DEVICES+1, messagesCount)
}
