package storage

import (
	"errors"
	"hash/fnv"

	"github.com/goccy/go-json"

	"github.com/benitogf/ooo/meta"
)

// Errors
var (
	ErrNotFound           = errors.New("storage: not found")
	ErrInvalidPath        = errors.New("storage: invalid path")
	ErrInvalidPattern     = errors.New("storage: invalid pattern, glob required")
	ErrInvalidRange       = errors.New("storage: invalid range")
	ErrInvalidLimit       = errors.New("storage: invalid limit")
	ErrInvalidStorageData = errors.New("storage: invalid data (empty)")
	ErrGlobNotAllowed     = errors.New("storage: glob pattern not allowed for this operation")
	ErrGlobRequired       = errors.New("storage: glob pattern required for this operation")
	ErrCantLockGlob       = errors.New("storage: can't lock a glob pattern path")
	ErrLockNotFound       = errors.New("storage: lock not found, can't unlock")
	ErrNoop               = errors.New("storage: no operation performed")
	ErrAllLayersNil       = errors.New("storage: all storage layers are nil")
	ErrNotActive          = errors.New("storage: storage is not active")
)

// StorageChan an operation events channel
type StorageChan chan Event

// Event an operation event
type Event struct {
	Key       string
	Operation string
	Object    *meta.Object
}

// Options for the storage instance
type Options struct {
	NoBroadcastKeys []string
	BeforeRead      func(key string)
	AfterWrite      func(key string)
	Workers         int
}

// ShardedChan manages multiple channels for per-key ordering.
type ShardedChan struct {
	shards []StorageChan
	count  int
}

// NewShardedChan creates a new sharded storage channel with the given number of shards.
func NewShardedChan(shardCount int) *ShardedChan {
	if shardCount <= 0 {
		shardCount = 1
	}
	shards := make([]StorageChan, shardCount)
	for i := range shards {
		shards[i] = make(StorageChan, 100)
	}
	return &ShardedChan{
		shards: shards,
		count:  shardCount,
	}
}

// Send routes an event to the appropriate shard based on key hash.
func (s *ShardedChan) Send(event Event) {
	shard := s.ShardFor(event.Key)
	s.shards[shard] <- event
}

// ShardFor returns the shard index for a given key using FNV-1a hash.
func (s *ShardedChan) ShardFor(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32()) % s.count
}

// Shard returns the channel for a specific shard index.
func (s *ShardedChan) Shard(index int) StorageChan {
	if index < 0 || index >= s.count {
		return nil
	}
	return s.shards[index]
}

// Count returns the number of shards.
func (s *ShardedChan) Count() int {
	return s.count
}

// Close closes all shard channels.
func (s *ShardedChan) Close() {
	for _, shard := range s.shards {
		close(shard)
	}
}

// EventCallback is a callback type for storage events
type EventCallback func(event Event)

// LayerOptions configuration for individual storage layers
type LayerOptions struct {
	// SkipLoadMemory when true, skips loading data from embedded layer into memory on startup
	SkipLoadMemory bool
}

// Layer is the interface for individual storage layers (memory, embedded, sql)
// This is a simpler interface focused on raw storage operations
type Layer interface {
	// Active returns whether the layer is active
	Active() bool
	// Start initializes the layer
	Start(opt LayerOptions) error
	// Close shuts down the layer
	Close()
	// Get retrieves a single value by exact key
	Get(key string) (meta.Object, error)
	// GetList retrieves all values matching a glob pattern
	GetList(path string) ([]meta.Object, error)
	// Set stores a value
	Set(key string, obj *meta.Object) error
	// Del deletes a key
	Del(key string) error
	// Keys returns all keys
	Keys() ([]string, error)
	// Clear removes all data
	Clear()
}

// Database interface to be implemented by storages
// This is the full interface that the layered storage implements
type Database interface {
	Active() bool
	Start(Options) error
	SetBeforeRead(fn func(key string))
	Close()
	Keys() ([]string, error)
	KeysRange(path string, from, to int64) ([]string, error)
	Get(key string) (meta.Object, error)
	GetAndLock(key string) (meta.Object, error)
	GetList(path string) ([]meta.Object, error)
	GetListDescending(path string) ([]meta.Object, error)
	GetN(path string, limit int) ([]meta.Object, error)
	GetNAscending(path string, limit int) ([]meta.Object, error)
	GetNRange(path string, limit int, from, to int64) ([]meta.Object, error)
	Set(key string, data json.RawMessage) (string, error)
	Push(path string, data json.RawMessage) (string, error)
	SetWithMeta(key string, data json.RawMessage, created, updated int64) (string, error)
	SetAndUnlock(key string, data json.RawMessage) (string, error)
	Unlock(key string) error
	Del(key string) error
	DelSilent(key string) error
	Clear()
	WatchSharded() *ShardedChan
}
