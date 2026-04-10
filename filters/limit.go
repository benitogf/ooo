package filters

import (
	"errors"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/benitogf/ooo/key"
	"github.com/benitogf/ooo/meta"
	"github.com/benitogf/ooo/monotonic"
	"github.com/benitogf/go-json"
)

var (
	ErrLimitMustBePositive   = errors.New("filters: limit must be positive")
	ErrPathMustBeGlob        = errors.New("filters: path must be a glob pattern")
	ErrNoConstraint          = errors.New("filters: at least one of Limit/LimitFunc or MaxAge/MaxAgeFunc must be provided")
	ErrMaxAgeMustBePositive  = errors.New("filters: max age must be positive")
	ErrCleanupIntervalTooLow = errors.New("filters: cleanup interval must be at least 1 minute")
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

// MaxAgeFunc returns the current max age dynamically.
// If provided in LimitFilterConfig, it takes precedence over the static MaxAge field.
type MaxAgeFunc func() time.Duration

// CleanupConfig controls optional periodic background cleanup.
type CleanupConfig struct {
	Enabled  bool          // Whether to run periodic cleanup
	Interval time.Duration // How often to run (default: 10min, minimum: 1min)
}

// LimitFilterConfig contains configuration for a LimitFilter.
// At least one of Limit/LimitFunc or MaxAge/MaxAgeFunc must be provided.
type LimitFilterConfig struct {
	Limit       int           // Maximum number of entries (used if LimitFunc is nil)
	LimitFunc   LimitFunc     // Dynamic count limit (takes precedence over Limit if set)
	MaxAge      time.Duration // Maximum age of entries (used if MaxAgeFunc is nil)
	MaxAgeFunc  MaxAgeFunc    // Dynamic max age (takes precedence over MaxAge if set)
	Order       Order         // Sort order for results (default: OrderDesc)
	Cleanup     CleanupConfig // Periodic background cleanup (default: disabled)
	Description string        // Description for UI display
	Schema      any           // Go type for the data schema (used for UI display)
}

// Database interface for limit filter operations
type Database interface {
	GetList(path string) ([]meta.Object, error)
	Del(path string) error
	DelSilent(path string) error
}

// LimitFilter maintains constraints (count and/or time) for a glob pattern path.
// Uses a ReadFilter to limit the view and AfterWrite to trigger cleanup.
type LimitFilter struct {
	path       string
	limit      int
	limitFunc  LimitFunc
	maxAge     time.Duration
	maxAgeFunc MaxAgeFunc
	order      Order
	cleanup    CleanupConfig
	db         Database
	now        func() int64 // injectable clock, defaults to monotonic.Now
	mu         sync.Mutex
	stopCh     chan struct{} // nil if periodic cleanup not enabled
}

// NewLimitFilter creates a new LimitFilter for the given glob pattern path.
// At least one of Limit/LimitFunc or MaxAge/MaxAgeFunc must be provided.
// The path must be a glob pattern (ending with *).
func NewLimitFilter(path string, cfg LimitFilterConfig, db Database) (*LimitFilter, error) {
	// Validate explicitly-invalid values first (before no-constraint check)
	if cfg.LimitFunc == nil && cfg.Limit < 0 {
		return nil, ErrLimitMustBePositive
	}
	if cfg.MaxAgeFunc == nil && cfg.MaxAge < 0 {
		return nil, ErrMaxAgeMustBePositive
	}

	hasCount := cfg.LimitFunc != nil || cfg.Limit > 0
	hasTime := cfg.MaxAgeFunc != nil || cfg.MaxAge > 0

	if !hasCount && !hasTime {
		// Backwards compat: existing callers with Limit <= 0 get the original error
		return nil, ErrLimitMustBePositive
	}

	// Validate cleanup interval
	if cfg.Cleanup.Enabled {
		if cfg.Cleanup.Interval == 0 {
			cfg.Cleanup.Interval = 10 * time.Minute
		} else if cfg.Cleanup.Interval < time.Minute {
			return nil, ErrCleanupIntervalTooLow
		}
	}

	if !key.IsGlob(path) {
		return nil, ErrPathMustBeGlob
	}

	return &LimitFilter{
		path:       path,
		limit:      cfg.Limit,
		limitFunc:  cfg.LimitFunc,
		maxAge:     cfg.MaxAge,
		maxAgeFunc: cfg.MaxAgeFunc,
		order:      cfg.Order,
		cleanup:    cfg.Cleanup,
		db:         db,
		now:        monotonic.Now,
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

// IsDynamic returns true if this filter uses a dynamic limit or max age function.
func (lf *LimitFilter) IsDynamic() bool {
	return lf.limitFunc != nil || lf.maxAgeFunc != nil
}

// MaxAge returns the current maximum age. Returns 0 if no time constraint.
func (lf *LimitFilter) MaxAge() time.Duration {
	if lf.maxAgeFunc != nil {
		return lf.maxAgeFunc()
	}
	return lf.maxAge
}

// HasMaxAge returns true if a time constraint is configured.
func (lf *LimitFilter) HasMaxAge() bool {
	return lf.maxAgeFunc != nil || lf.maxAge > 0
}

// HasLimit returns true if a count constraint is configured.
func (lf *LimitFilter) HasLimit() bool {
	return lf.limitFunc != nil || lf.limit > 0
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

	// Apply constraints via ReadListFilter
	filtered, err := lf.ReadListFilter(path, objects)
	if err != nil {
		return nil, err
	}

	// Re-encode
	return json.Marshal(filtered)
}

// ReadListFilter returns a meta-based filter function that limits the list to the configured entries.
// This is more efficient than ReadFilter as it avoids JSON encoding/decoding.
// Applies time constraint first (filter by age), then count constraint (keep newest N),
// then sorts for display based on Order setting.
func (lf *LimitFilter) ReadListFilter(path string, objs []meta.Object) ([]meta.Object, error) {
	// Step 1: Time constraint — filter by age (cheap comparison, no sort needed)
	if lf.HasMaxAge() {
		cutoff := lf.now() - lf.MaxAge().Nanoseconds()
		n := 0
		for _, obj := range objs {
			if obj.Created >= cutoff {
				objs[n] = obj
				n++
			}
		}
		objs = objs[:n]
	}

	// Step 2: Count constraint — keep newest N
	if lf.HasLimit() {
		limit := lf.Limit()
		if len(objs) > limit {
			// Sort newest first, then truncate
			sort.Slice(objs, func(i, j int) bool {
				return objs[i].Created > objs[j].Created
			})
			objs = objs[:limit]
		}
	}

	// Step 3: Sort for display
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

// Check deletes entries that exceed the configured constraints (time and/or count).
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

	if len(entries) == 0 {
		return
	}

	// Sort by created timestamp (ascending) to find the oldest
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Created < entries[j].Created
	})

	toDelete := make(map[string]struct{})

	// Time constraint: mark entries older than MaxAge
	if lf.HasMaxAge() {
		cutoff := lf.now() - lf.MaxAge().Nanoseconds()
		for _, entry := range entries {
			if entry.Created < cutoff {
				toDelete[entry.Path] = struct{}{}
			}
		}
	}

	// Count constraint: after time removals, check remaining count
	if lf.HasLimit() {
		limit := lf.Limit()
		remaining := len(entries) - len(toDelete)
		if remaining > limit {
			excess := remaining - limit
			for _, entry := range entries {
				if excess == 0 {
					break
				}
				if _, alreadyMarked := toDelete[entry.Path]; !alreadyMarked {
					toDelete[entry.Path] = struct{}{}
					excess--
				}
			}
		}
	}

	if len(toDelete) == 0 {
		return
	}

	// Delete in sorted order (oldest first) using silent delete
	for _, entry := range entries {
		if _, ok := toDelete[entry.Path]; ok {
			if err := lf.db.DelSilent(entry.Path); err != nil {
				log.Println("LimitFilter["+lf.path+"]: failed to delete", entry.Path, err)
				return
			}
		}
	}
}

// SetNow overrides the clock function used for time comparisons.
// This is intended for testing only, to enable deterministic time-based tests.
func (lf *LimitFilter) SetNow(fn func() int64) {
	lf.now = fn
}

// StartCleanup starts the periodic background cleanup goroutine if configured.
func (lf *LimitFilter) StartCleanup() {
	if !lf.cleanup.Enabled || lf.cleanup.Interval <= 0 {
		return
	}
	lf.stopCh = make(chan struct{})
	go lf.cleanupLoop()
}

// StopCleanup stops the periodic background cleanup goroutine.
func (lf *LimitFilter) StopCleanup() {
	if lf.stopCh != nil {
		close(lf.stopCh)
	}
}

func (lf *LimitFilter) cleanupLoop() {
	ticker := time.NewTicker(lf.cleanup.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-lf.stopCh:
			return
		case <-ticker.C:
			lf.Check()
		}
	}
}
