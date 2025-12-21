package ooo

import (
	"sort"
	"strings"
	"sync"

	"github.com/goccy/go-json"

	"github.com/benitogf/ooo/key"
	"github.com/benitogf/ooo/merge"
	"github.com/benitogf/ooo/meta"
	"github.com/benitogf/ooo/monotonic"
)

// MemoryStorage composition of Database interface
type MemoryStorage struct {
	mem             sync.Map
	mutex           sync.RWMutex
	memMutex        sync.Map
	noBroadcastKeys []string
	watcher         StorageChan
	storage         *Storage
	beforeRead      func(key string)
}

// Active provides access to the status of the storage client
func (db *MemoryStorage) Active() bool {
	db.mutex.RLock()
	defer db.mutex.RUnlock()
	return db.storage.Active
}

// Start the storage client
func (db *MemoryStorage) Start(storageOpt StorageOpt) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()
	if db.storage == nil {
		db.storage = &Storage{}
	}
	if db.watcher == nil {
		db.watcher = make(StorageChan)
	}
	db.noBroadcastKeys = storageOpt.NoBroadcastKeys
	db.beforeRead = storageOpt.BeforeRead
	db.storage.Active = true
	return nil
}

// Close the storage client
func (db *MemoryStorage) Close() {
	db.mutex.Lock()
	defer db.mutex.Unlock()
	db.storage.Active = false
	close(db.watcher)
	db.watcher = nil
}

func (db *MemoryStorage) _getLock(path string) *sync.Mutex {
	newLock := sync.Mutex{}
	lock, _ := db.memMutex.LoadOrStore(path, &newLock)
	return lock.(*sync.Mutex)
}

func (db *MemoryStorage) _loadLock(path string) (*sync.Mutex, error) {
	lock, found := db.memMutex.Load(path)
	if !found {
		return nil, ErrLockNotFound
	}
	return lock.(*sync.Mutex), nil
}

// Clear all keys in the storage
func (db *MemoryStorage) Clear() {
	db.mem.Range(func(key any, value any) bool {
		db.mem.Delete(key)
		return true
	})
}

// Keys list all the keys in the storage
func (db *MemoryStorage) Keys() ([]string, error) {
	keys := []string{}
	db.mem.Range(func(k any, value any) bool {
		keys = append(keys, k.(string))
		return true
	})

	sort.Slice(keys, func(i, j int) bool {
		return strings.ToLower(keys[i]) < strings.ToLower(keys[j])
	})

	return keys, nil
}

// KeysRange list keys in a path and time range
func (db *MemoryStorage) KeysRange(path string, from, to int64) ([]string, error) {
	keys := []string{}
	if !key.HasGlob(path) {
		return keys, ErrInvalidPattern
	}

	if to < from {
		return keys, ErrInvalidRange
	}

	db.mem.Range(func(k any, value any) bool {
		_key := k.(string)
		if !key.Match(path, _key) {
			return true
		}
		// Direct struct access - no JSON decode needed
		obj := value.(*meta.Object)
		if obj.Created < from || obj.Created > to {
			return true
		}
		keys = append(keys, _key)
		return true
	})

	return keys, nil
}

// getList retrieves values matching a glob pattern
func (db *MemoryStorage) getList(path string, order string) ([]meta.Object, error) {
	if !key.HasGlob(path) {
		return nil, ErrInvalidPattern
	}

	res := []meta.Object{}
	db.mem.Range(func(k any, value any) bool {
		if !key.Match(path, k.(string)) {
			return true
		}
		res = append(res, *value.(*meta.Object))
		return true
	})

	if order == "desc" {
		sort.Slice(res, meta.SortDesc(res))
	} else {
		sort.Slice(res, meta.SortAsc(res))
	}

	return res, nil
}

// Get retrieves a single value by exact key (non-glob).
func (db *MemoryStorage) Get(path string) (meta.Object, error) {
	if db.beforeRead != nil {
		db.beforeRead(path)
	}
	if key.HasGlob(path) {
		return meta.Object{}, ErrGlobNotAllowed
	}
	data, found := db.mem.Load(path)
	if !found {
		return meta.Object{}, ErrNotFound
	}
	return *data.(*meta.Object), nil
}

// GetList retrieves list of values matching a glob pattern (ascending order).
func (db *MemoryStorage) GetList(path string) ([]meta.Object, error) {
	if db.beforeRead != nil {
		db.beforeRead(path)
	}
	return db.getList(path, "asc")
}

// GetListDescending retrieves list of values matching a glob pattern (descending order).
func (db *MemoryStorage) GetListDescending(path string) ([]meta.Object, error) {
	if db.beforeRead != nil {
		db.beforeRead(path)
	}
	return db.getList(path, "desc")
}

// GetAndLock retrieves a single value and locks the key mutex.
func (db *MemoryStorage) GetAndLock(path string) (meta.Object, error) {
	if key.HasGlob(path) {
		return meta.Object{}, ErrCantLockGlob
	}
	if db.beforeRead != nil {
		db.beforeRead(path)
	}
	lock := db._getLock(path)
	lock.Lock()
	data, found := db.mem.Load(path)
	if !found {
		lock.Unlock()
		return meta.Object{}, ErrNotFound
	}
	return *data.(*meta.Object), nil
}

func (db *MemoryStorage) SetAndUnlock(path string, data json.RawMessage) (string, error) {
	if key.HasGlob(path) {
		return "", ErrCantLockGlob
	}
	lock, err := db._loadLock(path)
	if err != nil {
		return "", err
	}
	res, err := db.Set(path, data)
	lock.Unlock()
	return res, err
}

func (db *MemoryStorage) Unlock(path string) error {
	lock, found := db.memMutex.Load(path)
	if !found {
		return ErrLockNotFound
	}
	lock.(*sync.Mutex).Unlock()
	return nil
}

func (db *MemoryStorage) getN(path string, limit int, order string) ([]meta.Object, error) {
	res := []meta.Object{}
	if !key.HasGlob(path) {
		return res, ErrInvalidPattern
	}

	db.mem.Range(func(k any, value any) bool {
		if !key.Match(path, k.(string)) {
			return true
		}
		// Direct struct access - no JSON decode needed
		res = append(res, *value.(*meta.Object))
		return true
	})

	if order == "desc" {
		sort.Slice(res, meta.SortDesc(res))
	} else {
		sort.Slice(res, meta.SortAsc(res))
	}

	// limit <= 0 means no limit (return all)
	if limit > 0 && len(res) > limit {
		return res[:limit], nil
	}

	return res, nil
}

// GetN get last N elements of a path related value(s)
func (db *MemoryStorage) GetN(path string, limit int) ([]meta.Object, error) {
	if db.beforeRead != nil {
		db.beforeRead(path)
	}
	return db.getN(path, limit, "desc")
}

// GetNAscending get last N elements of a path related value(s)
func (db *MemoryStorage) GetNAscending(path string, limit int) ([]meta.Object, error) {
	if db.beforeRead != nil {
		db.beforeRead(path)
	}
	return db.getN(path, limit, "asc")
}

// GetNRange get last N elements of a path related value(s)
func (db *MemoryStorage) GetNRange(path string, limit int, from, to int64) ([]meta.Object, error) {
	if db.beforeRead != nil {
		db.beforeRead(path)
	}
	res := []meta.Object{}
	if !key.HasGlob(path) {
		return res, ErrInvalidPattern
	}

	if limit <= 0 {
		return res, ErrInvalidLimit
	}

	db.mem.Range(func(k any, value any) bool {
		current := k.(string)
		if !key.Match(path, current) {
			return true
		}
		// Direct struct access - no JSON decode needed
		obj := value.(*meta.Object)
		if obj.Created < from || obj.Created > to {
			return true
		}
		res = append(res, *obj)
		return true
	})

	sort.Slice(res, meta.SortDesc(res))

	if len(res) > limit {
		return res[:limit], nil
	}

	return res, nil
}

// Peek returns the created and updated timestamps for a key.
// If the key doesn't exist, returns (now, 0) to indicate a new entry.
func (db *MemoryStorage) Peek(k string, now int64) (int64, int64) {
	previous, found := db.mem.Load(k)
	if !found {
		return now, 0
	}
	// Direct struct access - no JSON decode needed
	return previous.(*meta.Object).Created, now
}

// Set a value
func (db *MemoryStorage) Set(path string, data json.RawMessage) (string, error) {
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
	created, updated := db.Peek(path, now)
	// Store struct directly - no JSON encoding on write
	db.mem.Store(path, &meta.Object{
		Created: created,
		Updated: updated,
		Index:   index,
		Path:    path,
		Data:    data,
	})

	if !key.Contains(db.noBroadcastKeys, path) && db.Active() {
		db.watcher <- StorageEvent{Key: path, Operation: "set"}
	}
	return index, nil
}

// Push stores data under a new key generated from a glob pattern path.
// The path must end with /* (glob pattern). Returns the generated index.
func (db *MemoryStorage) Push(path string, data json.RawMessage) (string, error) {
	if !key.IsValid(path) {
		return "", ErrInvalidPath
	}
	if len(data) == 0 {
		return "", ErrInvalidStorageData
	}

	if !key.IsGlob(path) {
		return "", ErrGlobRequired
	}

	// Generate new key from glob pattern
	newPath := key.Build(path)
	index := key.LastIndex(newPath)
	now := monotonic.Now()

	// Store struct directly - no JSON encoding on write
	db.mem.Store(newPath, &meta.Object{
		Created: now,
		Updated: now,
		Index:   index,
		Path:    newPath,
		Data:    data,
	})

	if !key.Contains(db.noBroadcastKeys, newPath) && db.Active() {
		db.watcher <- StorageEvent{Key: newPath, Operation: "set"}
	}

	return index, nil
}

// patchSingle applies a patch to a single key (non-glob path).
func (db *MemoryStorage) patchSingle(path string, data json.RawMessage, now int64) (string, error) {
	raw, found := db.mem.Load(path)
	if !found {
		return path, ErrNotFound
	}

	// Direct struct access - no JSON decode needed
	obj := raw.(*meta.Object)

	merged, info, err := merge.MergeBytes(obj.Data, data)
	if err != nil {
		return path, err
	}

	if len(info.Replaced) == 0 {
		return path, ErrNoop
	}

	index := key.LastIndex(path)
	created, updated := db.Peek(path, now)
	// Store struct directly - no JSON encoding on write
	db.mem.Store(path, &meta.Object{
		Created: created,
		Updated: updated,
		Index:   index,
		Path:    path,
		Data:    merged,
	})

	return path, nil
}

// Set a value to matching keys
func (db *MemoryStorage) Patch(path string, data json.RawMessage) (string, error) {
	if !key.IsValid(path) {
		return path, ErrInvalidPath
	}
	if len(data) == 0 {
		return path, ErrInvalidStorageData
	}

	now := monotonic.Now()
	if !key.HasGlob(path) {
		index, err := db.patchSingle(path, data, now)
		if err != nil {
			return path, err
		}

		if !key.Contains(db.noBroadcastKeys, path) && db.Active() {
			db.watcher <- StorageEvent{Key: path, Operation: "set"}
		}
		return index, nil
	}

	keys := []string{}
	db.mem.Range(func(_key any, value any) bool {
		current := _key.(string)
		if !key.Match(path, current) {
			return true
		}
		keys = append(keys, current)
		return true
	})

	// batch patch
	for _, key := range keys {
		_, err := db.patchSingle(key, data, now)
		if err != nil {
			return path, err
		}
	}

	return path, nil
}

// SetWithMeta set entries with metadata created/updated values
func (db *MemoryStorage) SetWithMeta(path string, data json.RawMessage, created int64, updated int64) (string, error) {
	if !key.IsValid(path) {
		return path, ErrInvalidPath
	}
	index := key.LastIndex(path)
	// Store struct directly - no JSON encoding on write
	db.mem.Store(path, &meta.Object{
		Created: created,
		Updated: updated,
		Index:   index,
		Path:    path,
		Data:    data,
	})

	if !key.Contains(db.noBroadcastKeys, path) && db.Active() {
		db.watcher <- StorageEvent{Key: path, Operation: "set"}
	}
	return index, nil
}

// Del a key/pattern value(s)
func (db *MemoryStorage) Del(path string) error {
	if !key.HasGlob(path) {
		_, found := db.mem.Load(path)
		if !found {
			return ErrNotFound
		}
		db.mem.Delete(path)
		if !key.Contains(db.noBroadcastKeys, path) && db.Active() {
			db.watcher <- StorageEvent{Key: path, Operation: "del"}
		}
		return nil
	}

	db.mem.Range(func(k any, value any) bool {
		if key.Match(path, k.(string)) {
			db.mem.Delete(k.(string))
		}
		return true
	})
	if !key.Contains(db.noBroadcastKeys, path) && db.Active() {
		db.watcher <- StorageEvent{Key: path, Operation: "del"}
	}
	return nil
}

// Watch the storage set/del events
func (db *MemoryStorage) Watch() StorageChan {
	db.mutex.RLock()
	defer db.mutex.RUnlock()
	return db.watcher
}
