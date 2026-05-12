package stream

import (
	"time"

	"github.com/gorilla/websocket"
)

// BroadcastClock sends time to all the subscribers
func (sm *Stream) BroadcastClock(data string) {
	sm.mutex.RLock()
	pool := sm.clockPool
	sm.mutex.RUnlock()
	if pool == nil {
		return
	}

	// Snapshot the connection slice under pool.mutex so a concurrent
	// attachConn/Close cannot mutate it while we iterate. Writing happens
	// outside the lock so a slow client cannot stall subscribers/teardown.
	pool.mutex.RLock()
	connections := make([]*Conn, len(pool.connections))
	copy(connections, pool.connections)
	pool.mutex.RUnlock()

	for _, client := range connections {
		sm.WriteClock(client, data)
	}
}

// WriteClock sends time to a subscriber
func (sm *Stream) WriteClock(client *Conn, data string) {
	client.mutex.Lock()
	defer client.mutex.Unlock()
	client.conn.SetWriteDeadline(time.Now().Add(sm.WriteTimeout))
	err := client.conn.WriteMessage(websocket.BinaryMessage, []byte(data))
	if err != nil {
		client.conn.Close()
		sm.Console.Log("writeTimeStreamErr: ", err)
	}
}
