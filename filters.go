package ooo

import (
	"fmt"

	"github.com/goccy/go-json"

	"github.com/benitogf/ooo/key"
)

// Apply filter function
// type for functions will serve as filters
// key: the key to filter
// data: the data received or about to be sent
// returns
// data: to be stored or sent to the client
// error: will prevent data to pass the filter
type Apply func(key string, data json.RawMessage) (json.RawMessage, error)

// ApplyDelete callback function
type ApplyDelete func(key string) error

// Notify after a write is done
type Notify func(key string)

type hook struct {
	path  string
	apply ApplyDelete
}

// Filter path -> match
type filter struct {
	path  string
	apply Apply
}

type watch struct {
	path  string
	apply Notify
}

// Router group of filters
type router []filter

type hooks []hook

type watchers []watch

// Filters read and write
type filters struct {
	Write      router
	Read       router
	Delete     hooks
	AfterWrite watchers
}

// DeleteFilter add a filter that runs before sending a read result
func (app *Server) DeleteFilter(path string, apply ApplyDelete) {
	app.filters.Delete = append(app.filters.Delete, hook{
		path:  path,
		apply: apply,
	})
}

// https://github.com/golang/go/issues/11862

// WriteFilter add a filter that triggers on write
func (app *Server) WriteFilter(path string, apply Apply) {
	app.filters.Write = append(app.filters.Write, filter{
		path:  path,
		apply: apply,
	})
}

// AfterWrite add a filter that triggers after a successful write
func (app *Server) AfterWrite(path string, apply Notify) {
	app.filters.AfterWrite = append(app.filters.AfterWrite, watch{
		path:  path,
		apply: apply,
	})
}

// ReadFilter add a filter that runs before sending a read result
func (app *Server) ReadFilter(path string, apply Apply) {
	app.filters.Read = append(app.filters.Read, filter{
		path:  path,
		apply: apply,
	})
}

// NoopHook open noop hook
func NoopHook(index string) error {
	return nil
}

// NoopFilter open noop filter
func NoopFilter(index string, data json.RawMessage) (json.RawMessage, error) {
	return data, nil
}

// OpenFilter open noop read and write filters
func (app *Server) OpenFilter(name string) {
	app.WriteFilter(name, NoopFilter)
	app.ReadFilter(name, NoopFilter)
	app.DeleteFilter(name, NoopHook)
}

// findMatch returns the index of the first filter that matches the given path.
// Returns -1 if no match is found.
// Note: Only the first matching filter is applied (first-match-only behavior).
func findMatch[T any](items []T, path string, getPath func(T) string) int {
	for i, item := range items {
		filterPath := getPath(item)
		if filterPath == path || key.Match(filterPath, path) {
			return i
		}
	}
	return -1
}

// check finds and executes the first matching watcher for the given path.
// Only the first matching watcher is called (first-match-only behavior).
func (r watchers) check(path string) {
	match := findMatch(r, path, func(w watch) string { return w.path })
	if match == -1 {
		return
	}
	r[match].apply(path)
}

// check finds and executes the first matching hook for the given path.
// In static mode, returns an error if no matching hook is found.
// Only the first matching hook is called (first-match-only behavior).
func (r hooks) check(path string, static bool) error {
	match := findMatch(r, path, func(h hook) string { return h.path })

	if match == -1 && !static {
		return nil
	}

	if match == -1 && static {
		return fmt.Errorf("%w, key:%s", ErrRouteNotDefined, path)
	}

	return r[match].apply(path)
}

// checkStatic verifies that a matching filter exists for the given path.
// In static mode, returns an error if no matching filter is found.
// Only checks for the first match (first-match-only behavior).
func (r router) checkStatic(path string, static bool) error {
	match := findMatch(r, path, func(f filter) string { return f.path })

	if match == -1 && !static {
		return nil
	}

	if match == -1 && static {
		return fmt.Errorf("%w, key:%s", ErrRouteNotDefined, path)
	}

	return nil
}

// check finds and executes the first matching filter for the given path.
// In static mode, returns an error if no matching filter is found.
// Only the first matching filter is applied (first-match-only behavior).
func (r router) check(path string, data json.RawMessage, static bool) (json.RawMessage, error) {
	match := findMatch(r, path, func(f filter) string { return f.path })

	if match == -1 && !static {
		return data, nil
	}

	if match == -1 && static {
		return nil, fmt.Errorf("%w, key:%s", ErrRouteNotDefined, path)
	}

	filtered, err := r[match].apply(path, data)
	if err != nil {
		return nil, err
	}
	filteredDecoded, err := json.Marshal(filtered)
	if err != nil {
		return nil, err
	}

	if len(filteredDecoded) == 0 {
		return nil, fmt.Errorf("%w, key:%s", ErrInvalidFilterResult, path)
	}

	return filteredDecoded, nil
}
