package ooo

import (
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"strings"

	"github.com/benitogf/ooo/ui"
)

// Vars maps route variable name to its description (e.g., {id} in path)
type Vars map[string]string

// Params maps query parameter name to its description (e.g., ?category=x)
type Params map[string]string

// MethodSpec defines the specification for an HTTP method
type MethodSpec struct {
	Request  any    // Go type for request body, nil for GET/DELETE
	Response any    // Go type for response body, nil if status-only
	Params   Params // Query parameters like ?category=x - optional
}

// Methods maps HTTP method to its specification
type Methods map[string]MethodSpec

// EndpointConfig configures a custom endpoint.
//
// Handler contract: long-running handlers must respect r.Context().Done().
// Server.Close gives handlers a graceful window of server.Deadline to exit,
// then force-closes connections to cancel their request contexts. A handler
// that ignores the context will leak its goroutine — Go offers no way to
// preempt it.
type EndpointConfig struct {
	Path        string
	Methods     Methods
	Description string
	Vars        Vars // Route variables like {id} - mandatory, auto-extracted from path if nil
	Handler     http.HandlerFunc
}

// pathParamRegex matches mux-style path parameters like {id}
var pathParamRegex = regexp.MustCompile(`\{([^}]+)\}`)

// extractPathVars extracts variable names from a path like "/policies/{id}"
func extractPathVars(path string) Vars {
	matches := pathParamRegex.FindAllStringSubmatch(path, -1)
	if len(matches) == 0 {
		return nil
	}
	vars := make(Vars, len(matches))
	for _, match := range matches {
		vars[match[1]] = match[1]
	}
	return vars
}

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

// AuditHandler wraps next so the configured Server.Audit gate runs
// before the handler. Returns 401 Unauthorized when Audit denies the
// request. Used by Endpoint registration and the proxy package so
// custom routes participate in the same auth/rate-limit/observability
// path as the built-in REST handlers.
//
// The nil-Audit branch is defensive for direct-Router test paths
// (ServeHTTP bypassing Start). Production callers reach this wrapper
// only after Start has populated the default Audit via defaults().
func (server *Server) AuditHandler(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if server.Audit != nil && !server.Audit(r) {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, "%s", ErrNotAuthorized)
			return
		}
		next(w, r)
	}
}

// Endpoint registers a custom HTTP endpoint with metadata for UI visibility
func (server *Server) Endpoint(cfg EndpointConfig) {
	methods := make([]string, 0, len(cfg.Methods))
	methodInfos := make([]ui.MethodInfo, 0, len(cfg.Methods))

	for method, spec := range cfg.Methods {
		methods = append(methods, method)

		info := ui.MethodInfo{
			Method:   method,
			Request:  reflectSchema(spec.Request),
			Response: reflectSchema(spec.Response),
			Params:   spec.Params,
		}

		methodInfos = append(methodInfos, info)
	}

	// Use explicit vars if provided, otherwise auto-extract from path
	vars := cfg.Vars
	if vars == nil {
		vars = extractPathVars(cfg.Path)
	}

	// Register route with router and mirror onto the route oracle so the
	// data wildcard defers to this Endpoint regardless of registration
	// order. Wrap the handler in AuditHandler so Server.Audit gates the
	// custom path the same way it gates the built-in REST handlers.
	// RouterMutate serializes this with the syncRouter wrapper's Match
	// so concurrent request dispatch is race-free.
	server.RouterMutate(func() {
		server.Router.HandleFunc(cfg.Path, server.AuditHandler(cfg.Handler)).Methods(methods...)
	})
	server.RegisterOracleRoute(cfg.Path, methods)

	// Track for UI. The append needs to serialize with getEndpoints
	// (the explorer UI hot path), so take the registry write lock.
	server.registryMu.Lock()
	server.endpoints = append(server.endpoints, ui.EndpointInfo{
		Path:        cfg.Path,
		Methods:     methodInfos,
		Description: cfg.Description,
		Vars:        vars,
	})
	server.registryMu.Unlock()
}

// RegisterProxy registers a proxy route for UI visibility. The append
// serializes with getProxies via the shared registry write lock.
func (server *Server) RegisterProxy(info ui.ProxyInfo) {
	server.registryMu.Lock()
	server.proxies = append(server.proxies, info)
	server.registryMu.Unlock()
}

// RegisterProxyCleanup registers a cleanup function to be called when the server closes.
// This is used by proxy routes to clean up their remote subscriptions.
func (server *Server) RegisterProxyCleanup(cleanup func()) {
	server.proxyCleanupMu.Lock()
	server.proxyCleanups = append(server.proxyCleanups, cleanup)
	server.proxyCleanupMu.Unlock()
}

// RegisterPreClose registers a cleanup function to be called at the very start of Close(),
// before stream and storage cleanup. This is useful for stopping background goroutines
// that depend on the stream being active. Multiple functions can be registered and will
// be called in registration order.
func (server *Server) RegisterPreClose(cleanup func()) {
	server.preCloseCleanupsMu.Lock()
	server.preCloseCleanups = append(server.preCloseCleanups, cleanup)
	server.preCloseCleanupsMu.Unlock()
}

// getEndpoints returns a snapshot of the registered endpoints. Takes
// the registry read lock and returns a copy of the slice header so
// the caller can iterate without contending with concurrent Endpoint
// registrations.
//
// The copy is shallow: an EndpointInfo's Methods slice and Vars map
// share backing memory with the registered entry. This is safe
// because already-registered EndpointInfo values are treated as
// immutable — Endpoint() only ever appends, nothing post-registration
// mutates an existing entry. Callers must preserve that invariant
// and not mutate the returned entries.
func (server *Server) getEndpoints() []ui.EndpointInfo {
	server.registryMu.RLock()
	defer server.registryMu.RUnlock()
	if len(server.endpoints) == 0 {
		return []ui.EndpointInfo{}
	}
	snapshot := make([]ui.EndpointInfo, len(server.endpoints))
	copy(snapshot, server.endpoints)
	return snapshot
}

// getProxies returns a snapshot of the registered proxies. Takes the
// registry read lock and returns a copy of the slice header so the
// caller can iterate without contending with concurrent RegisterProxy
// calls. ProxyInfo carries only scalars; the shallow snapshot is
// fully independent of the registry.
func (server *Server) getProxies() []ui.ProxyInfo {
	server.registryMu.RLock()
	defer server.registryMu.RUnlock()
	if len(server.proxies) == 0 {
		return []ui.ProxyInfo{}
	}
	snapshot := make([]ui.ProxyInfo, len(server.proxies))
	copy(snapshot, server.proxies)
	return snapshot
}

// getOrphanKeys returns keys that don't match any filter
// Filters out pivot-prefixed keys as they are internal use only
func (server *Server) getOrphanKeys() []string {
	allKeys, err := server.Storage.Keys()
	if err != nil {
		return []string{}
	}

	var orphans []string
	for _, k := range allKeys {
		// Skip pivot-prefixed keys (internal use only)
		if isPivotPath(k) {
			continue
		}
		hasFilter := server.filters.ReadObject.HasMatch(k) != -1 ||
			server.filters.ReadList.HasMatch(k) != -1
		if !hasFilter {
			orphans = append(orphans, k)
		}
	}

	if orphans == nil {
		return []string{}
	}
	return orphans
}
