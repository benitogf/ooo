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

func (app *Server) sendTime() {
	app.Stream.BroadcastClock(Time())
}

func (app *Server) startClock() {
	ticker := time.NewTicker(app.Tick)
	for {
		<-ticker.C
		if app.Active() {
			app.sendTime()
			continue
		}

		return
	}
}

func (app *Server) clock(w http.ResponseWriter, r *http.Request) {
	if !app.Audit(r) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, "%s", ErrNotAuthorized)
		app.Console.Err("socketConnectionUnauthorized time")
		return
	}

	client, err := app.Stream.New("", w, r)
	if err != nil {
		return
	}

	go app.Stream.WriteClock(client, Time())
	app.Stream.Read("", client)
}
