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
func (app *Server) DeleteFilter(path string, apply Block) {
	app.filters.AddDelete(path, apply)
}

// WriteFilter add a filter that triggers on write
func (app *Server) WriteFilter(path string, apply Apply) {
	app.filters.AddWrite(path, apply)
}

// AfterWriteFilter add a filter that triggers after a successful write
func (app *Server) AfterWriteFilter(path string, apply Notify) {
	app.filters.AddAfterWrite(path, apply)
}

// ReadObjectFilter add a filter for single meta.Object reads
func (app *Server) ReadObjectFilter(path string, apply ApplyObject) {
	app.filters.AddReadObject(path, apply)
}

// ReadListFilter add a filter for []meta.Object reads
func (app *Server) ReadListFilter(path string, apply ApplyList) {
	app.filters.AddReadList(path, apply)
}

// OpenFilter open noop read and write filters
// For glob paths like "things/*", this also enables reading individual items like "things/123"
func (app *Server) OpenFilter(name string) {
	app.filters.AddWrite(name, NoopFilter)
	app.filters.AddDelete(name, NoopHook)
	if key.IsGlob(name) {
		app.filters.AddReadList(name, NoopListFilter)
		// Also allow reading individual items that match the glob pattern
		app.filters.AddReadObject(name, NoopObjectFilter)
	} else {
		app.filters.AddReadObject(name, NoopObjectFilter)
	}
}

// LimitFilter creates a limit filter for a glob pattern path that maintains
// a maximum number of entries. Uses a ReadListFilter (meta-based) to limit the view
// (so clients never see more than limit items) and AfterWrite to delete old entries.
// Also adds write and delete filters to allow creating and deleting items.
func (app *Server) LimitFilter(path string, limit int) {
	lf, err := filters.NewLimitFilter(path, limit, app.Storage)
	if err != nil {
		panic(err)
	}

	// Allow writes and deletes
	app.filters.AddWrite(path, NoopFilter)
	app.filters.AddDelete(path, NoopHook)

	// ReadListFilter ensures clients never see more than limit items (meta-based, more efficient)
	app.filters.AddReadList(path, lf.ReadListFilter)

	// Also allow reading individual items that match the glob pattern
	app.filters.AddReadObject(path, NoopObjectFilter)

	// AfterWrite triggers cleanup of old entries
	app.filters.AddAfterWrite(path, func(k string) {
		lf.Check()
	})
}
