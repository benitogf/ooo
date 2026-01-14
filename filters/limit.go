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

// Order defines the sort direction for limit filter results
type Order int

const (
	OrderDesc Order = iota // Most recent first (default)
	OrderAsc               // Oldest first
)

// LimitFunc is a function that returns the current limit dynamically.
// If provided in LimitFilterConfig, it takes precedence over the static Limit field.
type LimitFunc func() int

// LimitFilterConfig contains configuration for a LimitFilter
type LimitFilterConfig struct {
	Limit       int       // Maximum number of entries (used if LimitFunc is nil)
	LimitFunc   LimitFunc // Dynamic limit function (takes precedence over Limit if set)
	Order       Order     // Sort order for results (default: OrderDesc)
	Description string    // Description for UI display
	Schema      any       // Go type for the data schema (used for UI display)
}

// Database interface for limit filter operations
type Database interface {
	GetList(path string) ([]meta.Object, error)
	Del(path string) error
	DelSilent(path string) error
}

// LimitFilter maintains a maximum number of entries for a glob pattern path.
// Uses a ReadFilter to limit the view and AfterWrite to trigger cleanup.
type LimitFilter struct {
	path      string
	limit     int
	limitFunc LimitFunc
	order     Order
	db        Database
	mu        sync.Mutex
}

// NewLimitFilter creates a new LimitFilter for the given glob pattern path.
// Either Limit must be positive or LimitFunc must be provided.
// The path must be a glob pattern (ending with *).
func NewLimitFilter(path string, cfg LimitFilterConfig, db Database) (*LimitFilter, error) {
	if cfg.LimitFunc == nil && cfg.Limit <= 0 {
		return nil, ErrLimitMustBePositive
	}
	if !key.IsGlob(path) {
		return nil, ErrPathMustBeGlob
	}
	return &LimitFilter{
		path:      path,
		limit:     cfg.Limit,
		limitFunc: cfg.LimitFunc,
		order:     cfg.Order,
		db:        db,
	}, nil
}

// Path returns the glob pattern path this filter applies to.
func (lf *LimitFilter) Path() string {
	return lf.path
}

// Limit returns the current maximum number of entries allowed.
// If LimitFunc was provided, it calls the function to get the dynamic limit.
// Otherwise, it returns the static Limit value.
func (lf *LimitFilter) Limit() int {
	if lf.limitFunc != nil {
		return lf.limitFunc()
	}
	return lf.limit
}

// Order returns the sort order for results.
func (lf *LimitFilter) Order() Order {
	return lf.order
}

// IsDynamic returns true if this filter uses a dynamic limit function.
func (lf *LimitFilter) IsDynamic() bool {
	return lf.limitFunc != nil
}

// ReadFilter returns a filter function that limits the data to the configured entries.
// This ensures clients never see more than the limit, even if storage has more.
// Legacy JSON-based version - use ReadListFilter for better performance.
func (lf *LimitFilter) ReadFilter(path string, data json.RawMessage) (json.RawMessage, error) {
	// Decode the list of objects
	var objects []meta.Object
	err := json.Unmarshal(data, &objects)
	if err != nil {
		// Not a list, return as-is
		return data, nil
	}

	limit := lf.Limit()

	// If under limit, return as-is
	if len(objects) <= limit {
		return data, nil
	}

	// Sort by created timestamp based on configured order
	if lf.order == OrderAsc {
		sort.Slice(objects, func(i, j int) bool {
			return objects[i].Created < objects[j].Created
		})
	} else {
		sort.Slice(objects, func(i, j int) bool {
			return objects[i].Created > objects[j].Created
		})
	}

	// Take only the 'limit' entries
	limited := objects[:limit]

	// Re-encode
	return json.Marshal(limited)
}

// ReadListFilter returns a meta-based filter function that limits the list to the configured entries.
// This is more efficient than ReadFilter as it avoids JSON encoding/decoding.
// Always keeps the most recent items, then sorts for display based on Order setting.
func (lf *LimitFilter) ReadListFilter(path string, objs []meta.Object) ([]meta.Object, error) {
	limit := lf.Limit()

	// If under limit, just sort for display and return
	if len(objs) <= limit {
		if lf.order == OrderAsc {
			sort.Slice(objs, func(i, j int) bool {
				return objs[i].Created < objs[j].Created
			})
		} else {
			sort.Slice(objs, func(i, j int) bool {
				return objs[i].Created > objs[j].Created
			})
		}
		return objs, nil
	}

	// First, sort by newest first to keep the most recent items
	sort.Slice(objs, func(i, j int) bool {
		return objs[i].Created > objs[j].Created
	})

	// Take the most recent 'limit' entries
	objs = objs[:limit]

	// Re-sort for display based on configured order
	if lf.order == OrderAsc {
		sort.Slice(objs, func(i, j int) bool {
			return objs[i].Created < objs[j].Created
		})
	}
	// OrderDesc is already sorted correctly (newest first)

	return objs, nil
}

// Check deletes the oldest entries if over limit.
// This should be called via AfterWrite to clean up old entries.
// Uses DelSilent to avoid broadcasting since the broadcast is handled
// by the add operation's FilterList which already removes the item from the view.
func (lf *LimitFilter) Check() {
	lf.mu.Lock()
	defer lf.mu.Unlock()

	limit := lf.Limit()

	entries, err := lf.db.GetList(lf.path)
	if err != nil {
		log.Println("LimitFilter["+lf.path+"]: failed to get list", err)
		return
	}

	// If we're at or under the limit, no deletion needed
	if len(entries) <= limit {
		return
	}

	// Sort by created timestamp (ascending) to find the oldest
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Created < entries[j].Created
	})

	// Delete oldest entries to enforce limit (silent - no broadcast)
	toDelete := len(entries) - limit
	for i := 0; i < toDelete; i++ {
		err = lf.db.DelSilent(entries[i].Path)
		if err != nil {
			log.Println("LimitFilter["+lf.path+"]: failed to delete entry", entries[i].Path, err)
			return
		}
	}
}
