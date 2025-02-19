package ooo

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/benitogf/ooo/key"
	"github.com/benitogf/ooo/messages"
	"github.com/benitogf/ooo/meta"
	"github.com/gorilla/mux"
)

var (
	ErrNotAuthorized = errors.New("ooo: pathKeyError key is not valid")
)

func (app *Server) getStats(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Upgrade") == "websocket" {
		app.clock(w, r)
		return
	}
	if !app.Audit(r) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, "%s", ErrNotAuthorized)
		return
	}

	stats, err := app.Storage.Keys()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(stats)
}

func (app *Server) publish(w http.ResponseWriter, r *http.Request) {
	if !app.Audit(r) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, "%s", ErrNotAuthorized)
		return
	}

	_key := mux.Vars(r)["key"]
	countGlob := strings.Count(_key, "*")
	where := strings.Index(_key, "*")
	invalidGlobCount := countGlob > 1
	globNotAtTheEndOfPath := countGlob == 1 && where != len(_key)-1
	if !key.IsValid(_key) || invalidGlobCount || globNotAtTheEndOfPath {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", errors.New("ooo: pathKeyError key is not valid"))
		return
	}

	event, err := messages.DecodeReader(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", err)
		return
	}

	_newKey := key.Build(_key)
	data, err := app.filters.Write.check(_newKey, event, app.Static)
	if err != nil {
		app.Console.Err("setError:filter["+_newKey+"]", err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", err)
		return
	}

	index, err := app.Storage.Set(_newKey, data)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err)
		return
	}

	app.Console.Log("publish", _newKey)
	app.filters.AfterWrite.check(_newKey)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"index":"` + index + `"}`))
}

func (app *Server) republish(w http.ResponseWriter, r *http.Request) {
	if !app.Audit(r) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, "%s", ErrNotAuthorized)
		return
	}

	_key := mux.Vars(r)["key"]
	countGlob := strings.Count(_key, "*")
	where := strings.Index(_key, "*")
	invalidGlobCount := countGlob > 1
	globNotAtTheEndOfPath := countGlob == 1 && where != len(_key)-1
	if !key.IsValid(_key) || invalidGlobCount || globNotAtTheEndOfPath {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", errors.New("ooo: pathKeyError key is not valid"))
		return
	}

	event, err := messages.DecodeReader(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", err)
		return
	}

	data, err := app.filters.Write.check(_key, event, app.Static)
	if err != nil {
		app.Console.Err("setError:filter["+_key+"]", err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", err)
		return
	}

	index, err := app.Storage.Set(_key, data)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err)
		return
	}

	app.Console.Log("republish", _key)
	app.filters.AfterWrite.check(_key)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"index":"` + index + `"}`))
}

func (app *Server) patch(w http.ResponseWriter, r *http.Request) {
	if !app.Audit(r) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, "%s", ErrNotAuthorized)
		return
	}

	_key := mux.Vars(r)["key"]
	countGlob := strings.Count(_key, "*")
	where := strings.Index(_key, "*")
	invalidGlobCount := countGlob > 1
	globNotAtTheEndOfPath := countGlob == 1 && where != len(_key)-1
	if !key.IsValid(_key) || invalidGlobCount || globNotAtTheEndOfPath {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", errors.New("ooo: pathKeyError key is not valid"))
		return
	}

	event, err := messages.DecodeReader(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", err)
		return
	}

	data, err := app.filters.Write.check(_key, event, app.Static)
	if err != nil {
		app.Console.Err("setError["+_key+"]", err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", err)
		return
	}

	index, err := app.Storage.Patch(_key, data)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err)
		return
	}

	app.Console.Log("patch", _key)
	app.filters.AfterWrite.check(_key)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"index":"` + index + `"}`))
}

func (app *Server) read(w http.ResponseWriter, r *http.Request) {
	_key := mux.Vars(r)["key"]
	if !key.IsValid(_key) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", errors.New("ooo: pathKeyError key is not valid"))
		return
	}

	if !app.Audit(r) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, "%s", ErrNotAuthorized)
		return
	}

	if r.Header.Get("Upgrade") == "websocket" {
		app.ws(w, r)
		return
	}

	app.Console.Log("read", _key)
	entry, err := app.fetch(_key)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", err)
		return
	}
	if bytes.Equal(entry.Data, meta.EmptyObject) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "%s", errors.New("ooo: empty key"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(entry.Data)
}

func (app *Server) unpublish(w http.ResponseWriter, r *http.Request) {
	_key := mux.Vars(r)["key"]
	if !key.IsValid(_key) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", errors.New("ooo: pathKeyError key is not valid"))
		return
	}

	if !app.Audit(r) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, "%s", ErrNotAuthorized)
		return
	}

	err := app.filters.Delete.check(_key, app.Static)
	if err != nil {
		app.Console.Err("detError["+_key+"]", err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", err)
		return
	}

	app.Console.Log("unpublish", _key)
	err = app.Storage.Del(_key)

	if err != nil {
		app.Console.Err(err.Error())
		if err == ErrNotFound {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		fmt.Fprintf(w, "%s", err)
		return
	}

	// this performs better than the watch channel
	// if app.Storage.Watch() == nil {
	// 	go app.broadcast(_key)
	// }

	w.WriteHeader(http.StatusNoContent)
	w.Write([]byte(`"unpublish "+_key`))
}
