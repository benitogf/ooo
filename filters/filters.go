package filters

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/goccy/go-json"

	"github.com/benitogf/ooo/key"
	"github.com/benitogf/ooo/meta"
)

// reflectSchema generates a schema from a Go type using reflection
func reflectSchema(t any) map[string]any {
	if t == nil {
		return nil
	}

	rv := reflect.ValueOf(t)
	rt := rv.Type()

	// Handle pointer types
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
		rv = reflect.New(rt).Elem()
	}

	if rt.Kind() != reflect.Struct {
		return map[string]any{"type": rt.Kind().String()}
	}

	schema := make(map[string]any)
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Get JSON tag name
		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}

		parts := strings.Split(jsonTag, ",")
		fieldName := field.Name
		if parts[0] != "" {
			fieldName = parts[0]
		}

		// Skip omitempty fields in request schema - they're optional
		hasOmitempty := false
		for _, opt := range parts[1:] {
			if opt == "omitempty" {
				hasOmitempty = true
				break
			}
		}
		if hasOmitempty {
			continue
		}

		// Generate default value based on type
		schema[fieldName] = defaultValue(field.Type)
	}

	return schema
}

// defaultValue returns a default value for a type
func defaultValue(t reflect.Type) any {
	switch t.Kind() {
	case reflect.String:
		return ""
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return 0
	case reflect.Float32, reflect.Float64:
		return 0.0
	case reflect.Bool:
		return false
	case reflect.Slice:
		elemDefault := defaultValue(t.Elem())
		return []any{elemDefault}
	case reflect.Map:
		return map[string]any{}
	case reflect.Struct:
		// Recursively handle nested structs
		nested := make(map[string]any)
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if !field.IsExported() {
				continue
			}
			jsonTag := field.Tag.Get("json")
			if jsonTag == "-" {
				continue
			}
			parts := strings.Split(jsonTag, ",")
			fieldName := field.Name
			if parts[0] != "" {
				fieldName = parts[0]
			}
			// Skip omitempty fields
			hasOmitempty := false
			for _, opt := range parts[1:] {
				if opt == "omitempty" {
					hasOmitempty = true
					break
				}
			}
			if hasOmitempty {
				continue
			}
			nested[fieldName] = defaultValue(field.Type)
		}
		return nested
	case reflect.Ptr:
		return defaultValue(t.Elem())
	default:
		return nil
	}
}

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

// Config provides optional configuration for filters.
// Pass this as a variadic argument to filter methods to add metadata.
type Config struct {
	Description string
	Schema      any // Go type for the data schema (used for UI display)
}

type block struct {
	path        string
	apply       Block
	description string
	schema      any
}

// filter for write operations (JSON-based)
type filter struct {
	path        string
	apply       Apply
	description string
	schema      any
}

// objectFilter for single meta.Object read operations
type objectFilter struct {
	path        string
	apply       ApplyObject
	description string
	schema      any
}

// listFilter for []meta.Object read operations
type listFilter struct {
	path        string
	apply       ApplyList
	description string
	schema      any
}

type watch struct {
	path        string
	apply       Notify
	description string
	schema      any
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
func (f *Filters) AddDelete(path string, apply Block, cfg ...Config) {
	var desc string
	var schema any
	if len(cfg) > 0 {
		desc = cfg[0].Description
		schema = cfg[0].Schema
	}
	f.Delete = append(f.Delete, block{
		path:        path,
		apply:       apply,
		description: desc,
		schema:      schema,
	})
}

// AddWrite adds a write filter
func (f *Filters) AddWrite(path string, apply Apply, cfg ...Config) {
	var desc string
	var schema any
	if len(cfg) > 0 {
		desc = cfg[0].Description
		schema = cfg[0].Schema
	}
	f.Write = append(f.Write, filter{
		path:        path,
		apply:       apply,
		description: desc,
		schema:      schema,
	})
}

// AddAfterWrite adds an after-write watcher
func (f *Filters) AddAfterWrite(path string, apply Notify, cfg ...Config) {
	var desc string
	var schema any
	if len(cfg) > 0 {
		desc = cfg[0].Description
		schema = cfg[0].Schema
	}
	f.AfterWrite = append(f.AfterWrite, watch{
		path:        path,
		apply:       apply,
		description: desc,
		schema:      schema,
	})
}

// AddReadObject adds a read filter for single meta.Object
func (f *Filters) AddReadObject(path string, apply ApplyObject, cfg ...Config) {
	var desc string
	var schema any
	if len(cfg) > 0 {
		desc = cfg[0].Description
		schema = cfg[0].Schema
	}
	f.ReadObject = append(f.ReadObject, objectFilter{
		path:        path,
		apply:       apply,
		description: desc,
		schema:      schema,
	})
}

// AddReadList adds a read filter for []meta.Object
func (f *Filters) AddReadList(path string, apply ApplyList, cfg ...Config) {
	var desc string
	var schema any
	if len(cfg) > 0 {
		desc = cfg[0].Description
		schema = cfg[0].Schema
	}
	f.ReadList = append(f.ReadList, listFilter{
		path:        path,
		apply:       apply,
		description: desc,
		schema:      schema,
	})
}

// NoopHook open noop hook
func NoopHook(index string) error {
	return nil
}

// NoopNotify open noop notify
func NoopNotify(index string) {}

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
func (f *Filters) AddOpenObject(name string, cfg ...Config) {
	f.AddWrite(name, NoopFilter, cfg...)
	f.AddReadObject(name, NoopObjectFilter, cfg...)
	f.AddDelete(name, NoopHook, cfg...)
}

// AddOpenList adds noop filters for list paths
func (f *Filters) AddOpenList(name string, cfg ...Config) {
	f.AddWrite(name, NoopFilter, cfg...)
	f.AddReadList(name, NoopListFilter, cfg...)
	f.AddDelete(name, NoopHook, cfg...)
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

// CheckWithListFallback is like Check but also accepts if a matching ListFilter exists.
// This allows ReadListFilter("logs/*") to also permit reading individual entries like "logs/123".
func (r ObjectFilter) CheckWithListFallback(path string, obj meta.Object, static bool, listFilters ListFilter) (meta.Object, error) {
	match := findMatch(r, path, func(f objectFilter) string { return f.path })

	if match == -1 && !static {
		return obj, nil
	}

	if match == -1 && static {
		// Check if there's a matching list filter that would cover this path
		listMatch := listFilters.HasMatch(path)
		if listMatch != -1 {
			// A list filter covers this path, allow the read with no transformation
			return obj, nil
		}
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

// FilterInfo contains detailed information about a filter path
type FilterInfo struct {
	Path           string         `json:"path"`
	Type           string         `json:"type"`                     // "open", "read-only", "write-only", "limit", "custom"
	CanRead        bool           `json:"canRead"`                  // Has read filter (object or list)
	CanWrite       bool           `json:"canWrite"`                 // Has write filter
	CanDelete      bool           `json:"canDelete"`                // Has delete filter
	IsGlob         bool           `json:"isGlob"`                   // Path contains wildcard
	Limit          int            `json:"limit,omitempty"`          // Limit value if it's a limit filter
	LimitDynamic   bool           `json:"limitDynamic,omitempty"`   // True if limit uses dynamic function
	Order          string         `json:"order,omitempty"`          // Sort order for limit filters ("desc" or "asc")
	DescWrite      string         `json:"descWrite,omitempty"`      // Description for write filter
	DescRead       string         `json:"descRead,omitempty"`       // Description for read filter
	DescDelete     string         `json:"descDelete,omitempty"`     // Description for delete filter
	DescAfterWrite string         `json:"descAfterWrite,omitempty"` // Description for after-write watcher
	DescLimit      string         `json:"descLimit,omitempty"`      // Description for limit filter
	Schema         map[string]any `json:"schema,omitempty"`         // JSON schema for the data structure
}

// LimitFilterInfo stores limit filter metadata
type LimitFilterInfo struct {
	Limit        int
	LimitDynamic bool
	Order        string
	Description  string
	Schema       map[string]any
}

// PathsInfo returns detailed information about all registered filter paths.
func (f *Filters) PathsInfo(limitFilters map[string]LimitFilterInfo) []FilterInfo {
	pathInfo := make(map[string]*FilterInfo)

	// Track write filters
	for _, filter := range f.Write {
		if _, ok := pathInfo[filter.path]; !ok {
			pathInfo[filter.path] = &FilterInfo{Path: filter.path, IsGlob: key.IsGlob(filter.path)}
		}
		pathInfo[filter.path].CanWrite = true
		if filter.description != "" {
			pathInfo[filter.path].DescWrite = filter.description
		}
		if filter.schema != nil && pathInfo[filter.path].Schema == nil {
			pathInfo[filter.path].Schema = reflectSchema(filter.schema)
		}
	}

	// Track read object filters
	for _, filter := range f.ReadObject {
		if _, ok := pathInfo[filter.path]; !ok {
			pathInfo[filter.path] = &FilterInfo{Path: filter.path, IsGlob: key.IsGlob(filter.path)}
		}
		pathInfo[filter.path].CanRead = true
		if filter.description != "" {
			pathInfo[filter.path].DescRead = filter.description
		}
		if filter.schema != nil && pathInfo[filter.path].Schema == nil {
			pathInfo[filter.path].Schema = reflectSchema(filter.schema)
		}
	}

	// Track read list filters
	for _, filter := range f.ReadList {
		if _, ok := pathInfo[filter.path]; !ok {
			pathInfo[filter.path] = &FilterInfo{Path: filter.path, IsGlob: key.IsGlob(filter.path)}
		}
		pathInfo[filter.path].CanRead = true
		if filter.description != "" {
			pathInfo[filter.path].DescRead = filter.description
		}
		if filter.schema != nil && pathInfo[filter.path].Schema == nil {
			pathInfo[filter.path].Schema = reflectSchema(filter.schema)
		}
	}

	// Track delete filters
	for _, filter := range f.Delete {
		if _, ok := pathInfo[filter.path]; !ok {
			pathInfo[filter.path] = &FilterInfo{Path: filter.path, IsGlob: key.IsGlob(filter.path)}
		}
		pathInfo[filter.path].CanDelete = true
		if filter.description != "" {
			pathInfo[filter.path].DescDelete = filter.description
		}
	}

	// Track after-write watchers
	for _, filter := range f.AfterWrite {
		if _, ok := pathInfo[filter.path]; !ok {
			pathInfo[filter.path] = &FilterInfo{Path: filter.path, IsGlob: key.IsGlob(filter.path)}
		}
		if filter.description != "" {
			pathInfo[filter.path].DescAfterWrite = filter.description
		}
	}

	// Determine filter type and add limit info
	result := make([]FilterInfo, 0, len(pathInfo))
	for path, info := range pathInfo {
		// Check if it's a limit filter
		if lf, ok := limitFilters[path]; ok {
			info.Type = "limit"
			info.Limit = lf.Limit
			info.LimitDynamic = lf.LimitDynamic
			info.Order = lf.Order
			if lf.Description != "" {
				info.DescLimit = lf.Description
			}
			if lf.Schema != nil && info.Schema == nil {
				info.Schema = lf.Schema
			}
		} else if info.CanRead && info.CanWrite && info.CanDelete {
			info.Type = "open"
		} else if info.CanRead && !info.CanWrite {
			info.Type = "read-only"
		} else if info.CanWrite && !info.CanRead {
			info.Type = "write-only"
		} else {
			info.Type = "custom"
		}
		result = append(result, *info)
	}

	return result
}
