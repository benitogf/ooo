package storage

import (
	"sort"
	"sync"

	"github.com/benitogf/go-json"

	"github.com/benitogf/ooo/key"
	"github.com/benitogf/ooo/meta"
	"github.com/benitogf/ooo/monotonic"
)

// LayeredConfig configuration for the layered storage
type LayeredConfig struct {
	// Memory layer (fastest) - optional
	Memory *MemoryLayer
	// MemoryOptions configuration for memory layer
	MemoryOptions LayerOptions

	// Embedded layer (medium speed) - optional
	Embedded EmbeddedLayer
	// EmbeddedOptions configuration for embedded layer
	EmbeddedOptions LayerOptions
}

// Layered is a multi-layer storage that coordinates between memory and embedded layers
type Layered struct {
	memory   *MemoryLayer
	embedded EmbeddedLayer

	memoryOpt   LayerOptions
	embeddedOpt LayerOptions

	mutex           sync.RWMutex
	memMutex        map[string]*refMu // refcounted per-key locks
	memMutexMu      sync.Mutex        // serializes ref++/ref-- + delete sequences on memMutex
	writeMutex      sync.RWMutex      // serializes glob deletes against individual writes
	noBroadcastKeys []string
	watcher         *ShardedChan
	active          bool
	beforeRead      func(key string)
	afterWrite      func(key string)
}

// NewLayered creates a new layered storage
func New(cfg LayeredConfig) *Layered {
	return &Layered{
		memory:      cfg.Memory,
		embedded:    cfg.Embedded,
		memoryOpt:   cfg.MemoryOptions,
		embeddedOpt: cfg.EmbeddedOptions,
	}
}

// Active returns whether the storage is active
func (l *Layered) Active() bool {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.active
}

// SetBeforeRead updates the BeforeRead callback without restarting the storage.
// This is safe to call on already-active storage, including embedded/leveldb storage.
func (l *Layered) SetBeforeRead(fn func(key string)) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.beforeRead = fn
}

// getBeforeRead returns the current BeforeRead callback in a thread-safe manner
func (l *Layered) getBeforeRead() func(key string) {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.beforeRead
}

// Start initializes all layers and populates caches
func (l *Layered) Start(opt Options) error {
	if l.memory == nil && l.embedded == nil {
		return ErrAllLayersNil
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()

	// Create sharded watcher
	workers := opt.Workers
	if workers <= 0 {
		workers = 6
	}
	l.watcher = NewShardedChan(workers)
	l.noBroadcastKeys = opt.NoBroadcastKeys
	l.beforeRead = opt.BeforeRead
	l.afterWrite = opt.AfterWrite

	// Start layers from slowest to fastest
	if l.embedded != nil {
		if err := l.embedded.Start(l.embeddedOpt); err != nil {
			return err
		}
	}

	if l.memory != nil {
		if err := l.memory.Start(l.memoryOpt); err != nil {
			if l.embedded != nil {
				l.embedded.Close()
			}
			return err
		}
	}

	// Initialize caches from slowest layer to fastest
	if err := l.initializeCaches(); err != nil {
		l.closeAllLayers()
		return err
	}

	l.active = true
	return nil
}

// initializeCaches populates faster layers from slower layers
func (l *Layered) initializeCaches() error {
	if l.embedded != nil && l.memory != nil {
		data, err := l.embedded.Load()
		if err != nil {
			return err
		}
		for k, obj := range data {
			_ = l.memory.Set(k, obj)
		}
	}

	return nil
}

// closeAllLayers closes all active layers
func (l *Layered) closeAllLayers() {
	if l.memory != nil {
		l.memory.Close()
	}
	if l.embedded != nil {
		l.embedded.Close()
	}
}

// Close shuts down all layers
func (l *Layered) Close() {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.active = false
	l.closeAllLayers()
	if l.watcher != nil {
		l.watcher.Close()
		l.watcher = nil
	}
}

// Get retrieves a single value by exact key
// Checks layers from fastest to slowest, populates faster layers on cache miss
func (l *Layered) Get(path string) (meta.Object, error) {
	if br := l.getBeforeRead(); br != nil {
		br(path)
	}
	if key.HasGlob(path) {
		return meta.Object{}, ErrGlobNotAllowed
	}

	// Memory is a complete mirror of embedded (seeded at Start, kept in sync on every write).
	if l.memory != nil {
		return l.memory.Get(path)
	}
	if l.embedded != nil {
		return l.embedded.Get(path)
	}

	return meta.Object{}, ErrNotFound
}

// refMu pairs a per-key mutex with a refcount so the registry can drop
// the entry once no caller is holding or waiting on it. Pre-fix the
// registry was an unbounded sync.Map — Push generates a fresh path per
// call (key.Build), so a queue-shaped workload leaked one entry per
// write until restart. Counting waiters + holders and deleting at zero
// caps steady-state memory.
type refMu struct {
	mu  sync.Mutex
	ref int // protected by Layered.memMutexMu
}

// _acquireLock returns the per-path mutex with its refcount incremented
// and its mu already locked. Callers MUST pair this with _releaseLock
// to bring the refcount back down. The ref is bumped under memMutexMu
// before mu.Lock so a concurrent _releaseLock cannot delete the entry
// out from under us while we wait on the lock.
func (l *Layered) _acquireLock(path string) *refMu {
	l.memMutexMu.Lock()
	if l.memMutex == nil {
		l.memMutex = make(map[string]*refMu)
	}
	rm, ok := l.memMutex[path]
	if !ok {
		rm = &refMu{}
		l.memMutex[path] = rm
	}
	rm.ref++
	l.memMutexMu.Unlock()
	rm.mu.Lock()
	return rm
}

// _releaseLock unlocks rm.mu and decrements the refcount; when no
// caller is holding or waiting on the entry, it is removed from the
// registry. Always pairs with exactly one prior ref bump — either from
// _acquireLock on the same path, or from a GetAndLock whose
// corresponding SetAndUnlock/Unlock uses _loadLock to find the entry.
// _loadLock does NOT bump ref, so calling _loadLock followed by
// _releaseLock without a matching prior _acquireLock would underflow
// the refcount and leak the entry — re-introducing this fix's leak.
func (l *Layered) _releaseLock(path string, rm *refMu) {
	rm.mu.Unlock()
	l.memMutexMu.Lock()
	rm.ref--
	if rm.ref == 0 {
		delete(l.memMutex, path)
	}
	l.memMutexMu.Unlock()
}

// _loadLock looks up the existing per-path mutex without changing its
// refcount. Returns ErrLockNotFound if no caller is currently holding
// or waiting on the path. Used by SetAndUnlock / Unlock to find the
// lock established by a prior GetAndLock.
func (l *Layered) _loadLock(path string) (*refMu, error) {
	l.memMutexMu.Lock()
	defer l.memMutexMu.Unlock()
	rm, ok := l.memMutex[path]
	if !ok {
		return nil, ErrLockNotFound
	}
	return rm, nil
}

// KeyLockCount returns the number of live entries in the per-key lock
// registry. Intended for tests and operational introspection — a
// steady-state value near zero confirms the refcounted release is
// reclaiming entries.
func (l *Layered) KeyLockCount() int {
	l.memMutexMu.Lock()
	defer l.memMutexMu.Unlock()
	return len(l.memMutex)
}

// GetAndLock retrieves a single value and locks the key mutex
func (l *Layered) GetAndLock(path string) (meta.Object, error) {
	if key.HasGlob(path) {
		return meta.Object{}, ErrCantLockGlob
	}
	if br := l.getBeforeRead(); br != nil {
		br(path)
	}
	rm := l._acquireLock(path)
	obj, err := l.Get(path)
	if err != nil {
		l._releaseLock(path, rm)
		return meta.Object{}, err
	}
	return obj, nil
}

// SetAndUnlock sets a value and unlocks the key mutex. The lock and
// its refcount entry are released even on early error paths so a
// caller that hit GetAndLock cannot leave a stale entry in the
// registry.
func (l *Layered) SetAndUnlock(path string, data json.RawMessage) (string, error) {
	if key.HasGlob(path) {
		return "", ErrCantLockGlob
	}
	rm, err := l._loadLock(path)
	if err != nil {
		return "", err
	}
	defer l._releaseLock(path, rm)
	if !key.IsValid(path) {
		return path, ErrInvalidPath
	}
	if len(data) == 0 {
		return path, ErrInvalidStorageData
	}
	l.writeMutex.RLock()
	defer l.writeMutex.RUnlock()
	return l.setLocked(path, data)
}

// Unlock unlocks a key mutex and releases its refcount entry. Returns
// ErrLockNotFound if no caller currently holds the path.
func (l *Layered) Unlock(path string) error {
	rm, err := l._loadLock(path)
	if err != nil {
		return err
	}
	l._releaseLock(path, rm)
	return nil
}

// getList retrieves values matching a glob pattern.
// Memory is a complete mirror of embedded, so we read from memory when available
// and only fall back to embedded in memory-less configurations.
func (l *Layered) getList(path string, order string) ([]meta.Object, error) {
	if !key.HasGlob(path) {
		return nil, ErrInvalidPattern
	}

	var res []meta.Object
	var err error
	switch {
	case l.memory != nil:
		res, err = l.memory.GetList(path)
	case l.embedded != nil:
		res, err = l.embedded.GetList(path)
	default:
		return []meta.Object{}, nil
	}
	if err != nil {
		return nil, err
	}

	if order == "desc" {
		sort.Slice(res, meta.SortDesc(res))
	} else {
		sort.Slice(res, meta.SortAsc(res))
	}

	return res, nil
}

// GetList retrieves list of values matching a glob pattern (ascending order)
func (l *Layered) GetList(path string) ([]meta.Object, error) {
	if br := l.getBeforeRead(); br != nil {
		br(path)
	}
	return l.getList(path, "asc")
}

// GetListDescending retrieves list of values matching a glob pattern (descending order)
func (l *Layered) GetListDescending(path string) ([]meta.Object, error) {
	if br := l.getBeforeRead(); br != nil {
		br(path)
	}
	return l.getList(path, "desc")
}

func (l *Layered) getN(path string, limit int, order string) ([]meta.Object, error) {
	if !key.HasGlob(path) {
		return nil, ErrInvalidPattern
	}

	res, err := l.getList(path, order)
	if err != nil {
		return nil, err
	}

	if limit > 0 && len(res) > limit {
		return res[:limit], nil
	}

	return res, nil
}

// GetN get last N elements of a path related value(s)
func (l *Layered) GetN(path string, limit int) ([]meta.Object, error) {
	if br := l.getBeforeRead(); br != nil {
		br(path)
	}
	return l.getN(path, limit, "desc")
}

// GetNAscending get first N elements of a path related value(s)
func (l *Layered) GetNAscending(path string, limit int) ([]meta.Object, error) {
	if br := l.getBeforeRead(); br != nil {
		br(path)
	}
	return l.getN(path, limit, "asc")
}

// GetNRange get last N elements in a time range
func (l *Layered) GetNRange(path string, limit int, from, to int64) ([]meta.Object, error) {
	if br := l.getBeforeRead(); br != nil {
		br(path)
	}
	if !key.HasGlob(path) {
		return nil, ErrInvalidPattern
	}
	if limit <= 0 {
		return nil, ErrInvalidLimit
	}

	all, err := l.getList(path, "desc")
	if err != nil {
		return nil, err
	}

	res := []meta.Object{}
	for _, obj := range all {
		if obj.Created >= from && obj.Created <= to {
			res = append(res, obj)
			if len(res) >= limit {
				break
			}
		}
	}

	return res, nil
}

// Keys returns all keys from all layers
func (l *Layered) Keys() ([]string, error) {
	seen := make(map[string]bool)

	// Collect from all layers
	if l.embedded != nil {
		keys, err := l.embedded.Keys()
		if err == nil {
			for _, k := range keys {
				seen[k] = true
			}
		}
	}

	if l.memory != nil {
		keys, err := l.memory.Keys()
		if err == nil {
			for _, k := range keys {
				seen[k] = true
			}
		}
	}

	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}

	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})

	return keys, nil
}

// KeysRange list keys in a path and time range
func (l *Layered) KeysRange(path string, from, to int64) ([]string, error) {
	if !key.HasGlob(path) {
		return nil, ErrInvalidPattern
	}
	if to < from {
		return nil, ErrInvalidRange
	}

	all, err := l.getList(path, "asc")
	if err != nil {
		return nil, err
	}

	keys := []string{}
	for _, obj := range all {
		if obj.Created >= from && obj.Created <= to {
			keys = append(keys, obj.Path)
		}
	}

	return keys, nil
}

// peek returns created and updated timestamps for a key
func (l *Layered) peek(k string, now int64) (int64, int64) {
	obj, err := l.Get(k)
	if err != nil {
		return now, 0
	}
	return obj.Created, now
}

// Set stores a value in all layers
func (l *Layered) Set(path string, data json.RawMessage) (string, error) {
	if !key.IsValid(path) {
		return path, ErrInvalidPath
	}
	if len(data) == 0 {
		return path, ErrInvalidStorageData
	}
	if key.IsGlob(path) {
		return path, ErrGlobNotAllowed
	}

	l.writeMutex.RLock()
	defer l.writeMutex.RUnlock()
	rm := l._acquireLock(path)
	defer l._releaseLock(path, rm)
	return l.setLocked(path, data)
}

// setLocked writes to all layers and broadcasts. Caller must hold the
// per-path refMu acquired via _acquireLock (or _loadLock for the
// GetAndLock/SetAndUnlock pattern).
// Without this serialization, concurrent writers can interleave layer writes
// (memory ends up at writer A, embedded at writer B), breaking the mirror
// invariant.
//
// If the embedded layer rejects the write, the memory layer is rolled back
// to its prior value (or deleted if the key did not exist), the error is
// returned to the caller, and the broadcast / afterWrite hooks are
// suppressed. Callers and subscribers must not be told a write succeeded
// when it did not durably commit, and in-process Get must not return data
// the durable store rejected.
func (l *Layered) setLocked(path string, data json.RawMessage) (string, error) {
	now := monotonic.Now()
	index := key.LastIndex(path)
	created, updated := l.peek(path, now)

	obj := &meta.Object{
		Created: created,
		Updated: updated,
		Index:   index,
		Path:    path,
		Data:    data,
	}

	if err := l.writeBothLayers(path, obj); err != nil {
		return index, err
	}

	if !key.Contains(l.noBroadcastKeys, path) && l.Active() {
		l.sendEvent(Event{Key: path, Operation: "set", Object: obj})
	}
	if l.afterWrite != nil {
		l.afterWrite(path)
	}

	return index, nil
}

// writeBothLayers writes obj to memory then embedded. If embedded rejects,
// memory is rolled back to its prior committed value (or deleted if the key
// did not exist), so a failed write leaves no trace in memory and Get does
// not return data the durable store rejected.
func (l *Layered) writeBothLayers(path string, obj *meta.Object) error {
	if l.memory != nil {
		var prior *meta.Object
		if existed, getErr := l.memory.Get(path); getErr == nil {
			priorCopy := existed
			prior = &priorCopy
		}
		if err := l.memory.Set(path, obj); err != nil {
			return err
		}
		if l.embedded != nil {
			if err := l.embedded.Set(path, obj); err != nil {
				if prior != nil {
					_ = l.memory.Set(path, prior)
				} else {
					_ = l.memory.Del(path)
				}
				return err
			}
		}
		return nil
	}
	if l.embedded != nil {
		return l.embedded.Set(path, obj)
	}
	return nil
}

// deleteBothLayers deletes path from memory then embedded. If embedded
// rejects, memory is rolled back by restoring the prior value, so a failed
// Del leaves the key visible exactly as before the call. Callers must
// provide the prior value (read under the same per-key lock).
func (l *Layered) deleteBothLayers(path string, prior *meta.Object) error {
	if l.memory != nil {
		if err := l.memory.Del(path); err != nil {
			return err
		}
		if l.embedded != nil {
			if err := l.embedded.Del(path); err != nil {
				if prior != nil {
					_ = l.memory.Set(path, prior)
				}
				return err
			}
		}
		return nil
	}
	if l.embedded != nil {
		return l.embedded.Del(path)
	}
	return nil
}

// deleteGlobBothLayers deletes every key matching a glob from memory then
// embedded. The pre-delete memory snapshot of the matching keys is held so
// that, if embedded rejects the delete, every removed item is restored to
// memory — keeping the layers in sync exactly as deleteBothLayers does for
// a single key.
func (l *Layered) deleteGlobBothLayers(path string) error {
	if l.memory != nil {
		snapshot, listErr := l.memory.GetList(path)
		if listErr != nil {
			return listErr
		}
		if err := l.memory.Del(path); err != nil {
			return err
		}
		if l.embedded != nil {
			if err := l.embedded.Del(path); err != nil {
				for i := range snapshot {
					item := snapshot[i]
					_ = l.memory.Set(item.Path, &item)
				}
				return err
			}
		}
		return nil
	}
	if l.embedded != nil {
		return l.embedded.Del(path)
	}
	return nil
}

// Push stores data under a new key generated from a glob pattern path
func (l *Layered) Push(path string, data json.RawMessage) (string, error) {
	if !key.IsValid(path) {
		return "", ErrInvalidPath
	}
	if len(data) == 0 {
		return "", ErrInvalidStorageData
	}
	if !key.IsGlob(path) {
		return "", ErrGlobRequired
	}

	newPath := key.Build(path)
	index := key.LastIndex(newPath)
	now := monotonic.Now()

	obj := &meta.Object{
		Created: now,
		Updated: 0,
		Index:   index,
		Path:    newPath,
		Data:    data,
	}

	l.writeMutex.RLock()
	defer l.writeMutex.RUnlock()
	rm := l._acquireLock(newPath)
	defer l._releaseLock(newPath, rm)

	if err := l.writeBothLayers(newPath, obj); err != nil {
		return index, err
	}

	if !key.Contains(l.noBroadcastKeys, newPath) && l.Active() {
		l.sendEvent(Event{Key: newPath, Operation: "set", Object: obj})
	}
	if l.afterWrite != nil {
		l.afterWrite(newPath)
	}

	return index, nil
}

// SetWithMeta set entries with metadata created/updated values
func (l *Layered) SetWithMeta(path string, data json.RawMessage, created, updated int64) (string, error) {
	if !key.IsValid(path) {
		return path, ErrInvalidPath
	}

	index := key.LastIndex(path)
	obj := &meta.Object{
		Created: created,
		Updated: updated,
		Index:   index,
		Path:    path,
		Data:    data,
	}

	l.writeMutex.RLock()
	defer l.writeMutex.RUnlock()
	rm := l._acquireLock(path)
	defer l._releaseLock(path, rm)

	if err := l.writeBothLayers(path, obj); err != nil {
		return index, err
	}

	if !key.Contains(l.noBroadcastKeys, path) && l.Active() {
		l.sendEvent(Event{Key: path, Operation: "set", Object: obj})
	}
	if l.afterWrite != nil {
		l.afterWrite(path)
	}

	return index, nil
}

// Del deletes a key from all layers
func (l *Layered) Del(path string) error {
	if key.HasGlob(path) {
		return l.delGlob(path)
	}

	l.writeMutex.RLock()
	defer l.writeMutex.RUnlock()
	rm := l._acquireLock(path)
	defer l._releaseLock(path, rm)

	o, err := l.Get(path)
	if err != nil {
		return ErrNotFound
	}
	obj := &o

	if err := l.deleteBothLayers(path, obj); err != nil {
		return err
	}

	if !key.Contains(l.noBroadcastKeys, path) && l.Active() {
		l.sendEvent(Event{Key: path, Operation: "del", Object: obj})
	}
	if l.afterWrite != nil {
		l.afterWrite(path)
	}

	return nil
}

// delGlob deletes all keys matching a glob pattern across all layers.
// Takes writeMutex.Lock exclusively so no Set/Push/SetWithMeta/Del can
// commit between the memory and embedded halves of the delete — a per-key
// mutex cannot protect this, because the glob may match keys that don't
// exist at snapshot time and could be created concurrently.
func (l *Layered) delGlob(path string) error {
	l.writeMutex.Lock()
	defer l.writeMutex.Unlock()
	if err := l.deleteGlobBothLayers(path); err != nil {
		return err
	}

	if !key.Contains(l.noBroadcastKeys, path) && l.Active() {
		l.sendEvent(Event{Key: path, Operation: "del", Object: nil})
	}
	if l.afterWrite != nil {
		l.afterWrite(path)
	}

	return nil
}

// DelSilent deletes a key from all layers without broadcasting
func (l *Layered) DelSilent(path string) error {
	if key.HasGlob(path) {
		l.writeMutex.Lock()
		defer l.writeMutex.Unlock()
		return l.deleteGlobBothLayers(path)
	}

	l.writeMutex.RLock()
	defer l.writeMutex.RUnlock()
	rm := l._acquireLock(path)
	defer l._releaseLock(path, rm)

	o, err := l.Get(path)
	if err != nil {
		return ErrNotFound
	}
	return l.deleteBothLayers(path, &o)
}

// Clear removes all data from all layers.
//
// Takes the exclusive writeMutex so a concurrent Set cannot interleave
// between the memory and embedded halves. Without that barrier, a Set
// landing in the gap leaves the new value committed to memory while
// embedded.Clear wipes embedded, diverging the two layers — same race
// class as the glob-delete bug fixed in PR #79.
func (l *Layered) Clear() {
	l.writeMutex.Lock()
	defer l.writeMutex.Unlock()
	if l.memory != nil {
		l.memory.Clear()
	}
	if l.embedded != nil {
		l.embedded.Clear()
	}
}

// WatchSharded returns the sharded storage channel
func (l *Layered) WatchSharded() *ShardedChan {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	return l.watcher
}

// sendEvent sends an event to the sharded watcher.
// The read of l.watcher is synchronized with Close (which nils it) so the
// race detector stays clean and an in-flight Send cannot race a channel close.
func (l *Layered) sendEvent(event Event) {
	l.mutex.RLock()
	defer l.mutex.RUnlock()
	if l.watcher != nil {
		l.watcher.Send(event)
	}
}

// WatchWithCallback starts goroutines that watch all sharded channels and call
// the provided callback for each event. Use this for external storages that need
// to trigger sync on storage events.
func WatchWithCallback(dataStore Database, callback func(Event)) {
	shardedWatcher := dataStore.WatchSharded()
	if shardedWatcher == nil {
		return
	}
	for i := 0; i < shardedWatcher.Count(); i++ {
		go func(ch StorageChan) {
			for {
				event, ok := <-ch
				if !ok || !dataStore.Active() {
					return
				}
				callback(event)
			}
		}(shardedWatcher.Shard(i))
	}
}
