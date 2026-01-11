package ooo

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

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
	server.Start("localhost:9889")
	server.Start("localhost:9889")
	defer server.Close(os.Interrupt)
}

func TestRestart(t *testing.T) {
	server := Server{}
	server.Silence = true
	server.Start("localhost:0")
	addr := server.Address
	server.Close(os.Interrupt)
	// https://golang.org/pkg/net/http/#example_Server_Shutdown
	server.Start(addr)
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
