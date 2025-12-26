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
	ticker := time.NewTicker(server.Tick)
	for {
		<-ticker.C
		if server.Active() {
			server.sendTime()
			continue
		}

		return
	}
}

func (server *Server) clock(w http.ResponseWriter, r *http.Request) {
	if !server.Audit(r) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, "%s", ErrNotAuthorized)
		server.Console.Err("socketConnectionUnauthorized time")
		return
	}

	client, err := server.Stream.New("", w, r)
	if err != nil {
		return
	}

	go server.Stream.WriteClock(client, Time())
	server.Stream.Read("", client)
}
