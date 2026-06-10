package ooo

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/benitogf/go-json"
	"github.com/benitogf/ooo/key"
	"github.com/benitogf/ooo/merge"
	"github.com/benitogf/ooo/messages"
	"github.com/benitogf/ooo/meta"
	"github.com/benitogf/ooo/storage"
	"github.com/benitogf/ooo/stream"
	"github.com/gorilla/mux"
)

func (server *Server) publish(w http.ResponseWriter, r *http.Request) {
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

	body, probe := server.LimitBody(w, r)
	event, err := messages.DecodeReader(body)
	if err != nil {
		if IsRequestBodyTooLargeErr(err) || (probe != nil && IsRequestBodyTooLargeErr(probe.last)) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			fmt.Fprintf(w, "%s", http.StatusText(http.StatusRequestEntityTooLarge))
			return
		}
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
	writeIndexResponse(w, index)
}

func (server *Server) patch(w http.ResponseWriter, r *http.Request) {
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

	body, probe := server.LimitBody(w, r)
	patchData, err := messages.DecodeReader(body)
	if err != nil {
		if IsRequestBodyTooLargeErr(err) || (probe != nil && IsRequestBodyTooLargeErr(probe.last)) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			fmt.Fprintf(w, "%s", http.StatusText(http.StatusRequestEntityTooLarge))
			return
		}
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

	mergedBytes, info, err := merge.MergeBytes(currentObj.Data, patchData)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", err)
		return
	}
	// Non-fatal merge errors (e.g. type mismatch at a specific path) leave
	// the original value in place at that path. Persisting silently would
	// confirm a 200 for a partially-applied patch.
	if len(info.Errors) > 0 {
		server.Console.Err("patchError["+_key+"]", info.Errors)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", errors.Join(info.Errors...))
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
	writeIndexResponse(w, index)
}

// writeIndexResponse writes the canonical {"index":"..."} response body
// using json.Marshal so JSON-significant characters in index do not
// corrupt the response.
func writeIndexResponse(w http.ResponseWriter, index string) {
	body, err := json.Marshal(struct {
		Index string `json:"index"`
	}{Index: index})
	if err != nil {
		// json.Marshal of a string field cannot fail in practice; fall back
		// to an empty object so the client still receives valid JSON.
		body = []byte(`{}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

func (server *Server) read(w http.ResponseWriter, r *http.Request) {
	_key := mux.Vars(r)["key"]
	if !key.IsValid(_key) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", ErrInvalidKey)
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
		if errors.Is(err, storage.ErrNotFound) {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		fmt.Fprintf(w, "%s", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// LimitBody wraps r.Body with http.MaxBytesReader so a runaway POST/PATCH
// cannot buffer arbitrary bytes into memory. A non-positive
// MaxRequestBodyBytes disables the cap and returns r.Body unchanged so
// operators can opt out (test harnesses, trusted internal callers).
//
// IMPORTANT: must be called before any write to w. http.MaxBytesReader
// signals the response writer side-channel to suppress connection keep-alive
// once the cap trips, and that side-channel is a no-op after the header has
// been committed. Today both REST callers (publish, patch) short-circuit on
// key checks without writing to w first, so the invariant holds — refactor
// that path with care. Same constraint applies to any other caller (proxy
// handlers, custom Endpoints). Router.Use() middleware that denies a request
// must short-circuit (not call next) — once next runs, the handler may
// invoke LimitBody, and any subsequent write to w from a deferred middleware
// closure would arrive too late.
//
// The returned ReadErrorProbe (non-nil only when the cap is active) records
// the most recent error returned by Read. benitogf/go-json is known to
// swallow underlying reader errors as a plain io.EOF when it sees the
// truncated payload, so REST callers must consult the probe to distinguish
// "client sent too many bytes" (413) from "client sent invalid JSON" (400).
// Other consumers (notably net/http transports surfacing the body to an
// upstream) may surface a less-specific cause; the probe is the defensive
// fallback there as well.
func (server *Server) LimitBody(w http.ResponseWriter, r *http.Request) (io.Reader, *ReadErrorProbe) {
	if server.MaxRequestBodyBytes <= 0 {
		return r.Body, nil
	}
	probe := &ReadErrorProbe{inner: http.MaxBytesReader(w, r.Body, server.MaxRequestBodyBytes)}
	return probe, probe
}

// ReadErrorProbe wraps an io.Reader and remembers the most recent non-nil
// error returned by Read, so callers can recover the underlying cause even
// when an upstream decoder or transport turns it into a less-specific error
// like EOF.
type ReadErrorProbe struct {
	inner io.Reader
	last  error
}

// Read forwards to the wrapped reader and records any non-nil error so
// callers can later disambiguate the cause via Last().
func (p *ReadErrorProbe) Read(b []byte) (int, error) {
	n, err := p.inner.Read(b)
	if err != nil {
		p.last = err
	}
	return n, err
}

// Last returns the most recent error observed during Read. Returns nil if
// Read never failed.
func (p *ReadErrorProbe) Last() error {
	return p.last
}

// IsRequestBodyTooLargeErr reports whether err originated in a
// MaxBytesReader exhausting its quota. It matches both modern
// *http.MaxBytesError and the legacy "http: request body too large"
// sentinel that older stdlib paths still return.
func IsRequestBodyTooLargeErr(err error) bool {
	if err == nil {
		return false
	}
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return true
	}
	return strings.Contains(err.Error(), "http: request body too large")
}
