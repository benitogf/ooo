package ooo

import (
	"github.com/goccy/go-json"

	"github.com/benitogf/ooo/meta"
)

// StorageChan an operation events channel
type StorageChan chan StorageEvent

// StorageEvent an operation event
type StorageEvent struct {
	Key       string
	Operation string
}

// StorageOpt options of the storage instance
type StorageOpt struct {
	NoBroadcastKeys []string
	DbOpt           interface{}
	BeforeRead      func(key string)
}

// Database interface to be implemented by storages
//
// Active: returns a boolean with the state of the storage
//
// Start: will attempt to start a storage client
//
// Close: closes the storage client
//
// Keys: returns a list with existing keys in the storage
//
// Get(key): retrieve a value or list of values, the key can include a glob pattern (ascending created time order)
//
// GetDescending(key): retrieve a value or list of values, the key can include a glob pattern (descending created time order)
//
// GetN(path, N): retrieve N list of values matching a glob pattern (descending created time order)
//
// GetNAscending(path, N): retrieve N list of values matching a glob pattern (ascending created time order)
//
// GetNRange(path, N, from, to): retrieve N list of values matching a glob pattern path created in the time from-to time range (descending created time order)
//
// Set(key, data): store data under the provided key, key cannot not include glob pattern
//
// SetWithMeta(key, data, created, updated): store data by manually providing created/updated time values
//
// GetAndLock(key): same as get but will lock the key mutex until SetAndUnlock is called for the same key (non glob key only)
//
// SetAndUnlock(key, data): same as set but will unlock the key mutex (non glob key only)
//
// Unlock(key): unlock key mutex
//
// Del(key): Delete a key from the storage
//
// Clear: will clear all data from the storage
//
// Watch: returns a channel that will receive any set or del operation
type Database interface {
	Active() bool
	Start(StorageOpt) error
	Close()
	Keys() ([]byte, error)
	KeysRange(path string, from, to int64) ([]string, error)
	Get(key string) ([]byte, error)
	GetDescending(key string) ([]byte, error)
	GetN(path string, limit int) ([]meta.Object, error)
	GetNAscending(path string, limit int) ([]meta.Object, error)
	GetNRange(path string, limit int, from, to int64) ([]meta.Object, error)
	Set(key string, data json.RawMessage) (string, error)
	Patch(key string, data json.RawMessage) (string, error)
	SetWithMeta(key string, data json.RawMessage, created, updated int64) (string, error)
	GetAndLock(key string) ([]byte, error)
	SetAndUnlock(key string, data json.RawMessage) (string, error)
	Unlock(key string) error
	Del(key string) error
	Clear()
	Watch() StorageChan
}

// Storage abstraction of persistent data layer
type Storage struct {
	Active bool
	Db     Database
}

// Stats data structure of global keys
type Stats struct {
	Keys []string `json:"keys"`
}

// WatchStorageNoop a noop reader of the watch channel
func WatchStorageNoop(dataStore Database) {
	for {
		<-dataStore.Watch()
		if !dataStore.Active() {
			break
		}
	}
}

// StorageEventCallback is a callback type for storage events
type StorageEventCallback func(event StorageEvent)
