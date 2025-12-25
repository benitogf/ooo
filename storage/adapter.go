package storage

import (
	"github.com/goccy/go-json"

	"github.com/benitogf/ooo/meta"
)

// Adapter wraps a storage.Database to provide compatibility with ooo.Database interface
// This allows the new storage package to be used with the existing ooo server
type Adapter struct {
	db Database
}

// NewAdapter creates a new adapter wrapping a storage.Database
func NewAdapter(db Database) *Adapter {
	return &Adapter{db: db}
}

// Active returns whether the storage is active
func (a *Adapter) Active() bool {
	return a.db.Active()
}

// Start initializes the storage
func (a *Adapter) Start(opt Options) error {
	return a.db.Start(opt)
}

// Close shuts down the storage
func (a *Adapter) Close() {
	a.db.Close()
}

// Keys returns all keys
func (a *Adapter) Keys() ([]string, error) {
	return a.db.Keys()
}

// KeysRange returns keys in a time range
func (a *Adapter) KeysRange(path string, from, to int64) ([]string, error) {
	return a.db.KeysRange(path, from, to)
}

// Get retrieves a single value
func (a *Adapter) Get(key string) (meta.Object, error) {
	return a.db.Get(key)
}

// GetAndLock retrieves a value and locks the key
func (a *Adapter) GetAndLock(key string) (meta.Object, error) {
	return a.db.GetAndLock(key)
}

// GetList retrieves values matching a pattern
func (a *Adapter) GetList(path string) ([]meta.Object, error) {
	return a.db.GetList(path)
}

// GetListDescending retrieves values matching a pattern in descending order
func (a *Adapter) GetListDescending(path string) ([]meta.Object, error) {
	return a.db.GetListDescending(path)
}

// GetN retrieves N values matching a pattern
func (a *Adapter) GetN(path string, limit int) ([]meta.Object, error) {
	return a.db.GetN(path, limit)
}

// GetNAscending retrieves N values matching a pattern in ascending order
func (a *Adapter) GetNAscending(path string, limit int) ([]meta.Object, error) {
	return a.db.GetNAscending(path, limit)
}

// GetNRange retrieves N values in a time range
func (a *Adapter) GetNRange(path string, limit int, from, to int64) ([]meta.Object, error) {
	return a.db.GetNRange(path, limit, from, to)
}

// Set stores a value
func (a *Adapter) Set(key string, data json.RawMessage) (string, error) {
	return a.db.Set(key, data)
}

// Push stores data under a new key
func (a *Adapter) Push(path string, data json.RawMessage) (string, error) {
	return a.db.Push(path, data)
}

// Patch applies a patch to matching keys
func (a *Adapter) Patch(key string, data json.RawMessage) (string, error) {
	return a.db.Patch(key, data)
}

// SetWithMeta stores a value with metadata
func (a *Adapter) SetWithMeta(key string, data json.RawMessage, created, updated int64) (string, error) {
	return a.db.SetWithMeta(key, data, created, updated)
}

// SetAndUnlock stores a value and unlocks the key
func (a *Adapter) SetAndUnlock(key string, data json.RawMessage) (string, error) {
	return a.db.SetAndUnlock(key, data)
}

// Unlock unlocks a key
func (a *Adapter) Unlock(key string) error {
	return a.db.Unlock(key)
}

// Del deletes a key
func (a *Adapter) Del(key string) error {
	return a.db.Del(key)
}

// DelSilent deletes a key without broadcasting
func (a *Adapter) DelSilent(key string) error {
	return a.db.DelSilent(key)
}

// Clear removes all data
func (a *Adapter) Clear() {
	a.db.Clear()
}

// WatchSharded returns the sharded storage channel
func (a *Adapter) WatchSharded() *ShardedChan {
	return a.db.WatchSharded()
}

// Underlying returns the underlying database
func (a *Adapter) Underlying() Database {
	return a.db
}
