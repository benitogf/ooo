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
			hasFilter = server.filters.ReadObject.HasMatch(_key) != -1
		}
		if !hasFilter {
			return filters.ErrRouteNotDefined
		}
	}

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
		go server.Stream.Write(client, result.Data, true, result.Version)
	}
	server.Stream.Read(_key, client)
	return nil
}
