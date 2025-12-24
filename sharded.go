package ooo

import (
	"hash/fnv"
)

// ShardedStorageChan manages multiple channels for per-key ordering.
// Events for the same key are always routed to the same channel,
// ensuring ordering is preserved per key while allowing parallelism across keys.
type ShardedStorageChan struct {
	shards []StorageChan
	count  int
}

// NewShardedStorageChan creates a new sharded storage channel with the given number of shards.
func NewShardedStorageChan(shardCount int) *ShardedStorageChan {
	if shardCount <= 0 {
		shardCount = 1
	}
	shards := make([]StorageChan, shardCount)
	for i := range shards {
		shards[i] = make(StorageChan, 100) // buffered to avoid blocking
	}
	return &ShardedStorageChan{
		shards: shards,
		count:  shardCount,
	}
}

// Send routes an event to the appropriate shard based on key hash.
func (s *ShardedStorageChan) Send(event StorageEvent) {
	shard := s.shardFor(event.Key)
	s.shards[shard] <- event
}

// shardFor returns the shard index for a given key using FNV-1a hash.
func (s *ShardedStorageChan) shardFor(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32()) % s.count
}

// Shard returns the channel for a specific shard index.
func (s *ShardedStorageChan) Shard(index int) StorageChan {
	if index < 0 || index >= s.count {
		return nil
	}
	return s.shards[index]
}

// Count returns the number of shards.
func (s *ShardedStorageChan) Count() int {
	return s.count
}

// Close closes all shard channels.
func (s *ShardedStorageChan) Close() {
	for _, shard := range s.shards {
		close(shard)
	}
}
