package ooo

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"

	"github.com/benitogf/ooo/key"
	"github.com/benitogf/ooo/messages"
	"github.com/benitogf/ooo/meta"
	"github.com/gorilla/mux"
)

func (app *Server) publish(w http.ResponseWriter, r *http.Request) {
	if !app.Audit(r) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, "%s", ErrNotAuthorized)
		return
	}

	_key := mux.Vars(r)["key"]
	if !key.IsValid(_key) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", ErrInvalidKey)
		return
	}
	err := key.ValidateGlob(_key)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", err)
		return
	}

	event, err := messages.DecodeReader(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", err)
		return
	}

	data, err := app.filters.Write.Check(_key, event, app.Static)
	if err != nil {
		app.Console.Err("setError:filter["+_key+"]", err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", err)
		return
	}

	// Use Push for glob patterns, Set for specific keys
	var index string
	if key.IsGlob(_key) {
		index, err = app.Storage.Push(_key, data)
	} else {
		index, err = app.Storage.Set(_key, data)
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err)
		return
	}

	app.Console.Log("publish", _key)
	app.filters.AfterWrite.Check(_key)
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
	if !key.IsValid(_key) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", ErrInvalidKey)
		return
	}
	err := key.ValidateGlob(_key)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", err)
		return
	}

	event, err := messages.DecodeReader(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", err)
		return
	}

	data, err := app.filters.Write.Check(_key, event, app.Static)
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
	app.filters.AfterWrite.Check(_key)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"index":"` + index + `"}`))
}

func (app *Server) read(w http.ResponseWriter, r *http.Request) {
	_key := mux.Vars(r)["key"]
	if !key.IsValid(_key) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", ErrInvalidKey)
		return
	}

	if !app.Audit(r) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, "%s", ErrNotAuthorized)
		return
	}

	if r.Header.Get("Upgrade") == "websocket" {
		err := app.ws(w, r)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
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
		fmt.Fprintf(w, "%s", ErrEmptyKey)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(entry.Data)
}

func (app *Server) unpublish(w http.ResponseWriter, r *http.Request) {
	_key := mux.Vars(r)["key"]
	if !key.IsValid(_key) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", ErrInvalidKey)
		return
	}

	if !app.Audit(r) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, "%s", ErrNotAuthorized)
		return
	}

	err := app.filters.Delete.Check(_key, app.Static)
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
		if err == ErrNotFound || strings.Contains(err.Error(), "not found") {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		fmt.Fprintf(w, "%s", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
	w.Write([]byte(`"unpublish "+_key`))
}
