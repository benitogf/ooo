package ooo

import (
	"net/http"
	"strconv"

	"github.com/benitogf/ooo/filters"
	"github.com/benitogf/ooo/key"
	"github.com/gorilla/mux"
)

func (server *Server) ws(w http.ResponseWriter, r *http.Request) error {
	_key := mux.Vars(r)["key"]
	version := r.FormValue("v")

	// Static mode validation - check if any read filter exists
	if server.Static {
		var hasFilter bool
		if key.IsGlob(_key) {
			hasFilter = server.filters.ReadList.HasMatch(_key) != -1
		} else {
			// For individual items, check both ObjectFilter and ListFilter
			// This allows ReadListFilter("logs/*") to also permit subscribing to "logs/123"
			hasFilter = server.filters.ReadObject.HasMatch(_key) != -1 ||
				server.filters.ReadList.HasMatch(_key) != -1
		}
		if !hasFilter {
			return filters.ErrRouteNotDefined
		}
	}

	server.handlerWg.Add(1)
	defer server.handlerWg.Done()

	// Fetch initial data BEFORE creating WebSocket connection
	result, err := server.fetch(_key)
	if err != nil {
		server.Console.Err("ooo: filtered route", err)
		return err
	}

	// Determine if we need to send initial snapshot
	var initialData []byte
	if version != strconv.FormatInt(result.Version, 16) {
		initialData = result.Data
	}

	// Create connection and send initial snapshot atomically BEFORE joining broadcast pool
	// This prevents the race condition where broadcasts could arrive before the initial snapshot
	client, err := server.Stream.New(_key, w, r, initialData, result.Version)
	if err != nil {
		server.Console.Err("ooo: filtered route", err)
		return err
	}

	server.Stream.Read(_key, client)
	return nil
}
