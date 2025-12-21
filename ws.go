package ooo

import (
	"net/http"
	"strconv"

	"github.com/benitogf/ooo/filters"
	"github.com/benitogf/ooo/key"
	"github.com/gorilla/mux"
)

func (app *Server) ws(w http.ResponseWriter, r *http.Request) error {
	_key := mux.Vars(r)["key"]
	version := r.FormValue("v")

	// Static mode validation - check if any read filter exists
	if app.Static {
		var hasFilter bool
		if key.IsGlob(_key) {
			hasFilter = app.filters.ReadList.HasMatch(_key) != -1
		} else {
			hasFilter = app.filters.ReadObject.HasMatch(_key) != -1
		}
		if !hasFilter {
			return filters.ErrRouteNotDefined
		}
	}

	client, err := app.Stream.New(_key, w, r)
	if err != nil {
		app.Console.Err("ooo: filtered route", err)
		return err
	}

	// send initial msg
	entry, err := app.fetch(_key)
	if err != nil {
		app.Console.Err("ooo: filtered route", err)
		return err
	}

	// log.Println("version", version, "entry.Version", strconv.FormatInt(entry.Version, 16), version != strconv.FormatInt(entry.Version, 16))
	if version != strconv.FormatInt(entry.Version, 16) {
		go app.Stream.Write(client, string(entry.Data), true, entry.Version)
	}
	app.Stream.Read(_key, client)
	return nil
}
