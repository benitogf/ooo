package ooo

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/benitogf/go-json"
	"github.com/benitogf/ooo/storage"
	"github.com/stretchr/testify/require"
)

func TestDoubleShutdown(t *testing.T) {
	server := Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	server.Close(os.Interrupt)
}

// TestConcurrentCloseDoesNotPanic asserts Server.Close is safe to call
// from multiple goroutines simultaneously. Pre-fix the guard was a
// non-atomic Load + Store of `closing`, so two concurrent callers both
// observed the field == 0, both transitioned it to 1, and both
// proceeded to `close(server.clockStop)` — the second close-of-closed
// channel call panics. The fix is an atomic CompareAndSwap that lets
// only the first caller in.
//
// This is a race-detector regression test. The Load+Store window
// between the buggy guard and the close-of-channel is single-digit
// nanoseconds, narrow enough that scheduling without -race
// instrumentation rarely puts two goroutines in it on a fast CPU. The
// fan-out + iteration loop maximizes the odds, but the reliable signal
// is `go test -race`, which CI runs.
func TestConcurrentCloseDoesNotPanic(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	const (
		iterations = 100
		callers    = 32
	)
	for range iterations {
		server := &Server{}
		server.Silence = true
		server.Start("localhost:0")

		barrier := make(chan struct{})
		var (
			wg     sync.WaitGroup
			panics atomic.Int64
		)
		wg.Add(callers)
		for range callers {
			go func() {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						panics.Add(1)
					}
				}()
				<-barrier
				server.Close(os.Interrupt)
			}()
		}
		close(barrier)
		wg.Wait()
		require.Zero(t, panics.Load(), "concurrent Server.Close must not panic")
		require.False(t, server.Active(), "server must be inactive after Close")
	}
}

func TestDoubleStart(t *testing.T) {
	server := Server{}
	server.Silence = true
	server.Start("localhost:0")
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
}

func TestRestart(t *testing.T) {
	server := Server{}
	server.Silence = true
	server.Start("localhost:0")
	server.Close(os.Interrupt)
	// https://golang.org/pkg/net/http/#example_Server_Shutdown
	// Use localhost:0 again to avoid TIME_WAIT port conflicts
	server.Start("localhost:0")
	require.True(t, server.Active())
	defer server.Close(os.Interrupt)
}

func TestDeadline(t *testing.T) {
	// Test that TimeoutHandler works correctly with a custom route
	deadline := 10 * time.Millisecond
	slowHandler := http.TimeoutHandler(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.Write([]byte("done"))
		}), deadline, deadlineMsg)

	// Use httptest server to test the timeout handler directly
	ts := httptest.NewServer(slowHandler)
	defer ts.Close()

	resp, err := http.Post(ts.URL, "application/json", bytes.NewBuffer([]byte(`{"data":"test"}`)))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestServerValidate(t *testing.T) {
	// Valid config
	server := &Server{}
	require.NoError(t, server.Validate())

	// ForcePatch and NoPatch both enabled
	server = &Server{
		ForcePatch: true,
		NoPatch:    true,
	}
	err := server.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "ForcePatch and NoPatch cannot both be enabled")

	// Negative Workers
	server = &Server{
		Workers: -1,
	}
	err = server.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "Workers cannot be negative")

	// Negative Deadline
	server = &Server{
		Deadline: -1,
	}
	err = server.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "Deadline cannot be negative")
}

func TestCloseMarksServerInactive(t *testing.T) {
	server := Server{}
	server.Silence = true
	server.Start("localhost:0")
	require.True(t, server.Active())
	require.NotNil(t, server.Router)
	require.NotNil(t, server.Storage)

	server.Close(os.Interrupt)

	// Close marks the server inactive; the field references stay populated
	// so leaked handlers / callbacks don't nil-deref.
	require.False(t, server.Active())
}

// TestServerCloseShutdownBoundedByDeadline asserts that Server.Close does not
// hang forever on a stuck HTTP handler. The shutdown context must be bounded
// by server.Deadline. Pre-fix, Shutdown(context.Background()) waited for the
// blocking handler indefinitely.
func TestServerCloseShutdownBoundedByDeadline(t *testing.T) {
	server := &Server{
		Silence:  true,
		Deadline: 500 * time.Millisecond,
	}

	var entered sync.WaitGroup
	entered.Add(1)
	var enteredOnce sync.Once

	unblock := make(chan struct{})

	server.Start("localhost:0")
	server.Endpoint(EndpointConfig{
		Path: "/blocking-handler",
		Methods: Methods{
			http.MethodGet: {Response: nil},
		},
		Handler: func(w http.ResponseWriter, r *http.Request) {
			enteredOnce.Do(entered.Done)
			<-unblock
		},
	})

	var requestExited sync.WaitGroup
	requestExited.Add(1)
	go func() {
		defer requestExited.Done()
		client := &http.Client{Timeout: 30 * time.Second}
		req, _ := http.NewRequest(http.MethodGet, "http://"+server.Address+"/blocking-handler", nil)
		resp, err := client.Do(req)
		if err == nil && resp != nil {
			resp.Body.Close()
		}
	}()

	entered.Wait()

	var closed sync.WaitGroup
	closed.Add(1)
	closeStart := time.Now()
	go func() {
		defer closed.Done()
		server.Close(os.Interrupt)
	}()

	closedDone := make(chan struct{})
	go func() {
		closed.Wait()
		close(closedDone)
	}()

	select {
	case <-closedDone:
		elapsed := time.Since(closeStart)
		require.Less(t, elapsed, 3*time.Second,
			"Server.Close must be bounded by Deadline (500ms); took %s", elapsed)
	case <-time.After(5 * time.Second):
		t.Fatal("Server.Close did not return within bounded time on a stuck handler")
	}

	// Unblock the leaked handler goroutine so the in-flight request and the
	// handler exit before the test ends.
	close(unblock)
	requestExited.Wait()
}

// TestServerCloseCancelsHandlerContext asserts that Server.Close cancels
// in-flight request contexts so well-behaved handlers (those that respect
// r.Context().Done()) exit cleanly. Without the post-Shutdown force-close,
// stdlib Shutdown timing out leaves request contexts uncancelled and the
// handler goroutine leaks.
func TestServerCloseCancelsHandlerContext(t *testing.T) {
	server := &Server{
		Silence:  true,
		Deadline: 200 * time.Millisecond,
	}

	var entered sync.WaitGroup
	entered.Add(1)
	var enteredOnce sync.Once

	var handlerExited sync.WaitGroup
	handlerExited.Add(1)
	var handlerOnce sync.Once
	var contextCancelled atomic.Bool

	server.Start("localhost:0")
	server.Endpoint(EndpointConfig{
		Path: "/context-aware-handler",
		Methods: Methods{
			http.MethodGet: {Response: nil},
		},
		Handler: func(w http.ResponseWriter, r *http.Request) {
			defer handlerOnce.Do(handlerExited.Done)
			enteredOnce.Do(entered.Done)
			// Well-behaved handler: block until the request context is
			// cancelled. Close should cancel it via force-close.
			<-r.Context().Done()
			contextCancelled.Store(true)
		},
	})

	var requestExited sync.WaitGroup
	requestExited.Add(1)
	go func() {
		defer requestExited.Done()
		client := &http.Client{Timeout: 30 * time.Second}
		req, _ := http.NewRequest(http.MethodGet, "http://"+server.Address+"/context-aware-handler", nil)
		resp, err := client.Do(req)
		if err == nil && resp != nil {
			resp.Body.Close()
		}
	}()

	entered.Wait()

	server.Close(os.Interrupt)

	// Bound waits: handler must have exited via ctx.Done() by the time
	// Close returned (or shortly after the force-close took effect).
	exitedDone := make(chan struct{})
	go func() {
		handlerExited.Wait()
		close(exitedDone)
	}()
	select {
	case <-exitedDone:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not exit after Server.Close — request context was not cancelled")
	}
	requestExited.Wait()

	require.True(t, contextCancelled.Load(),
		"handler should have observed r.Context().Done() during Close")
}

// TestServerCloseDoesNotNilFieldsUnderHandler asserts that Server.Close does
// not nil fields like Storage out from under a leaked handler goroutine.
// Pre-fix, Close cleared server.Storage = nil with no synchronisation; a
// handler that resumed after Close panicked on nil deref.
func TestServerCloseDoesNotNilFieldsUnderHandler(t *testing.T) {
	server := &Server{
		Silence:  true,
		Deadline: 200 * time.Millisecond,
	}

	var entered sync.WaitGroup
	entered.Add(1)
	var enteredOnce sync.Once

	unblock := make(chan struct{})

	var handlerExited sync.WaitGroup
	handlerExited.Add(1)
	var handlerOnce sync.Once
	var handlerPanic atomic.Value

	server.Start("localhost:0")
	server.Endpoint(EndpointConfig{
		Path: "/leaky-handler",
		Methods: Methods{
			http.MethodGet: {Response: nil},
		},
		Handler: func(w http.ResponseWriter, r *http.Request) {
			defer handlerOnce.Do(handlerExited.Done)
			defer func() {
				if rec := recover(); rec != nil {
					handlerPanic.Store(fmt.Sprintf("%v", rec))
				}
			}()
			enteredOnce.Do(entered.Done)
			<-unblock
			// Touch a field that Close used to nil. Pre-fix this panics
			// with nil pointer dereference; post-fix the call returns
			// without panic (Storage may be closed but the field is non-nil).
			_, _ = server.Storage.Get("never-set")
		},
	})

	var requestExited sync.WaitGroup
	requestExited.Add(1)
	go func() {
		defer requestExited.Done()
		client := &http.Client{Timeout: 30 * time.Second}
		req, _ := http.NewRequest(http.MethodGet, "http://"+server.Address+"/leaky-handler", nil)
		resp, err := client.Do(req)
		if err == nil && resp != nil {
			resp.Body.Close()
		}
	}()

	entered.Wait()

	server.Close(os.Interrupt)

	// Release the leaked handler. Pre-fix, the handler now nil-derefs
	// because Close cleared server.Storage.
	close(unblock)
	handlerExited.Wait()
	requestExited.Wait()

	if v := handlerPanic.Load(); v != nil {
		t.Fatalf("handler panicked after Server.Close: %v", v)
	}
}

func TestStartWithError(t *testing.T) {
	server := Server{}
	server.Silence = true

	// Start successfully
	err := server.StartWithError("localhost:0")
	require.NoError(t, err)
	require.True(t, server.Active())

	// Try to start again - should return ErrServerAlreadyActive
	err = server.StartWithError("localhost:0")
	require.Error(t, err)
	require.Equal(t, ErrServerAlreadyActive, err)

	server.Close(os.Interrupt)
}

func TestActive(t *testing.T) {
	server := Server{}
	server.Silence = true

	// Not active before start
	require.False(t, server.Active())

	server.Start("localhost:0")
	require.True(t, server.Active())

	server.Close(os.Interrupt)
	require.False(t, server.Active())
}

func TestOnStart(t *testing.T) {
	started := false
	server := Server{}
	server.Silence = true
	server.OnStart = func() {
		started = true
	}

	require.False(t, started)
	server.Start("localhost:0")
	require.True(t, started)
	defer server.Close(os.Interrupt)
}

func TestOnStartComposition(t *testing.T) {
	var order []string
	server := Server{}
	server.Silence = true

	// First callback
	server.OnStart = func() {
		order = append(order, "first")
	}

	// Compose second callback
	existingOnStart := server.OnStart
	server.OnStart = func() {
		existingOnStart()
		order = append(order, "second")
	}

	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	require.Equal(t, []string{"first", "second"}, order)
}

func TestIsPivotPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path string
		want bool
	}{
		// real pivot internal paths
		{"pivot", true},
		{"pivot/", true},
		{"pivot/status", true},
		{"pivot/sync/node-1", true},

		// user keys that merely start with the letters "pivot"
		{"pivothings", false},
		{"pivots", false},
		{"pivots/x", false},
		{"pivotal", false},

		// unrelated keys
		{"", false},
		{"piv", false},
		{"things/pivot", false},
		{"users/alice", false},
	}

	for _, c := range cases {
		require.Equalf(t, c.want, isPivotPath(c.path),
			"isPivotPath(%q) = %v, want %v", c.path, !c.want, c.want)
	}
}

func TestOnWatchPanicReceivesOffendingEvent(t *testing.T) {
	type capture struct {
		ev storage.Event
		r  any
	}
	got := make(chan capture, 1)

	server := &Server{
		Silence: true,
		Workers: 1,
	}
	server.OnStorageEvent = func(ev storage.Event) {
		panic("boom")
	}
	server.OnWatchPanic = func(ev storage.Event, r any) {
		select {
		case got <- capture{ev: ev, r: r}:
		default:
		}
	}

	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	_, err := server.Storage.Set("test/key", json.RawMessage(`{"v":1}`))
	require.NoError(t, err)

	select {
	case c := <-got:
		require.Equal(t, "test/key", c.ev.Key)
		require.Equal(t, "set", c.ev.Operation)
		require.NotNil(t, c.r)
	case <-time.After(2 * time.Second):
		t.Fatal("OnWatchPanic was not invoked")
	}

	require.Equal(t, int64(1), atomic.LoadInt64(&server.WatchPanics))
}

// TestOnDroppedEventReceivesOffendingEvent asserts the Server-level
// observability hook fires when the sharded watcher drops an event after its
// send timeout, and that DroppedEvents counts the drop. Pre-fix a stuck
// watcher silently lost writes from subscribers' point of view, with only a
// log line as evidence.
func TestOnDroppedEventReceivesOffendingEvent(t *testing.T) {
	dropped := make(chan storage.Event, 4)

	server := &Server{
		Silence: true,
		Workers: 1,
	}
	// Block the worker so the shard buffer can saturate. `firstSeen` confirms
	// the worker actually dequeued the first event before the fill loop
	// starts — without that signal, the buffer math depends on goroutine
	// scheduling and the test would bump DroppedEvents to 2 or block on the
	// default 5 s Send timeout when the worker is slow to start.
	hold := make(chan struct{})
	firstSeen := make(chan struct{})
	var firstOnce sync.Once
	server.OnStorageEvent = func(ev storage.Event) {
		firstOnce.Do(func() { close(firstSeen) })
		<-hold
	}
	server.OnDroppedEvent = func(ev storage.Event) {
		select {
		case dropped <- ev:
		default:
		}
	}

	server.Start("localhost:0")
	defer func() {
		close(hold)
		server.Close(os.Interrupt)
	}()

	_, err := server.Storage.Set("first", json.RawMessage(`{"v":1}`))
	require.NoError(t, err)
	<-firstSeen

	// Saturate the shard buffer: the channel holds 100 events, and the worker
	// is blocked draining the first.
	for i := range 100 {
		_, err := server.Storage.Set(fmt.Sprintf("fill-%d", i), json.RawMessage(`{"v":1}`))
		require.NoError(t, err)
	}
	// Override the default 5 s send timeout via a short-timeout direct send so
	// the test does not wait that long.
	shardedWatcher := server.Storage.WatchSharded()
	require.False(t,
		shardedWatcher.SendWithTimeout(storage.Event{Key: "dropped", Operation: "set"}, 50*time.Millisecond),
		"event should have been dropped after timing out on a saturated shard",
	)

	select {
	case ev := <-dropped:
		require.Equal(t, "dropped", ev.Key)
	case <-time.After(2 * time.Second):
		t.Fatal("OnDroppedEvent was not invoked")
	}
	require.Equal(t, int64(1), atomic.LoadInt64(&server.DroppedEvents))
}
