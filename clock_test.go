package ooo

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestTime(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	timeStr := Time()
	timestamp, err := strconv.ParseInt(timeStr, 10, 64)
	require.NoError(t, err)
	require.Greater(t, timestamp, int64(0))

	// Verify it's a recent timestamp (within last minute)
	now := time.Now().UTC().UnixNano()
	require.Less(t, now-timestamp, int64(time.Minute))
}

func TestSendTime(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true
	server.Tick = 50 * time.Millisecond
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Test sendTime method directly
	server.sendTime()
	// Should not panic or error
}

func TestTick(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true
	server.Tick = 1 * time.Millisecond
	server.Start("localhost:0")

	// Let it tick a few times
	time.Sleep(10 * time.Millisecond)

	server.Close(os.Interrupt)

	// Verify server is no longer active
	require.False(t, server.Active())
}

func TestClockWebsocketUnauthorized(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true
	server.Audit = func(r *http.Request) bool {
		return false // Deny all requests
	}
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	u := url.URL{Scheme: "ws", Host: server.Address, Path: "/"}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.Nil(t, c)
	require.Error(t, err)
}

// TestClockWebsocketAuthorized tests clock websocket connection
// Note: Uses raw websocket because clock endpoint sends raw timestamp strings, not JSON objects
func TestClockWebsocketAuthorized(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true
	server.Tick = 50 * time.Millisecond
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	u := url.URL{Scheme: "ws", Host: server.Address, Path: "/"}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	require.NotNil(t, c)
	defer c.Close()

	// Read initial time message
	_, message, err := c.ReadMessage()
	require.NoError(t, err)

	// Verify message is a valid timestamp
	timestamp, err := strconv.ParseInt(string(message), 10, 64)
	require.NoError(t, err)
	require.Greater(t, timestamp, int64(0))
}

func TestClockHTTPRequest(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Test regular HTTP request to clock endpoint (should get explorer HTML)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "text/html; charset=utf-8", resp.Header.Get("Content-Type"))
}
