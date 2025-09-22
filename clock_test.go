package ooo

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestTime(t *testing.T) {
	t.Parallel()
	timeStr := Time()
	timestamp, err := strconv.ParseInt(timeStr, 10, 64)
	require.NoError(t, err)
	require.Greater(t, timestamp, int64(0))
	
	// Verify it's a recent timestamp (within last minute)
	now := time.Now().UTC().UnixNano()
	require.Less(t, now-timestamp, int64(time.Minute))
}

func TestSendTime(t *testing.T) {
	app := Server{}
	app.Silence = true
	app.Tick = 50 * time.Millisecond
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)
	
	// Test sendTime method directly
	app.sendTime()
	// Should not panic or error
}

func TestTick(t *testing.T) {
	app := Server{}
	app.Silence = true
	app.Tick = 10 * time.Millisecond
	app.Start("localhost:0")
	
	// Let it tick a few times
	time.Sleep(50 * time.Millisecond)
	
	app.Close(os.Interrupt)
	
	// Verify server is no longer active
	require.False(t, app.Active())
}

func TestClockWebsocketUnauthorized(t *testing.T) {
	app := Server{}
	app.Silence = true
	app.Audit = func(r *http.Request) bool {
		return false // Deny all requests
	}
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)
	
	u := url.URL{Scheme: "ws", Host: app.Address, Path: "/"}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.Nil(t, c)
	require.Error(t, err)
}

func TestClockWebsocketAuthorized(t *testing.T) {
	app := Server{}
	app.Silence = true
	app.Tick = 50 * time.Millisecond
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)
	
	u := url.URL{Scheme: "ws", Host: app.Address, Path: "/"}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	require.NotNil(t, c)
	
	// Read initial time message
	_, message, err := c.ReadMessage()
	require.NoError(t, err)
	
	// Verify message is a valid timestamp
	timestamp, err := strconv.ParseInt(string(message), 10, 64)
	require.NoError(t, err)
	require.Greater(t, timestamp, int64(0))
	
	c.Close()
}

func TestClockHTTPRequest(t *testing.T) {
	app := Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)
	
	// Test regular HTTP request to clock endpoint (should get stats instead)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	app.Router.ServeHTTP(w, req)
	resp := w.Result()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
}
