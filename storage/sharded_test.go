package storage

import (
	"sync"
	"testing"
)

func TestNewShardedChan(t *testing.T) {
	tests := []struct {
		name       string
		shardCount int
		wantCount  int
	}{
		{"positive count", 4, 4},
		{"zero count defaults to 1", 0, 1},
		{"negative count defaults to 1", -5, 1},
		{"single shard", 1, 1},
		{"many shards", 16, 16},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewShardedChan(tt.shardCount)
			defer s.Close()

			if got := s.Count(); got != tt.wantCount {
				t.Errorf("Count() = %d, want %d", got, tt.wantCount)
			}

			// Verify all shards are accessible
			for i := 0; i < tt.wantCount; i++ {
				if shard := s.Shard(i); shard == nil {
					t.Errorf("Shard(%d) returned nil", i)
				}
			}
		})
	}
}

func TestShardedChan_Shard_OutOfBounds(t *testing.T) {
	s := NewShardedChan(4)
	defer s.Close()

	if shard := s.Shard(-1); shard != nil {
		t.Error("Shard(-1) should return nil")
	}

	if shard := s.Shard(4); shard != nil {
		t.Error("Shard(4) should return nil for 4-shard instance")
	}

	if shard := s.Shard(100); shard != nil {
		t.Error("Shard(100) should return nil")
	}
}

func TestShardedChan_Send(t *testing.T) {
	s := NewShardedChan(4)
	defer s.Close()

	event := Event{
		Key:       "test/key",
		Operation: "set",
	}

	// Send should not block (buffered channel)
	s.Send(event)

	// Verify event was received on the correct shard
	shardIdx := s.ShardFor(event.Key)
	select {
	case received := <-s.Shard(shardIdx):
		if received.Key != event.Key {
			t.Errorf("received Key = %s, want %s", received.Key, event.Key)
		}
		if received.Operation != event.Operation {
			t.Errorf("received Operation = %s, want %s", received.Operation, event.Operation)
		}
	default:
		t.Error("expected event on shard, got none")
	}
}

func TestShardedChan_KeyConsistency(t *testing.T) {
	s := NewShardedChan(8)
	defer s.Close()

	// Same key should always go to the same shard
	key := "consistent/key/path"
	shard1 := s.ShardFor(key)
	shard2 := s.ShardFor(key)
	shard3 := s.ShardFor(key)

	if shard1 != shard2 || shard2 != shard3 {
		t.Errorf("same key mapped to different shards: %d, %d, %d", shard1, shard2, shard3)
	}
}

func TestShardedChan_KeyDistribution(t *testing.T) {
	s := NewShardedChan(8)
	defer s.Close()

	// Test that different keys distribute across shards
	shardHits := make(map[int]int)
	keys := []string{
		"users/1", "users/2", "users/3", "users/4",
		"items/a", "items/b", "items/c", "items/d",
		"data/x", "data/y", "data/z", "data/w",
		"foo", "bar", "baz", "qux",
	}

	for _, key := range keys {
		shard := s.ShardFor(key)
		shardHits[shard]++
	}

	// With 16 keys and 8 shards, we expect some distribution
	// At minimum, more than 1 shard should be hit
	if len(shardHits) < 2 {
		t.Errorf("poor key distribution: only %d shards used for %d keys", len(shardHits), len(keys))
	}
}

func TestShardedChan_ConcurrentSend(t *testing.T) {
	s := NewShardedChan(4)

	var wg sync.WaitGroup
	eventCount := 100

	// Send events concurrently
	wg.Add(eventCount)
	for i := range eventCount {
		go func(idx int) {
			defer wg.Done()
			s.Send(Event{
				Key:       "key/" + string(rune('a'+idx%26)),
				Operation: "set",
			})
		}(i)
	}
	wg.Wait()

	// Drain all shards and count events
	s.Close()

	received := 0
	for i := 0; i < s.Count(); i++ {
		for range s.Shard(i) {
			received++
		}
	}

	if received != eventCount {
		t.Errorf("received %d events, want %d", received, eventCount)
	}
}

func TestShardedChan_Close(t *testing.T) {
	s := NewShardedChan(4)
	s.Close()

	// After close, channels should be closed
	for i := 0; i < s.Count(); i++ {
		shard := s.Shard(i)
		_, ok := <-shard
		if ok {
			t.Errorf("shard %d should be closed", i)
		}
	}
}

func TestShardedChan_SingleShard(t *testing.T) {
	s := NewShardedChan(1)
	defer s.Close()

	// All keys should go to shard 0
	keys := []string{"a", "b", "c", "different/path", "another/one"}
	for _, key := range keys {
		if shard := s.ShardFor(key); shard != 0 {
			t.Errorf("shardFor(%q) = %d, want 0 for single-shard instance", key, shard)
		}
	}
}

func BenchmarkShardedChan_Send(b *testing.B) {
	s := NewShardedChan(8)
	event := Event{Key: "bench/key", Operation: "set"}

	// Drain in background
	done := make(chan struct{})
	go func() {
		for i := 0; i < s.Count(); i++ {
			go func(ch StorageChan) {
				for range ch {
				}
			}(s.Shard(i))
		}
		<-done
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Send(event)
	}
	b.StopTimer()

	s.Close()
	close(done)
}

func BenchmarkShardedChan_ShardFor(b *testing.B) {
	s := NewShardedChan(8)
	defer s.Close()

	key := "benchmark/test/key/path"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.ShardFor(key)
	}
}
