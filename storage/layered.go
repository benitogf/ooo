package storage

import (
	"sort"
	"sync"

	"github.com/goccy/go-json"

	"github.com/benitogf/ooo/key"
	"github.com/benitogf/ooo/merge"
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
	memMutex        sync.Map
	noBroadcastKeys []string
	watcher         *ShardedChan
	active          bool
	beforeRead      func(key string)
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
	// Load from embedded into memory unless skipped
	if l.embedded != nil && l.memory != nil && !l.memoryOpt.SkipLoadMemory {
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
	if l.beforeRead != nil {
		l.beforeRead(path)
	}
	if key.HasGlob(path) {
		return meta.Object{}, ErrGlobNotAllowed
	}

	// Try memory first
	if l.memory != nil {
		obj, err := l.memory.Get(path)
		if err == nil {
			return obj, nil
		}
	}

	// Try embedded
	if l.embedded != nil {
		obj, err := l.embedded.Get(path)
		if err == nil {
			// Populate memory cache
			if l.memory != nil {
				_ = l.memory.Set(path, &obj)
			}
			return obj, nil
		}
	}

	return meta.Object{}, ErrNotFound
}

func (l *Layered) _getLock(path string) *sync.Mutex {
	newLock := sync.Mutex{}
	lock, _ := l.memMutex.LoadOrStore(path, &newLock)
	return lock.(*sync.Mutex)
}

func (l *Layered) _loadLock(path string) (*sync.Mutex, error) {
	lock, found := l.memMutex.Load(path)
	if !found {
		return nil, ErrLockNotFound
	}
	return lock.(*sync.Mutex), nil
}

// GetAndLock retrieves a single value and locks the key mutex
func (l *Layered) GetAndLock(path string) (meta.Object, error) {
	if key.HasGlob(path) {
		return meta.Object{}, ErrCantLockGlob
	}
	if l.beforeRead != nil {
		l.beforeRead(path)
	}
	lock := l._getLock(path)
	lock.Lock()
	obj, err := l.Get(path)
	if err != nil {
		lock.Unlock()
		return meta.Object{}, err
	}
	return obj, nil
}

// SetAndUnlock sets a value and unlocks the key mutex
func (l *Layered) SetAndUnlock(path string, data json.RawMessage) (string, error) {
	if key.HasGlob(path) {
		return "", ErrCantLockGlob
	}
	lock, err := l._loadLock(path)
	if err != nil {
		return "", err
	}
	res, err := l.Set(path, data)
	lock.Unlock()
	return res, err
}

// Unlock unlocks a key mutex
func (l *Layered) Unlock(path string) error {
	lock, found := l.memMutex.Load(path)
	if !found {
		return ErrLockNotFound
	}
	lock.(*sync.Mutex).Unlock()
	return nil
}

// getList retrieves values matching a glob pattern from all layers
func (l *Layered) getList(path string, order string) ([]meta.Object, error) {
	if !key.HasGlob(path) {
		return nil, ErrInvalidPattern
	}

	// Collect from all layers, deduplicate by path
	seen := make(map[string]meta.Object)

	// Embedded layer
	if l.embedded != nil {
		objs, err := l.embedded.GetList(path)
		if err == nil {
			for _, obj := range objs {
				seen[obj.Path] = obj
			}
		}
	}

	// Memory layer (fastest, most recent)
	if l.memory != nil {
		objs, err := l.memory.GetList(path)
		if err == nil {
			for _, obj := range objs {
				seen[obj.Path] = obj
			}
		}
	}

	res := make([]meta.Object, 0, len(seen))
	for _, obj := range seen {
		res = append(res, obj)
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
	if l.beforeRead != nil {
		l.beforeRead(path)
	}
	return l.getList(path, "asc")
}

// GetListDescending retrieves list of values matching a glob pattern (descending order)
func (l *Layered) GetListDescending(path string) ([]meta.Object, error) {
	if l.beforeRead != nil {
		l.beforeRead(path)
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
	if l.beforeRead != nil {
		l.beforeRead(path)
	}
	return l.getN(path, limit, "desc")
}

// GetNAscending get first N elements of a path related value(s)
func (l *Layered) GetNAscending(path string, limit int) ([]meta.Object, error) {
	if l.beforeRead != nil {
		l.beforeRead(path)
	}
	return l.getN(path, limit, "asc")
}

// GetNRange get last N elements in a time range
func (l *Layered) GetNRange(path string, limit int, from, to int64) ([]meta.Object, error) {
	if l.beforeRead != nil {
		l.beforeRead(path)
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

	// Write to all layers
	if l.memory != nil {
		_ = l.memory.Set(path, obj)
	}
	if l.embedded != nil {
		_ = l.embedded.Set(path, obj)
	}

	if !key.Contains(l.noBroadcastKeys, path) && l.Active() {
		l.sendEvent(Event{Key: path, Operation: "set", Object: obj})
	}

	return index, nil
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

	// Write to all layers
	if l.memory != nil {
		_ = l.memory.Set(newPath, obj)
	}
	if l.embedded != nil {
		_ = l.embedded.Set(newPath, obj)
	}

	if !key.Contains(l.noBroadcastKeys, newPath) && l.Active() {
		l.sendEvent(Event{Key: newPath, Operation: "set", Object: obj})
	}

	return index, nil
}

// patchSingle applies a patch to a single key
func (l *Layered) patchSingle(path string, data json.RawMessage, now int64) (*meta.Object, error) {
	obj, err := l.Get(path)
	if err != nil {
		return nil, err
	}

	merged, info, err := merge.MergeBytes(obj.Data, data)
	if err != nil {
		return nil, err
	}

	if len(info.Replaced) == 0 {
		return nil, ErrNoop
	}

	index := key.LastIndex(path)
	created, updated := l.peek(path, now)

	newObj := &meta.Object{
		Created: created,
		Updated: updated,
		Index:   index,
		Path:    path,
		Data:    merged,
	}

	// Write to all layers
	if l.memory != nil {
		_ = l.memory.Set(path, newObj)
	}
	if l.embedded != nil {
		_ = l.embedded.Set(path, newObj)
	}

	return newObj, nil
}

// Patch applies a patch to matching keys
func (l *Layered) Patch(path string, data json.RawMessage) (string, error) {
	if !key.IsValid(path) {
		return path, ErrInvalidPath
	}
	if len(data) == 0 {
		return path, ErrInvalidStorageData
	}

	now := monotonic.Now()

	if !key.HasGlob(path) {
		obj, err := l.patchSingle(path, data, now)
		if err != nil {
			return path, err
		}
		if !key.Contains(l.noBroadcastKeys, path) && l.Active() {
			l.sendEvent(Event{Key: path, Operation: "set", Object: obj})
		}
		return path, nil
	}

	// Get all matching keys
	keys, err := l.Keys()
	if err != nil {
		return path, err
	}

	for _, k := range keys {
		if !key.Match(path, k) {
			continue
		}
		obj, err := l.patchSingle(k, data, now)
		if err != nil {
			return path, err
		}
		if !key.Contains(l.noBroadcastKeys, k) && l.Active() {
			l.sendEvent(Event{Key: k, Operation: "set", Object: obj})
		}
	}

	return path, nil
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

	// Write to all layers
	if l.memory != nil {
		_ = l.memory.Set(path, obj)
	}
	if l.embedded != nil {
		_ = l.embedded.Set(path, obj)
	}

	if !key.Contains(l.noBroadcastKeys, path) && l.Active() {
		l.sendEvent(Event{Key: path, Operation: "set", Object: obj})
	}

	return index, nil
}

// Del deletes a key from all layers
func (l *Layered) Del(path string) error {
	var obj *meta.Object

	// Get object for broadcast before deleting
	if !key.HasGlob(path) {
		o, err := l.Get(path)
		if err != nil {
			return ErrNotFound
		}
		obj = &o
	}

	// Delete from all layers
	if l.memory != nil {
		_ = l.memory.Del(path)
	}
	if l.embedded != nil {
		_ = l.embedded.Del(path)
	}

	if !key.Contains(l.noBroadcastKeys, path) && l.Active() {
		l.sendEvent(Event{Key: path, Operation: "del", Object: obj})
	}

	return nil
}

// DelSilent deletes a key from all layers without broadcasting
func (l *Layered) DelSilent(path string) error {
	// Check existence for non-glob paths
	if !key.HasGlob(path) {
		_, err := l.Get(path)
		if err != nil {
			return ErrNotFound
		}
	}

	// Delete from all layers
	if l.memory != nil {
		_ = l.memory.Del(path)
	}
	if l.embedded != nil {
		_ = l.embedded.Del(path)
	}

	return nil
}

// Clear removes all data from all layers
func (l *Layered) Clear() {
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

// sendEvent sends an event to the sharded watcher
func (l *Layered) sendEvent(event Event) {
	if l.watcher != nil {
		l.watcher.Send(event)
	}
}
