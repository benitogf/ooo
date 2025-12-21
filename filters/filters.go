package filters

import (
	"fmt"

	"github.com/goccy/go-json"

	"github.com/benitogf/ooo/key"
	"github.com/benitogf/ooo/meta"
)

var (
	ErrRouteNotDefined     = fmt.Errorf("filters: route not defined")
	ErrInvalidFilterResult = fmt.Errorf("filters: invalid filter result error")
)

// Apply filter function for write operations (works with raw JSON)
// key: the key to filter
// data: the data received
// returns
// data: to be stored
// error: will prevent data to pass the filter
type Apply func(key string, data json.RawMessage) (json.RawMessage, error)

// ApplyObject filter function for single meta.Object read operations
type ApplyObject func(key string, obj meta.Object) (meta.Object, error)

// ApplyList filter function for []meta.Object read operations
type ApplyList func(key string, objs []meta.Object) ([]meta.Object, error)

// Block callback function for delete operations
type Block func(key string) error

// Notify callback after a write is done
type Notify func(key string)

type block struct {
	path  string
	apply Block
}

// filter for write operations (JSON-based)
type filter struct {
	path  string
	apply Apply
}

// objectFilter for single meta.Object read operations
type objectFilter struct {
	path  string
	apply ApplyObject
}

// listFilter for []meta.Object read operations
type listFilter struct {
	path  string
	apply ApplyList
}

type watch struct {
	path  string
	apply Notify
}

// Filter for write operations (JSON-based)
type Filter []filter

// ObjectFilter for single meta.Object read operations
type ObjectFilter []objectFilter

// ListFilter for []meta.Object read operations
type ListFilter []listFilter

type Blocker []block

type Watcher []watch

// Filters for read and write operations
type Filters struct {
	Write      Filter       // Write filters (JSON-based)
	ReadObject ObjectFilter // Read filters for single objects
	ReadList   ListFilter   // Read filters for lists
	Delete     Blocker
	AfterWrite Watcher
}

// AddDelete adds a delete filter
func (f *Filters) AddDelete(path string, apply Block) {
	f.Delete = append(f.Delete, block{
		path:  path,
		apply: apply,
	})
}

// AddWrite adds a write filter
func (f *Filters) AddWrite(path string, apply Apply) {
	f.Write = append(f.Write, filter{
		path:  path,
		apply: apply,
	})
}

// AddAfterWrite adds an after-write watcher
func (f *Filters) AddAfterWrite(path string, apply Notify) {
	f.AfterWrite = append(f.AfterWrite, watch{
		path:  path,
		apply: apply,
	})
}

// AddReadObject adds a read filter for single meta.Object
func (f *Filters) AddReadObject(path string, apply ApplyObject) {
	f.ReadObject = append(f.ReadObject, objectFilter{
		path:  path,
		apply: apply,
	})
}

// AddReadList adds a read filter for []meta.Object
func (f *Filters) AddReadList(path string, apply ApplyList) {
	f.ReadList = append(f.ReadList, listFilter{
		path:  path,
		apply: apply,
	})
}

// NoopHook open noop hook
func NoopHook(index string) error {
	return nil
}

// NoopFilter noop filter for write operations
func NoopFilter(index string, data json.RawMessage) (json.RawMessage, error) {
	return data, nil
}

// NoopObjectFilter noop filter for single meta.Object
func NoopObjectFilter(key string, obj meta.Object) (meta.Object, error) {
	return obj, nil
}

// NoopListFilter noop filter for []meta.Object
func NoopListFilter(key string, objs []meta.Object) ([]meta.Object, error) {
	return objs, nil
}

// AddOpenObject adds noop filters for single object paths
func (f *Filters) AddOpenObject(name string) {
	f.AddWrite(name, NoopFilter)
	f.AddReadObject(name, NoopObjectFilter)
	f.AddDelete(name, NoopHook)
}

// AddOpenList adds noop filters for list paths
func (f *Filters) AddOpenList(name string) {
	f.AddWrite(name, NoopFilter)
	f.AddReadList(name, NoopListFilter)
	f.AddDelete(name, NoopHook)
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

// Check finds and executes the first matching watcher for the given path.
// Only the first matching watcher is called (first-match-only behavior).
func (r Watcher) Check(path string) {
	match := findMatch(r, path, func(w watch) string { return w.path })
	if match == -1 {
		return
	}
	r[match].apply(path)
}

// Check finds and executes the first matching block for the given path.
// In static mode, returns an error if no matching block is found.
// Only the first matching block is called (first-match-only behavior).
func (r Blocker) Check(path string, static bool) error {
	match := findMatch(r, path, func(h block) string { return h.path })

	if match == -1 && !static {
		return nil
	}

	if match == -1 && static {
		return fmt.Errorf("%w, key:%s", ErrRouteNotDefined, path)
	}

	return r[match].apply(path)
}

// CheckStatic verifies that a matching filter exists for the given path.
// In static mode, returns an error if no matching filter is found.
// Only checks for the first match (first-match-only behavior).
func (r Filter) CheckStatic(path string, static bool) error {
	match := findMatch(r, path, func(f filter) string { return f.path })

	if match == -1 && !static {
		return nil
	}

	if match == -1 && static {
		return fmt.Errorf("%w, key:%s", ErrRouteNotDefined, path)
	}

	return nil
}

// HasMatch returns the index of a matching filter, or -1 if none exists.
// This allows callers to check if a filter exists before preparing data.
func (r Filter) HasMatch(path string) int {
	return findMatch(r, path, func(f filter) string { return f.path })
}

// Apply executes the filter at the given index. Caller must ensure index is valid.
func (r Filter) Apply(index int, path string, data json.RawMessage) (json.RawMessage, error) {
	filtered, err := r[index].apply(path, data)
	if err != nil {
		return nil, err
	}

	if len(filtered) == 0 || !json.Valid(filtered) {
		return nil, fmt.Errorf("%w, key:%s", ErrInvalidFilterResult, path)
	}

	return filtered, nil
}

// Check finds and executes the first matching filter for the given path.
// In static mode, returns an error if no matching filter is found.
// Only the first matching filter is applied (first-match-only behavior).
func (r Filter) Check(path string, data json.RawMessage, static bool) (json.RawMessage, error) {
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

	if len(filtered) == 0 || !json.Valid(filtered) {
		return nil, fmt.Errorf("%w, key:%s", ErrInvalidFilterResult, path)
	}

	return filtered, nil
}

// HasMatch returns the index of a matching filter, or -1 if none exists.
func (r ObjectFilter) HasMatch(path string) int {
	return findMatch(r, path, func(f objectFilter) string { return f.path })
}

// Check finds and executes the first matching object filter for the given path.
// In static mode, returns an error if no matching filter is found.
// Returns input object if no match and not static.
func (r ObjectFilter) Check(path string, obj meta.Object, static bool) (meta.Object, error) {
	match := findMatch(r, path, func(f objectFilter) string { return f.path })

	if match == -1 && !static {
		return obj, nil
	}

	if match == -1 && static {
		return meta.Object{}, fmt.Errorf("%w, key:%s", ErrRouteNotDefined, path)
	}

	return r[match].apply(path, obj)
}

// HasMatch returns the index of a matching filter, or -1 if none exists.
func (r ListFilter) HasMatch(path string) int {
	return findMatch(r, path, func(f listFilter) string { return f.path })
}

// Check finds and executes the first matching list filter for the given path.
// In static mode, returns an error if no matching filter is found.
// Returns input slice if no match and not static.
func (r ListFilter) Check(path string, objs []meta.Object, static bool) ([]meta.Object, error) {
	match := findMatch(r, path, func(f listFilter) string { return f.path })

	if match == -1 && !static {
		return objs, nil
	}

	if match == -1 && static {
		return nil, fmt.Errorf("%w, key:%s", ErrRouteNotDefined, path)
	}

	return r[match].apply(path, objs)
}

// Paths returns all unique registered filter paths.
// This is useful for preallocating stream pools based on configured routes.
func (f *Filters) Paths() []string {
	seen := make(map[string]struct{})

	for _, filter := range f.Write {
		seen[filter.path] = struct{}{}
	}
	for _, filter := range f.ReadObject {
		seen[filter.path] = struct{}{}
	}
	for _, filter := range f.ReadList {
		seen[filter.path] = struct{}{}
	}
	for _, filter := range f.Delete {
		seen[filter.path] = struct{}{}
	}
	for _, filter := range f.AfterWrite {
		seen[filter.path] = struct{}{}
	}

	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	return paths
}
