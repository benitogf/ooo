package ooo

import (
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestWsTime(t *testing.T) {
	t.Parallel()
	var wg sync.WaitGroup
	app := Server{}
	app.Silence = true
	app.Tick = 1 * time.Millisecond
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)
	u := url.URL{Scheme: "ws", Host: app.Address, Path: "/"}
	c1, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	c2, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)

	// Wait for 2 messages on c1 and 1 message on c2
	c1Count := 0
	wg.Add(1) // For c1 goroutine completion
	go func() {
		defer wg.Done()
		for {
			_, message, err := c1.ReadMessage()
			if err != nil {
				app.Console.Err("read c1", err)
				break
			}
			app.Console.Log("time c1", string(message))
			c1Count++
			if c1Count >= 2 {
				break
			}
		}
	}()

	// Read one message from c2
	_, message, err := c2.ReadMessage()
	require.NoError(t, err)
	app.Console.Log("time c2", string(message))
	c2.Close()

	wg.Wait()

	err = c1.Close()
	require.NoError(t, err)
}
