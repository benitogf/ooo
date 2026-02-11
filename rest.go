package ooo

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/benitogf/ooo/key"
	"github.com/benitogf/ooo/merge"
	"github.com/benitogf/ooo/messages"
	"github.com/benitogf/ooo/meta"
	"github.com/benitogf/ooo/stream"
	"github.com/goccy/go-json"
	"github.com/gorilla/mux"
)

func (server *Server) publish(w http.ResponseWriter, r *http.Request) {
	if !server.Audit(r) {
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

	data, err := server.filters.Write.Check(_key, event, server.Static)
	if err != nil {
		server.Console.Err("setError:filter["+_key+"]", err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", err)
		return
	}

	// Use Push for glob patterns, Set for specific keys
	var index string
	if key.IsGlob(_key) {
		index, err = server.Storage.Push(_key, data)
	} else {
		index, err = server.Storage.Set(_key, data)
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err)
		return
	}

	server.Console.Log("publish", _key)
	server.filters.AfterWrite.Check(_key)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"index":"` + index + `"}`))
}

func (server *Server) patch(w http.ResponseWriter, r *http.Request) {
	if !server.Audit(r) {
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
	if key.IsGlob(_key) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", ErrGlobNotAllowed)
		return
	}

	patchData, err := messages.DecodeReader(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", err)
		return
	}

	currentObj, err := server.Storage.Get(_key)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "%s", ErrNotFound)
		return
	}

	mergedBytes, _, err := merge.MergeBytes(currentObj.Data, patchData)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", err)
		return
	}

	data, err := server.filters.Write.Check(_key, json.RawMessage(mergedBytes), server.Static)
	if err != nil {
		server.Console.Err("patchError["+_key+"]", err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", err)
		return
	}

	index, err := server.Storage.Set(_key, data)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err)
		return
	}

	server.Console.Log("patch", _key)
	server.filters.AfterWrite.Check(_key)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"index":"` + index + `"}`))
}

func (server *Server) read(w http.ResponseWriter, r *http.Request) {
	_key := mux.Vars(r)["key"]
	if !key.IsValid(_key) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", ErrInvalidKey)
		return
	}

	if !server.Audit(r) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, "%s", ErrNotAuthorized)
		return
	}

	if r.Header.Get("Upgrade") == "websocket" {
		err := server.ws(w, r)
		if err != nil {
			// Only write HTTP status if connection was NOT hijacked
			// After hijack, the HTTP response writer is invalid
			if !errors.Is(err, stream.ErrHijacked) {
				w.WriteHeader(http.StatusBadRequest)
			}
			return
		}
		return
	}

	server.Console.Log("read", _key)
	entry, err := server.fetch(_key)
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

func (server *Server) unpublish(w http.ResponseWriter, r *http.Request) {
	_key := mux.Vars(r)["key"]
	if !key.IsValid(_key) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", ErrInvalidKey)
		return
	}

	if !server.Audit(r) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, "%s", ErrNotAuthorized)
		return
	}

	err := server.filters.Delete.Check(_key, server.Static)
	if err != nil {
		server.Console.Err("detError["+_key+"]", err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", err)
		return
	}

	server.Console.Log("unpublish", _key)
	err = server.Storage.Del(_key)

	if err != nil {
		server.Console.Err(err.Error())
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
