package ooo

import (
	"net/http"
	"regexp"

	"github.com/benitogf/ooo/schema"
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

// Endpoint registers a custom HTTP endpoint with metadata for UI visibility
func (server *Server) Endpoint(cfg EndpointConfig) {
	methods := make([]string, 0, len(cfg.Methods))
	methodInfos := make([]ui.MethodInfo, 0, len(cfg.Methods))

	for method, spec := range cfg.Methods {
		methods = append(methods, method)

		info := ui.MethodInfo{
			Method:   method,
			Request:  schema.Reflect(spec.Request),
			Response: schema.Reflect(spec.Response),
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
	// order. Any middleware registered via server.Router.Use(...) is
	// applied by gorilla/mux's Match before dispatch — no per-handler
	// wrapper is needed here. RouterMutate serializes this with the
	// syncRouter wrapper's Match so concurrent request dispatch is
	// race-free.
	server.RouterMutate(func() {
		server.Router.HandleFunc(cfg.Path, cfg.Handler).Methods(methods...)
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

// RegisterCloseHook registers fn to run during Server.Close at the
// given teardown phase. Multiple hooks at the same phase run in
// registration order. Phases run in CloseHookPhase declaration order
// (PreShutdown → ProxyTeardown → PostShutdown). The aggregate
// callback runtime is bounded by Server.CloseCallbackBudget when set.
// Panics if phase is out of range.
func (server *Server) RegisterCloseHook(phase CloseHookPhase, fn func()) {
	if phase < PreShutdown || phase >= closeHookPhaseCount {
		panic("ooo: RegisterCloseHook called with invalid CloseHookPhase")
	}
	server.closeHooksMu.Lock()
	server.closeHooks[phase] = append(server.closeHooks[phase], fn)
	server.closeHooksMu.Unlock()
}

// RegisterProxyCleanup registers a cleanup function to be called when the server closes.
// This is used by proxy routes to clean up their remote subscriptions.
//
// Deprecated: use RegisterCloseHook(ProxyTeardown, cleanup) instead.
func (server *Server) RegisterProxyCleanup(cleanup func()) {
	server.RegisterCloseHook(ProxyTeardown, cleanup)
}

// RegisterPreClose registers a cleanup function to be called at the very start of Close(),
// before stream and storage cleanup. This is useful for stopping background goroutines
// that depend on the stream being active. Multiple functions can be registered and will
// be called in registration order.
//
// Deprecated: use RegisterCloseHook(PreShutdown, cleanup) instead.
func (server *Server) RegisterPreClose(cleanup func()) {
	server.RegisterCloseHook(PreShutdown, cleanup)
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
