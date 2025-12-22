package filters

import (
	"errors"
	"log"
	"sort"
	"sync"

	"github.com/benitogf/ooo/key"
	"github.com/benitogf/ooo/meta"
	"github.com/goccy/go-json"
)

var (
	ErrLimitMustBePositive = errors.New("filters: limit must be positive")
	ErrPathMustBeGlob      = errors.New("filters: path must be a glob pattern")
)

// Database interface for limit filter operations
type Database interface {
	GetList(path string) ([]meta.Object, error)
	Del(path string) error
	DelSilent(path string) error
}

// LimitFilter maintains a maximum number of entries for a glob pattern path.
// Uses a ReadFilter to limit the view and AfterWrite to trigger cleanup.
type LimitFilter struct {
	path  string
	limit int
	db    Database
	mu    sync.Mutex
}

// NewLimitFilter creates a new LimitFilter for the given glob pattern path.
// The limit must be positive and the path must be a glob pattern (ending with *).
func NewLimitFilter(path string, limit int, db Database) (*LimitFilter, error) {
	if limit <= 0 {
		return nil, ErrLimitMustBePositive
	}
	if !key.IsGlob(path) {
		return nil, ErrPathMustBeGlob
	}
	return &LimitFilter{
		path:  path,
		limit: limit,
		db:    db,
	}, nil
}

// Path returns the glob pattern path this filter applies to.
func (lf *LimitFilter) Path() string {
	return lf.path
}

// Limit returns the maximum number of entries allowed.
func (lf *LimitFilter) Limit() int {
	return lf.limit
}

// ReadFilter returns a filter function that limits the data to the most recent entries.
// This ensures clients never see more than the limit, even if storage has more.
// Legacy JSON-based version - use ReadListFilter for better performance.
func (lf *LimitFilter) ReadFilter(path string, data json.RawMessage) (json.RawMessage, error) {
	// Decode the list of objects
	var objects []meta.Object
	if err := json.Unmarshal(data, &objects); err != nil {
		// Not a list, return as-is
		return data, nil
	}

	// If under limit, return as-is
	if len(objects) <= lf.limit {
		return data, nil
	}

	// Sort by created timestamp descending to get most recent
	sort.Slice(objects, func(i, j int) bool {
		return objects[i].Created > objects[j].Created
	})

	// Take only the most recent 'limit' entries
	limited := objects[:lf.limit]

	// Re-encode
	return json.Marshal(limited)
}

// ReadListFilter returns a meta-based filter function that limits the list to the most recent entries.
// This is more efficient than ReadFilter as it avoids JSON encoding/decoding.
func (lf *LimitFilter) ReadListFilter(path string, objs []meta.Object) ([]meta.Object, error) {
	// Always sort by created timestamp descending for consistent ordering
	// This ensures bytes.Equal optimization works correctly
	sort.Slice(objs, func(i, j int) bool {
		return objs[i].Created > objs[j].Created
	})

	// If under limit, return all (now sorted)
	if len(objs) <= lf.limit {
		return objs, nil
	}

	// Take only the most recent 'limit' entries
	return objs[:lf.limit], nil
}

// Check deletes the oldest entries if over limit.
// This should be called via AfterWrite to clean up old entries.
// Uses DelSilent to avoid broadcasting since the broadcast is handled
// by the add operation's FilterList which already removes the item from the view.
func (lf *LimitFilter) Check() {
	lf.mu.Lock()
	defer lf.mu.Unlock()

	entries, err := lf.db.GetList(lf.path)
	if err != nil {
		log.Println("LimitFilter["+lf.path+"]: failed to get list", err)
		return
	}

	// If we're at or under the limit, no deletion needed
	if len(entries) <= lf.limit {
		return
	}

	// Sort by created timestamp (ascending) to find the oldest
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Created < entries[j].Created
	})

	// Delete oldest entries to enforce limit (silent - no broadcast)
	toDelete := len(entries) - lf.limit
	for i := 0; i < toDelete; i++ {
		err = lf.db.DelSilent(entries[i].Path)
		if err != nil {
			log.Println("LimitFilter["+lf.path+"]: failed to delete entry", entries[i].Path, err)
			return
		}
	}
}
