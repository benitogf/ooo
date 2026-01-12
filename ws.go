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

	client, err := server.Stream.New(_key, w, r)
	if err != nil {
		server.Console.Err("ooo: filtered route", err)
		return err
	}

	// send initial msg
	result, err := server.fetch(_key)
	if err != nil {
		server.Console.Err("ooo: filtered route", err)
		return err
	}

	// log.Println("version", version, "result.Version", strconv.FormatInt(result.Version, 16), version != strconv.FormatInt(result.Version, 16))
	if version != strconv.FormatInt(result.Version, 16) {
		server.Stream.Write(client, result.Data, true, result.Version)
	}
	server.Stream.Read(_key, client)
	return nil
}
