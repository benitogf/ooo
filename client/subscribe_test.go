package client_test

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
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
	var connectedOnce sync.Once
	var disconnected sync.WaitGroup
	disconnected.Add(1)
	var disconnectedOnce sync.Once
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
				connectedOnce.Do(connected.Done)
			},
			OnError: func(err error) {
				disconnectedOnce.Do(disconnected.Done)
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
	var connectionFailedOnce sync.Once
	var exited sync.WaitGroup
	exited.Add(1)
	messageReceived := false

	go func() {
		defer exited.Done()
		client.SubscribeList(client.SubscribeConfig{
			Ctx:              ctx,
			Server:           client.Server{Protocol: "ws", Host: "127.0.0.1:1"},
			HandshakeTimeout: 10 * time.Millisecond,
			Silence:          true,
		}, "devices/*", client.SubscribeListEvents[Device]{
			OnMessage: func(devices []client.Meta[Device]) {
				messageReceived = true
			},
			OnError: func(err error) {
				connectionFailedOnce.Do(connectionFailed.Done)
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

// TestSubscribeCancelExitsPromptlyOnDialFailure asserts that cancelling the
// subscription context during a dial-failure backoff propagates within
// hundreds of milliseconds. Before the fix, the dial-failure path slept a
// fixed two seconds with no context awareness, so cancellation was delayed.
func TestSubscribeCancelExitsPromptlyOnDialFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var firstErr sync.WaitGroup
	firstErr.Add(1)
	var firstErrOnce sync.Once
	var exited sync.WaitGroup
	exited.Add(1)

	go func() {
		defer exited.Done()
		_ = client.SubscribeList(client.SubscribeConfig{
			Ctx:              ctx,
			Server:           client.Server{Protocol: "ws", Host: "127.0.0.1:1"},
			HandshakeTimeout: 10 * time.Millisecond,
			Silence:          true,
		}, "devices/*", client.SubscribeListEvents[Device]{
			OnMessage: func([]client.Meta[Device]) {},
			OnError: func(error) {
				firstErrOnce.Do(firstErr.Done)
			},
		})
	}()

	firstErr.Wait()
	cancelAt := time.Now()
	cancel()
	exited.Wait()
	require.Less(t, time.Since(cancelAt), 500*time.Millisecond,
		"Subscribe should exit within 500ms of context cancellation")
}

// TestSubscribeSuppressesOnErrorAfterCancel asserts that once the
// subscription context is cancelled, neither Subscribe nor
// SubscribeList may fire OnError again.
//
// Background: ctx cancellation is the caller's explicit "stop." The
// retry loop checks an atomic `closing` flag set by a separate
// close-watcher goroutine. Between ctx.Done firing and the watcher
// flipping that flag, the retry loop's waitRetry returns and the loop
// top may still observe closing=false — at which point it dials again,
// the dial fails against the dead server, and OnError fires.
//
// Without this guard, that spurious OnError reaches user code after
// the caller has torn down its assertion scaffolding — producing
// "panic: Fail in goroutine after test has completed" when callers
// (including the ooo test suite itself, ws_test.go) call t.Errorf from
// OnError. The contract under test: after cancel(), zero OnError
// invocations may follow.
//
// The fix has two independent guard sites — one in connect() for
// dial-failure-after-cancel, one in readLoop() for read-error-after-
// cancel. The dead-port subtests cover the connect() guard; the
// real-server subtests cover the readLoop() guard. Both are needed
// because a future refactor that removes one of the two guards
// would otherwise still pass the regression suite.
//
// All four subtests count post-cancel OnError invocations by reading
// ctx.Err() inside OnError. That is the same signal the production
// isClosing() guard reads — so the test asserts the exact contract
// the production fix promises (no OnError fires once the ctx has
// transitioned to cancelled) rather than asserting an adjacent
// main-side flag.
func TestSubscribeSuppressesOnErrorAfterCancel(t *testing.T) {
	t.Run("Subscribe", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var firstErr sync.WaitGroup
		firstErr.Add(1)
		var firstErrOnce sync.Once
		var exited sync.WaitGroup
		exited.Add(1)

		var errsAfterCancel atomic.Int32

		go func() {
			defer exited.Done()
			_ = client.Subscribe(client.SubscribeConfig{
				Ctx:              ctx,
				Server:           client.Server{Protocol: "ws", Host: "127.0.0.1:1"},
				HandshakeTimeout: 10 * time.Millisecond,
				Silence:          true,
			}, "device", client.SubscribeEvents[Device]{
				OnMessage: func(client.Meta[Device]) {},
				OnError: func(error) {
					// ctx.Err(): see test doc — canonical cancel-observed signal.
					if ctx.Err() != nil {
						errsAfterCancel.Add(1)
					}
					firstErrOnce.Do(firstErr.Done)
				},
			})
		}()

		firstErr.Wait()
		cancel()
		exited.Wait()

		require.Zero(t, errsAfterCancel.Load(),
			"Subscribe must not fire OnError once ctx has been cancelled")
	})

	t.Run("SubscribeList", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var firstErr sync.WaitGroup
		firstErr.Add(1)
		var firstErrOnce sync.Once
		var exited sync.WaitGroup
		exited.Add(1)

		var errsAfterCancel atomic.Int32

		go func() {
			defer exited.Done()
			_ = client.SubscribeList(client.SubscribeConfig{
				Ctx:              ctx,
				Server:           client.Server{Protocol: "ws", Host: "127.0.0.1:1"},
				HandshakeTimeout: 10 * time.Millisecond,
				Silence:          true,
			}, "devices/*", client.SubscribeListEvents[Device]{
				OnMessage: func([]client.Meta[Device]) {},
				OnError: func(error) {
					// ctx.Err(): see test doc — canonical cancel-observed signal.
					if ctx.Err() != nil {
						errsAfterCancel.Add(1)
					}
					firstErrOnce.Do(firstErr.Done)
				},
			})
		}()

		firstErr.Wait()
		cancel()
		exited.Wait()

		require.Zero(t, errsAfterCancel.Load(),
			"SubscribeList must not fire OnError once ctx has been cancelled")
	})

	// The two real-server subtests below cover the readLoop() guard:
	// they let the subscription connect successfully, deliver one
	// snapshot, then cancel — driving the close-watcher → ReadMessage
	// error path that the dead-port subtests cannot reach.

	t.Run("Subscribe/RealServerReadLoop", func(t *testing.T) {
		server := ooo.Server{}
		server.Silence = true
		server.Start("localhost:0")
		defer server.Close(os.Interrupt)

		// Seed a value so the individual Subscribe gets a non-empty
		// initial snapshot and reaches readLoop.
		_, err := server.Storage.Set("devices/d0", json.RawMessage(`{"name":"d0"}`))
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var connected sync.WaitGroup
		connected.Add(1)
		var connectedOnce sync.Once
		var exited sync.WaitGroup
		exited.Add(1)

		var errsAfterCancel atomic.Int32

		go func() {
			defer exited.Done()
			_ = client.Subscribe(client.SubscribeConfig{
				Ctx:     ctx,
				Server:  client.Server{Protocol: "ws", Host: server.Address},
				Silence: true,
			}, "devices/d0", client.SubscribeEvents[Device]{
				OnMessage: func(client.Meta[Device]) {
					connectedOnce.Do(connected.Done)
				},
				OnError: func(error) {
					// ctx.Err(): see test doc — canonical cancel-observed signal.
					if ctx.Err() != nil {
						errsAfterCancel.Add(1)
					}
				},
			})
		}()

		connected.Wait()
		cancel()
		exited.Wait()

		require.Zero(t, errsAfterCancel.Load(),
			"Subscribe must not fire OnError on the readLoop teardown path after cancel")
	})

	t.Run("SubscribeList/RealServerReadLoop", func(t *testing.T) {
		server := ooo.Server{}
		server.Silence = true
		server.Start("localhost:0")
		defer server.Close(os.Interrupt)

		_, err := server.Storage.Set("devices/d0", json.RawMessage(`{"name":"d0"}`))
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var connected sync.WaitGroup
		connected.Add(1)
		var connectedOnce sync.Once
		var exited sync.WaitGroup
		exited.Add(1)

		var errsAfterCancel atomic.Int32

		go func() {
			defer exited.Done()
			_ = client.SubscribeList(client.SubscribeConfig{
				Ctx:     ctx,
				Server:  client.Server{Protocol: "ws", Host: server.Address},
				Silence: true,
			}, "devices/*", client.SubscribeListEvents[Device]{
				OnMessage: func([]client.Meta[Device]) {
					connectedOnce.Do(connected.Done)
				},
				OnError: func(error) {
					// ctx.Err(): see test doc — canonical cancel-observed signal.
					if ctx.Err() != nil {
						errsAfterCancel.Add(1)
					}
				},
			})
		}()

		connected.Wait()
		cancel()
		exited.Wait()

		require.Zero(t, errsAfterCancel.Load(),
			"SubscribeList must not fire OnError on the readLoop teardown path after cancel")
	})
}
