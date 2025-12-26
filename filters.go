package ooo

import (
	"github.com/benitogf/ooo/filters"
	"github.com/benitogf/ooo/key"
)

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
	NoopFilter       = filters.NoopFilter
	NoopObjectFilter = filters.NoopObjectFilter
	NoopListFilter   = filters.NoopListFilter
)

// DeleteFilter add a filter that runs before delete
func (server *Server) DeleteFilter(path string, apply Block) {
	server.filters.AddDelete(path, apply)
}

// WriteFilter add a filter that triggers on write
func (server *Server) WriteFilter(path string, apply Apply) {
	server.filters.AddWrite(path, apply)
}

// AfterWriteFilter add a filter that triggers after a successful write
func (server *Server) AfterWriteFilter(path string, apply Notify) {
	server.filters.AddAfterWrite(path, apply)
}

// ReadObjectFilter add a filter for single meta.Object reads
func (server *Server) ReadObjectFilter(path string, apply ApplyObject) {
	server.filters.AddReadObject(path, apply)
}

// ReadListFilter add a filter for []meta.Object reads
func (server *Server) ReadListFilter(path string, apply ApplyList) {
	server.filters.AddReadList(path, apply)
}

// OpenFilter open noop read and write filters
// For glob paths like "things/*", this also enables reading individual items like "things/123"
func (server *Server) OpenFilter(name string) {
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
func (server *Server) LimitFilter(path string, limit int) {
	lf, err := filters.NewLimitFilter(path, limit, server.Storage)
	if err != nil {
		panic(err)
	}

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
}
