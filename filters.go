package ooo

import (
	"fmt"
	"strings"

	"github.com/benitogf/ooo/filters"
	"github.com/benitogf/ooo/key"
	"github.com/benitogf/ooo/ui"
)

// LimitFilterConfig is an alias for filters.LimitFilterConfig for convenience.
// Use this with LimitFilter to configure limit and sort order.
type LimitFilterConfig = filters.LimitFilterConfig

// LimitFunc is an alias for filters.LimitFunc for convenience.
// Use this to provide a dynamic limit function that is called each time the limit is needed.
type LimitFunc = filters.LimitFunc

// Order constants for LimitFilterConfig
const (
	OrderDesc = filters.OrderDesc // Most recent first (default)
	OrderAsc  = filters.OrderAsc  // Oldest first
)

// checkReservedPath panics if the path conflicts with reserved UI paths
func checkReservedPath(path string) {
	// Extract the first segment of the path
	firstSegment := path
	if idx := strings.Index(path, "/"); idx != -1 {
		firstSegment = path[:idx]
	}

	for _, reserved := range ui.ReservedPaths {
		if firstSegment == reserved || path == reserved {
			panic(fmt.Sprintf("%v: %q conflicts with reserved path %q", ErrReservedPath, path, reserved))
		}
	}
}

// Re-export filter types from filters package
type (
	Apply       = filters.Apply
	ApplyObject = filters.ApplyObject
	ApplyList   = filters.ApplyList
	Block       = filters.Block
	Notify      = filters.Notify
)

// Re-export filter functions from filters package
var (
	NoopHook         = filters.NoopHook
	NoopNotify       = filters.NoopNotify
	NoopFilter       = filters.NoopFilter
	NoopObjectFilter = filters.NoopObjectFilter
	NoopListFilter   = filters.NoopListFilter
)

// DeleteFilter add a filter that runs before delete
func (server *Server) DeleteFilter(path string, apply Block) {
	checkReservedPath(path)
	server.filters.AddDelete(path, apply)
}

// WriteFilter add a filter that triggers on write
func (server *Server) WriteFilter(path string, apply Apply) {
	checkReservedPath(path)
	server.filters.AddWrite(path, apply)
}

// AfterWriteFilter add a filter that triggers after a successful write
func (server *Server) AfterWriteFilter(path string, apply Notify) {
	checkReservedPath(path)
	server.filters.AddAfterWrite(path, apply)
}

// ReadObjectFilter add a filter for single meta.Object reads
func (server *Server) ReadObjectFilter(path string, apply ApplyObject) {
	checkReservedPath(path)
	server.filters.AddReadObject(path, apply)
}

// ReadListFilter add a filter for []meta.Object reads.
// For glob paths like "things/*", individual item reads (e.g., "things/123") will also
// be allowed if no explicit ReadObjectFilter is registered for that path.
func (server *Server) ReadListFilter(path string, apply ApplyList) {
	checkReservedPath(path)
	server.filters.AddReadList(path, apply)
}

// OpenFilter open noop read and write filters
// For glob paths like "things/*", this also enables reading individual items like "things/123"
func (server *Server) OpenFilter(name string) {
	checkReservedPath(name)
	server.filters.AddWrite(name, NoopFilter)
	server.filters.AddDelete(name, NoopHook)
	if key.IsGlob(name) {
		server.filters.AddReadList(name, NoopListFilter)
		// Also allow reading individual items that match the glob pattern
		server.filters.AddReadObject(name, NoopObjectFilter)
	} else {
		server.filters.AddReadObject(name, NoopObjectFilter)
	}
}

// LimitFilter creates a limit filter for a glob pattern path that maintains
// a maximum number of entries. Uses a ReadListFilter (meta-based) to limit the view
// (so clients never see more than limit items) and AfterWrite to delete old entries.
// Also adds write and delete filters to allow creating and deleting items.
func (server *Server) LimitFilter(path string, cfg filters.LimitFilterConfig) {
	checkReservedPath(path)
	lf, err := filters.NewLimitFilter(path, cfg, server.Storage)
	if err != nil {
		panic(err)
	}
	server.registerLimitFilter(path, lf)
}

func (server *Server) registerLimitFilter(path string, lf *filters.LimitFilter) {
	// Allow writes and deletes
	server.filters.AddWrite(path, NoopFilter)
	server.filters.AddDelete(path, NoopHook)

	// ReadListFilter ensures clients never see more than limit items (meta-based, more efficient)
	server.filters.AddReadList(path, lf.ReadListFilter)

	// Also allow reading individual items that match the glob pattern
	server.filters.AddReadObject(path, NoopObjectFilter)

	// AfterWrite triggers cleanup of old entries
	server.filters.AddAfterWrite(path, func(k string) {
		lf.Check()
	})

	// Register for explorer display
	server.RegisterLimitFilter(lf)
}
