package ui

import (
	"embed"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

//go:embed all:static
var staticFiles embed.FS

// ReservedPaths are paths used by the UI that cannot be used as filter paths.
// This list is exported so the ooo core can check for conflicts.
var ReservedPaths = []string{
	"vanilla-jsoneditor.js",
	"react.min.js",
	"react-dom.min.js",
	"babel.min.js",
	"styles.css",
	"ooo-client.js",
	"react-json-view.js",
	"api.js",
	"favicon.ico",
	"favicon.png",
	"favicon.svg",
	"logo.jpg",
	"logo.png",
	"components",
}

// ServerInfo contains server configuration exposed to the explorer
type ServerInfo struct {
	Name              string        `json:"name"`
	Address           string        `json:"address"`
	Deadline          time.Duration `json:"deadline"`
	ReadTimeout       time.Duration `json:"readTimeout"`
	WriteTimeout      time.Duration `json:"writeTimeout"`
	ReadHeaderTimeout time.Duration `json:"readHeaderTimeout"`
	IdleTimeout       time.Duration `json:"idleTimeout"`
	ForcePatch        bool          `json:"forcePatch"`
	NoPatch           bool          `json:"noPatch"`
	Static            bool          `json:"static"`
	Silence           bool          `json:"silence"`
	Workers           int           `json:"workers"`
	Tick              time.Duration `json:"tick"`
}

// FilterInfo contains detailed information about a filter path
type FilterInfo struct {
	Path      string `json:"path"`
	Type      string `json:"type"`
	CanRead   bool   `json:"canRead"`
	CanWrite  bool   `json:"canWrite"`
	CanDelete bool   `json:"canDelete"`
	IsGlob    bool   `json:"isGlob"`
	Limit     int    `json:"limit,omitempty"`
}

// FiltersInfo contains filter paths exposed to the explorer
type FiltersInfo struct {
	Paths   []string     `json:"paths"`
	Filters []FilterInfo `json:"filters"`
}

// PoolInfo contains information about a connection pool
type PoolInfo struct {
	Key         string `json:"key"`
	Connections int    `json:"connections"`
}

// StateInfo contains stream state information
type StateInfo struct {
	Pools            []PoolInfo `json:"pools"`
	TotalConnections int        `json:"totalConnections"`
}

// PivotNodeStatus represents the health status of a single node
type PivotNodeStatus struct {
	Address   string `json:"address"`
	Healthy   bool   `json:"healthy"`
	LastCheck string `json:"lastCheck"`
}

// PivotInfo contains pivot synchronization status
type PivotInfo struct {
	Role       string            `json:"role"`       // "pivot", "node", or "none"
	PivotIP    string            `json:"pivotIP"`    // Empty for pivot server, pivot address for nodes
	Nodes      []PivotNodeStatus `json:"nodes"`      // Node health status (only for pivot servers)
	SyncedKeys []string          `json:"syncedKeys"` // Keys being synchronized
}

// Handler serves the storage explorer SPA
type Handler struct {
	GetKeys        func() ([]string, error)
	GetInfo        func() ServerInfo
	GetFilters     func() []string
	GetFiltersInfo func() []FilterInfo
	GetState       func() []PoolInfo
	GetPivotInfo   func() *PivotInfo // Optional: returns nil if pivot not configured
	AuditFunc      func(r *http.Request) bool
	ClockFunc      func(w http.ResponseWriter, r *http.Request)
}

// ServeHTTP handles requests to the explorer
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Handle WebSocket upgrade for clock
	if r.Header.Get("Upgrade") == "websocket" {
		if h.ClockFunc != nil {
			h.ClockFunc(w, r)
		}
		return
	}

	// Check authorization
	if h.AuditFunc != nil && !h.AuditFunc(r) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("not authorized"))
		return
	}

	// API endpoints
	switch r.URL.Query().Get("api") {
	case "keys":
		h.handleKeys(w, r)
		return
	case "info":
		h.handleInfo(w, r)
		return
	case "filters":
		h.handleFilters(w, r)
		return
	case "state":
		h.handleState(w, r)
		return
	case "pivot":
		h.handlePivot(w, r)
		return
	}

	// Serve static files (use HasSuffix to handle sub-path mounting)
	path := r.URL.Path
	if strings.HasSuffix(path, "/vanilla-jsoneditor.js") {
		h.serveStatic(w, r, "vanilla-jsoneditor.js")
		return
	}
	if strings.HasSuffix(path, "/react.min.js") {
		h.serveStatic(w, r, "react.min.js")
		return
	}
	if strings.HasSuffix(path, "/react-dom.min.js") {
		h.serveStatic(w, r, "react-dom.min.js")
		return
	}
	if strings.HasSuffix(path, "/babel.min.js") {
		h.serveStatic(w, r, "babel.min.js")
		return
	}
	if strings.HasSuffix(path, "/styles.css") {
		h.serveStatic(w, r, "styles.css")
		return
	}
	if strings.HasSuffix(path, "/ooo-client.js") {
		h.serveStatic(w, r, "ooo-client.js")
		return
	}
	if strings.HasSuffix(path, "/react-json-view.js") {
		h.serveStatic(w, r, "react-json-view.js")
		return
	}
	if strings.HasSuffix(path, "/api.js") {
		h.serveStatic(w, r, "api.js")
		return
	}
	if strings.HasSuffix(path, "/favicon.ico") {
		h.serveStatic(w, r, "favicon.ico")
		return
	}
	if strings.HasSuffix(path, "/favicon.png") {
		h.serveStatic(w, r, "favicon.png")
		return
	}
	if strings.HasSuffix(path, "/logo.jpg") {
		h.serveStatic(w, r, "logo.jpg")
		return
	}
	if strings.HasSuffix(path, "/logo.png") {
		h.serveStatic(w, r, "logo.png")
		return
	}
	// Serve React JSX component files
	if idx := strings.Index(path, "/components/"); idx != -1 {
		componentPath := path[idx+1:] // Get "components/..."
		h.serveStatic(w, r, componentPath)
		return
	}

	// Serve the SPA
	h.serveIndex(w, r)
}

// KeysResponse contains paginated keys response
type KeysResponse struct {
	Keys   []string `json:"keys"`
	Total  int      `json:"total"`
	Page   int      `json:"page"`
	Limit  int      `json:"limit"`
	Filter string   `json:"filter"`
}

func (h *Handler) handleKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := h.GetKeys()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	// Parse query parameters
	filter := r.URL.Query().Get("filter")
	pageStr := r.URL.Query().Get("page")
	limitStr := r.URL.Query().Get("limit")

	page := 1
	limit := 50 // default page size

	if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
		page = p
	}
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 500 {
		limit = l
	}

	// Filter keys by glob pattern or search term
	var filteredKeys []string
	if filter != "" {
		if strings.Contains(filter, "*") {
			// Glob pattern matching
			prefix := strings.Split(filter, "*")[0]
			for _, k := range keys {
				if strings.HasPrefix(k, prefix) {
					filteredKeys = append(filteredKeys, k)
				}
			}
		} else {
			// Exact prefix match for static filters
			for _, k := range keys {
				if strings.HasPrefix(k, filter) {
					filteredKeys = append(filteredKeys, k)
				}
			}
		}
	} else {
		filteredKeys = keys
	}

	total := len(filteredKeys)

	// Paginate
	start := (page - 1) * limit
	end := start + limit
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	pagedKeys := filteredKeys[start:end]
	if pagedKeys == nil {
		pagedKeys = []string{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(KeysResponse{
		Keys:   pagedKeys,
		Total:  total,
		Page:   page,
		Limit:  limit,
		Filter: filter,
	})
}

func (h *Handler) handleInfo(w http.ResponseWriter, r *http.Request) {
	info := h.GetInfo()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func (h *Handler) handleFilters(w http.ResponseWriter, r *http.Request) {
	paths := h.GetFilters()
	var filtersInfo []FilterInfo
	if h.GetFiltersInfo != nil {
		filtersInfo = h.GetFiltersInfo()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(FiltersInfo{Paths: paths, Filters: filtersInfo})
}

func (h *Handler) handleState(w http.ResponseWriter, r *http.Request) {
	var pools []PoolInfo
	if h.GetState != nil {
		pools = h.GetState()
	}
	totalConnections := 0
	for _, p := range pools {
		totalConnections += p.Connections
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(StateInfo{Pools: pools, TotalConnections: totalConnections})
}

func (h *Handler) handlePivot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.GetPivotInfo == nil {
		// Pivot not configured - return "none" role
		json.NewEncoder(w).Encode(PivotInfo{Role: "none"})
		return
	}
	info := h.GetPivotInfo()
	if info == nil {
		json.NewEncoder(w).Encode(PivotInfo{Role: "none"})
		return
	}
	json.NewEncoder(w).Encode(info)
}

func (h *Handler) serveIndex(w http.ResponseWriter, r *http.Request) {
	content, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("failed to load explorer"))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(content)
}

func (h *Handler) serveStatic(w http.ResponseWriter, r *http.Request, filename string) {
	content, err := staticFiles.ReadFile("static/" + filename)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("file not found"))
		return
	}

	contentType := "application/octet-stream"
	if strings.HasSuffix(filename, ".js") || strings.HasSuffix(filename, ".jsx") {
		contentType = "application/javascript"
	} else if strings.HasSuffix(filename, ".css") {
		contentType = "text/css"
	} else if strings.HasSuffix(filename, ".svg") {
		contentType = "image/svg+xml"
	} else if strings.HasSuffix(filename, ".html") {
		contentType = "text/html; charset=utf-8"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=31536000")
	w.Write(content)
}
