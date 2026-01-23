package ooo

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/benitogf/ooo/monotonic"
)

// Time returns a string timestamp using the monotonic clock
func Time() string {
	now := monotonic.Now()
	return strconv.FormatInt(now, 10)
}

func (server *Server) sendTime() {
	server.Stream.BroadcastClock(Time())
}

func (server *Server) startClock() {
	defer server.clockWg.Done()
	ticker := time.NewTicker(server.Tick)
	defer ticker.Stop()
	for {
		select {
		case <-server.clockStop:
			return
		case <-ticker.C:
			if server.Active() {
				server.sendTime()
			} else {
				return
			}
		}
	}
}

func (server *Server) clock(w http.ResponseWriter, r *http.Request) {
	if !server.Audit(r) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, "%s", ErrNotAuthorized)
		server.Console.Err("socketConnectionUnauthorized time")
		return
	}

	server.handlerWg.Add(1)
	defer server.handlerWg.Done()

	client, err := server.Stream.New("", w, r, nil, 0)
	if err != nil {
		return
	}
	// Clock uses raw text format, not the JSON envelope
	server.Stream.WriteClock(client, Time())
	server.Stream.Read("", client)
}
