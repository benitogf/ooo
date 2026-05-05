package ooo

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestCloseResetsState(t *testing.T) {
	server := Server{}
	server.Silence = true
	server.Start("localhost:0")
	require.True(t, server.Active())
	require.NotNil(t, server.Router)
	require.NotNil(t, server.Storage)

	server.Close(os.Interrupt)

	// After close, internal state should be cleared
	require.Nil(t, server.Router)
	require.Nil(t, server.Storage)
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
