package storage

import (
	"errors"
	"hash/fnv"
	"log"
	"sync/atomic"
	"time"

	"github.com/benitogf/go-json"

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
	// AfterWriteOp is an operation-aware companion to AfterWrite, invoked after
	// a successful write with the operation ("set" or "del"). It fires in
	// addition to AfterWrite (both run when both are set). Consumers that must
	// react differently to sets vs deletes — e.g. clearing per-operation
	// bookkeeping for exactly the operation that occurred — use this instead of
	// trying to infer the operation, which AfterWrite alone cannot convey and a
	// storage read-back cannot determine race-free.
	AfterWriteOp func(key string, op string)
	Workers      int
}

// ShardedChan manages multiple channels for per-key ordering.
type ShardedChan struct {
	shards  []StorageChan
	count   int
	dropped atomic.Int64
	onDrop  atomic.Pointer[func(Event)]
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

// Dropped returns the total number of events the sharded channel has dropped
// after exhausting its send timeout. Operators monitor this to detect a stuck
// or slow watcher consumer that's silently desyncing subscribers from storage.
func (s *ShardedChan) Dropped() int64 {
	return s.dropped.Load()
}

// SetOnDrop installs (or clears) a callback invoked for each event that
// SendWithTimeout drops on timeout. The callback runs synchronously on the
// sender's goroutine — keep it cheap. Pass nil to unset.
func (s *ShardedChan) SetOnDrop(fn func(Event)) {
	if fn == nil {
		s.onDrop.Store(nil)
		return
	}
	s.onDrop.Store(&fn)
}

// SEND_TIMEOUT is the maximum time to wait when sending an event to a shard channel.
// If the consumer goroutine is stuck or dead, the send will time out and the event
// will be dropped with a log warning, preventing permanent write hangs.
const SEND_TIMEOUT = 5 * time.Second

// SendWithTimeout attempts to send an event to the appropriate shard channel
// within the given timeout. Returns true if sent, false if timed out.
//
// On timeout the event is dropped, the dropped counter is incremented, and
// any installed OnDrop callback is invoked synchronously. The drop is now
// programmatically observable instead of leaving only a log line for
// operators to grep.
func (s *ShardedChan) SendWithTimeout(event Event, timeout time.Duration) bool {
	shard := s.ShardFor(event.Key)
	select {
	case s.shards[shard] <- event:
		return true
	case <-time.After(timeout):
		s.dropped.Add(1)
		if ptr := s.onDrop.Load(); ptr != nil {
			(*ptr)(event)
		}
		log.Printf("storage: send timeout on shard %d for key %q (consumer stuck or dead), event dropped", shard, event.Key)
		return false
	}
}

// Send routes an event to the appropriate shard based on key hash.
// Uses SEND_TIMEOUT to prevent permanent blocking if the shard consumer is stuck.
func (s *ShardedChan) Send(event Event) {
	s.SendWithTimeout(event, SEND_TIMEOUT)
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
type LayerOptions struct{}

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
