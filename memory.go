package ooo

import (
	"errors"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/goccy/go-json"

	"github.com/benitogf/ooo/key"
	"github.com/benitogf/ooo/merge"
	"github.com/benitogf/ooo/meta"
)

var (
	ErrInvalidPath = errors.New("ooo: invalid path")
	ErrNotFound    = errors.New("ooo: not found")
	ErrNoop        = errors.New("ooo: noop")
)

// MemoryStorage composition of Database interface
type MemoryStorage struct {
	mem             sync.Map
	mutex           sync.RWMutex
	memMutex        sync.Map
	noBroadcastKeys []string
	watcher         StorageChan
	storage         *Storage
}

// Active provides access to the status of the storage client
func (db *MemoryStorage) Active() bool {
	db.mutex.Lock()
	defer db.mutex.Unlock()
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
		return nil, errors.New("ooo: lock not found can't unlock")
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
func (db *MemoryStorage) Keys() ([]byte, error) {
	stats := Stats{}
	db.mem.Range(func(key any, value any) bool {
		stats.Keys = append(stats.Keys, key.(string))
		return true
	})

	if stats.Keys == nil {
		stats.Keys = []string{}
	}
	sort.Slice(stats.Keys, func(i, j int) bool {
		return strings.ToLower(stats.Keys[i]) < strings.ToLower(stats.Keys[j])
	})

	return meta.Encode(stats)
}

// KeysRange list keys in a path and time range
func (db *MemoryStorage) KeysRange(path string, from, to int64) ([]string, error) {
	keys := []string{}
	if !strings.Contains(path, "*") {
		return keys, errors.New("ooo: invalid pattern")
	}

	if to < from {
		return keys, errors.New("ooo: invalid range")
	}

	db.mem.Range(func(k any, value any) bool {
		_key := k.(string)
		if !key.Match(path, _key) {
			return true
		}
		obj, err := meta.Decode(value.([]byte))
		if err != nil {
			return true
		}
		if obj.Created < from || obj.Created > to {
			log.Println("not in range", obj.Created, from, to)
			return true
		}
		keys = append(keys, _key)
		return true
	})

	return keys, nil
}

// get a key/pattern related value(s)
func (db *MemoryStorage) get(path string, order string) ([]byte, error) {
	if !strings.Contains(path, "*") {
		data, found := db.mem.Load(path)
		if !found {
			return []byte(""), ErrNotFound
		}

		return data.([]byte), nil
	}

	res := []meta.Object{}
	db.mem.Range(func(k any, value any) bool {
		if !key.Match(path, k.(string)) {
			return true
		}

		newObject, err := meta.Decode(value.([]byte))
		if err != nil {
			return true
		}

		res = append(res, newObject)
		return true
	})

	if order == "desc" {
		sort.Slice(res, meta.SortDesc(res))
	} else {
		sort.Slice(res, meta.SortAsc(res))
	}

	return meta.Encode(res)
}

// Get a key/pattern related value(s)
func (db *MemoryStorage) Get(path string) ([]byte, error) {
	return db.get(path, "asc")
}

// Get a key/pattern related value(s)
func (db *MemoryStorage) GetDescending(path string) ([]byte, error) {
	return db.get(path, "desc")
}

func (db *MemoryStorage) GetAndLock(path string) ([]byte, error) {
	if strings.Contains(path, "*") {
		return []byte{}, errors.New("ooo: can't lock a glob pattern path")
	}
	lock := db._getLock(path)
	lock.Lock()
	return db.Get(path)
}

func (db *MemoryStorage) SetAndUnlock(path string, data json.RawMessage) (string, error) {
	if strings.Contains(path, "*") {
		return "", errors.New("ooo: can't lock a glob pattern path")
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
		return errors.New("ooo: lock not found can't unlock")
	}
	lock.(*sync.Mutex).Unlock()
	return nil
}

func (db *MemoryStorage) getN(path string, limit int, order string) ([]meta.Object, error) {
	res := []meta.Object{}
	if !strings.Contains(path, "*") {
		return res, errors.New("ooo: invalid pattern")
	}

	if limit <= 0 {
		return res, errors.New("ooo: invalid limit")
	}

	db.mem.Range(func(k any, value any) bool {
		if !key.Match(path, k.(string)) {
			return true
		}

		newObject, err := meta.Decode(value.([]byte))
		if err != nil {
			return true
		}

		res = append(res, newObject)
		return true
	})

	if order == "desc" {
		sort.Slice(res, meta.SortDesc(res))
	} else {
		sort.Slice(res, meta.SortAsc(res))
	}

	if len(res) > limit {
		return res[:limit], nil
	}

	return res, nil
}

// GetN get last N elements of a path related value(s)
func (db *MemoryStorage) GetN(path string, limit int) ([]meta.Object, error) {
	return db.getN(path, limit, "desc")
}

// GetN get last N elements of a path related value(s)
func (db *MemoryStorage) GetNAscending(path string, limit int) ([]meta.Object, error) {
	return db.getN(path, limit, "asc")
}

// GetNRange get last N elements of a path related value(s)
func (db *MemoryStorage) GetNRange(path string, limit int, from, to int64) ([]meta.Object, error) {
	res := []meta.Object{}
	if !strings.Contains(path, "*") {
		return res, errors.New("ooo: invalid pattern")
	}

	if limit <= 0 {
		return res, errors.New("ooo: invalid limit")
	}

	db.mem.Range(func(k any, value any) bool {
		current := k.(string)
		if !key.Match(path, current) {
			return true
		}
		paths := strings.Split(current, "/")
		created := key.Decode(paths[len(paths)-1])
		if created < from || created > to {
			return true
		}

		newObject, err := meta.Decode(value.([]byte))
		if err != nil {
			return true
		}

		res = append(res, newObject)
		return true
	})

	sort.Slice(res, meta.SortDesc(res))

	if len(res) > limit {
		return res[:limit], nil
	}

	return res, nil
}

// Peek a value timestamps
func (db *MemoryStorage) Peek(key string, now int64) (int64, int64) {
	previous, found := db.mem.Load(key)
	if !found {
		return now, 0
	}

	oldObject, err := meta.Decode(previous.([]byte))
	if err != nil {
		return now, 0
	}

	return oldObject.Created, now
}

// Set a value
func (db *MemoryStorage) Set(path string, data json.RawMessage) (string, error) {
	if !key.IsValid(path) {
		return path, ErrInvalidPath
	}
	if len(data) == 0 {
		return path, errors.New("ooo: invalid storage data (empty)")
	}
	now := time.Now().UTC().UnixNano()

	if !strings.Contains(path, "*") {
		index := key.LastIndex(path)
		created, updated := db.Peek(path, now)
		db.mem.Store(path, meta.New(&meta.Object{
			Created: created,
			Updated: updated,
			Index:   index,
			Path:    path,
			Data:    data,
		}))

		if !key.Contains(db.noBroadcastKeys, path) && db.Active() {
			db.watcher <- StorageEvent{Key: path, Operation: "set"}
		}
		return index, nil
	}

	// DO BATCH SET

	return path, nil
}

func (db *MemoryStorage) _patch(path string, data json.RawMessage, now int64) (string, error) {
	raw, found := db.mem.Load(path)
	if !found {
		return path, ErrNotFound
	}

	obj, err := meta.Decode(raw.([]byte))
	if err != nil {
		return path, err
	}

	merged, info, err := merge.MergeBytes(obj.Data, data)
	if err != nil {
		return path, err
	}

	if len(info.Replaced) == 0 {
		return path, ErrNoop
	}

	index := key.LastIndex(path)
	created, updated := db.Peek(path, now)
	db.mem.Store(path, meta.New(&meta.Object{
		Created: created,
		Updated: updated,
		Index:   index,
		Path:    path,
		Data:    merged,
	}))

	return path, nil
}

// Set a value to matching keys
func (db *MemoryStorage) Patch(path string, data json.RawMessage) (string, error) {
	if !key.IsValid(path) {
		return path, ErrInvalidPath
	}
	if len(data) == 0 {
		return path, errors.New("ooo: invalid storage data (empty)")
	}

	now := time.Now().UTC().UnixNano()
	if !strings.Contains(path, "*") {
		index, err := db._patch(path, data, now)
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
		_, err := db._patch(key, data, now)
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
	db.mem.Store(path, meta.New(&meta.Object{
		Created: created,
		Updated: updated,
		Index:   index,
		Path:    path,
		Data:    data,
	}))

	if len(path) > 8 && path[0:7] == "history" {
		return index, nil
	}

	if !key.Contains(db.noBroadcastKeys, path) && db.Active() {
		db.watcher <- StorageEvent{Key: path, Operation: "set"}
	}
	return index, nil
}

// Del a key/pattern value(s)
func (db *MemoryStorage) Del(path string) error {
	if !strings.Contains(path, "*") {
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
	return db.watcher
}
