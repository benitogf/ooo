package stream

import (
	"strconv"
	"testing"

	"github.com/benitogf/coat"
)

const benchDomain = "http://example.com"

// =============================================================================
// Patch Pool Benchmarks
// =============================================================================

// BenchmarkPatchPool benchmarks the patchPool function which creates JSON patches
func BenchmarkPatchPool(b *testing.B) {
	stream := &Stream{
		Console:   coat.NewConsole(benchDomain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
	}

	// Simulate a list with objects
	oldData := []byte(`[{"id":"1","name":"Alice"},{"id":"2","name":"Bob"}]`)
	newData := []byte(`[{"id":"1","name":"Alice"},{"id":"2","name":"Bob"},{"id":"3","name":"Charlie"}]`)

	pool := &Pool{
		Key: "users/*",
		cache: Cache{
			Data:    oldData,
			Version: 1,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.cache.Data = oldData
		_, _, _ = stream.patchPool(pool, newData)
	}
}

// BenchmarkPatchPoolAppend benchmarks patchPool with append-only optimization (now always enabled in jsonpatch)
func BenchmarkPatchPoolAppend(b *testing.B) {
	stream := &Stream{
		Console:   coat.NewConsole(benchDomain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
	}

	// Simulate a list with objects - append case
	oldData := []byte(`[{"id":"1","name":"Alice"},{"id":"2","name":"Bob"}]`)
	newData := []byte(`[{"id":"1","name":"Alice"},{"id":"2","name":"Bob"},{"id":"3","name":"Charlie"}]`)

	pool := &Pool{
		Key: "users/*",
		cache: Cache{
			Data:    oldData,
			Version: 1,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.cache.Data = oldData
		_, _, _ = stream.patchPool(pool, newData)
	}
}

// BenchmarkPatchPoolSnapshot benchmarks patchPool when snapshot is forced (NoPatch=true)
func BenchmarkPatchPoolSnapshot(b *testing.B) {
	stream := &Stream{
		Console:   coat.NewConsole(benchDomain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
		NoPatch:   true,
	}

	oldData := []byte(`[{"id":"1","name":"Alice"},{"id":"2","name":"Bob"}]`)
	newData := []byte(`[{"id":"1","name":"Alice"},{"id":"2","name":"Bob"},{"id":"3","name":"Charlie"}]`)

	pool := &Pool{
		Key: "users/*",
		cache: Cache{
			Data:    oldData,
			Version: 1,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.cache.Data = oldData
		_, _, _ = stream.patchPool(pool, newData)
	}
}

// BenchmarkPatchPoolUpdateSingleItem benchmarks patching when a single item in a list is updated
func BenchmarkPatchPoolUpdateSingleItem(b *testing.B) {
	stream := &Stream{
		Console:   coat.NewConsole(benchDomain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
	}

	// List with 10 items, update one item's field
	oldData := []byte(`[{"id":"1","name":"Alice","status":"active"},{"id":"2","name":"Bob","status":"active"},{"id":"3","name":"Charlie","status":"active"},{"id":"4","name":"David","status":"active"},{"id":"5","name":"Eve","status":"active"},{"id":"6","name":"Frank","status":"active"},{"id":"7","name":"Grace","status":"active"},{"id":"8","name":"Henry","status":"active"},{"id":"9","name":"Ivy","status":"active"},{"id":"10","name":"Jack","status":"active"}]`)
	newData := []byte(`[{"id":"1","name":"Alice","status":"active"},{"id":"2","name":"Bob","status":"active"},{"id":"3","name":"Charlie","status":"active"},{"id":"4","name":"David","status":"active"},{"id":"5","name":"Eve","status":"inactive"},{"id":"6","name":"Frank","status":"active"},{"id":"7","name":"Grace","status":"active"},{"id":"8","name":"Henry","status":"active"},{"id":"9","name":"Ivy","status":"active"},{"id":"10","name":"Jack","status":"active"}]`)

	pool := &Pool{
		Key: "users/*",
		cache: Cache{
			Data:    oldData,
			Version: 1,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.cache.Data = oldData
		_, _, _ = stream.patchPool(pool, newData)
	}
}

// BenchmarkPatchPoolUpdateMultipleItems benchmarks patching when multiple items in a list are updated
func BenchmarkPatchPoolUpdateMultipleItems(b *testing.B) {
	stream := &Stream{
		Console:   coat.NewConsole(benchDomain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
	}

	// List with 10 items, update 3 items
	oldData := []byte(`[{"id":"1","name":"Alice","status":"active"},{"id":"2","name":"Bob","status":"active"},{"id":"3","name":"Charlie","status":"active"},{"id":"4","name":"David","status":"active"},{"id":"5","name":"Eve","status":"active"},{"id":"6","name":"Frank","status":"active"},{"id":"7","name":"Grace","status":"active"},{"id":"8","name":"Henry","status":"active"},{"id":"9","name":"Ivy","status":"active"},{"id":"10","name":"Jack","status":"active"}]`)
	newData := []byte(`[{"id":"1","name":"Alice","status":"inactive"},{"id":"2","name":"Bob","status":"active"},{"id":"3","name":"Charlie","status":"active"},{"id":"4","name":"David","status":"active"},{"id":"5","name":"Eve","status":"inactive"},{"id":"6","name":"Frank","status":"active"},{"id":"7","name":"Grace","status":"active"},{"id":"8","name":"Henry","status":"active"},{"id":"9","name":"Ivy","status":"inactive"},{"id":"10","name":"Jack","status":"active"}]`)

	pool := &Pool{
		Key: "users/*",
		cache: Cache{
			Data:    oldData,
			Version: 1,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.cache.Data = oldData
		_, _, _ = stream.patchPool(pool, newData)
	}
}

// BenchmarkPatchPoolRemoveItem benchmarks patching when an item is removed from a list
func BenchmarkPatchPoolRemoveItem(b *testing.B) {
	stream := &Stream{
		Console:   coat.NewConsole(benchDomain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
	}

	// List with 5 items, remove one
	oldData := []byte(`[{"id":"1","name":"Alice"},{"id":"2","name":"Bob"},{"id":"3","name":"Charlie"},{"id":"4","name":"David"},{"id":"5","name":"Eve"}]`)
	newData := []byte(`[{"id":"1","name":"Alice"},{"id":"2","name":"Bob"},{"id":"4","name":"David"},{"id":"5","name":"Eve"}]`)

	pool := &Pool{
		Key: "users/*",
		cache: Cache{
			Data:    oldData,
			Version: 1,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.cache.Data = oldData
		_, _, _ = stream.patchPool(pool, newData)
	}
}

// BenchmarkPatchPoolLargeList benchmarks patching a large list with a single update
func BenchmarkPatchPoolLargeList(b *testing.B) {
	stream := &Stream{
		Console:   coat.NewConsole(benchDomain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
	}

	// Build a list with 100 items
	oldItems := make([]byte, 0, 10000)
	newItems := make([]byte, 0, 10000)
	oldItems = append(oldItems, '[')
	newItems = append(newItems, '[')

	for i := 0; i < 100; i++ {
		if i > 0 {
			oldItems = append(oldItems, ',')
			newItems = append(newItems, ',')
		}
		item := `{"id":"` + strconv.Itoa(i) + `","name":"User` + strconv.Itoa(i) + `","status":"active"}`
		oldItems = append(oldItems, item...)
		if i == 50 {
			// Update item 50
			newItems = append(newItems, `{"id":"50","name":"User50","status":"inactive"}`...)
		} else {
			newItems = append(newItems, item...)
		}
	}
	oldItems = append(oldItems, ']')
	newItems = append(newItems, ']')

	pool := &Pool{
		Key: "users/*",
		cache: Cache{
			Data:    oldItems,
			Version: 1,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.cache.Data = oldItems
		_, _, _ = stream.patchPool(pool, newItems)
	}
}

// BenchmarkPatchPoolLargeListAppend benchmarks appending to a large list
func BenchmarkPatchPoolLargeListAppend(b *testing.B) {
	stream := &Stream{
		Console:   coat.NewConsole(benchDomain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
	}

	// Build a list with 100 items
	oldItems := make([]byte, 0, 10000)
	oldItems = append(oldItems, '[')
	for i := 0; i < 100; i++ {
		if i > 0 {
			oldItems = append(oldItems, ',')
		}
		item := `{"id":"` + strconv.Itoa(i) + `","name":"User` + strconv.Itoa(i) + `"}`
		oldItems = append(oldItems, item...)
	}
	oldItems = append(oldItems, ']')

	// New list with 101 items (append one)
	newItems := make([]byte, len(oldItems)+50)
	copy(newItems, oldItems[:len(oldItems)-1]) // Copy without closing bracket
	newItems = newItems[:len(oldItems)-1]
	newItems = append(newItems, `,{"id":"100","name":"User100"}]`...)

	pool := &Pool{
		Key: "users/*",
		cache: Cache{
			Data:    oldItems,
			Version: 1,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.cache.Data = oldItems
		_, _, _ = stream.patchPool(pool, newItems)
	}
}

// BenchmarkPatchPoolNestedObjects benchmarks patching nested objects in a list
func BenchmarkPatchPoolNestedObjects(b *testing.B) {
	stream := &Stream{
		Console:   coat.NewConsole(benchDomain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
	}

	oldData := []byte(`[{"id":"1","profile":{"name":"Alice","settings":{"theme":"dark","notifications":true}}},{"id":"2","profile":{"name":"Bob","settings":{"theme":"light","notifications":false}}}]`)
	newData := []byte(`[{"id":"1","profile":{"name":"Alice","settings":{"theme":"light","notifications":true}}},{"id":"2","profile":{"name":"Bob","settings":{"theme":"light","notifications":false}}}]`)

	pool := &Pool{
		Key: "users/*",
		cache: Cache{
			Data:    oldData,
			Version: 1,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.cache.Data = oldData
		_, _, _ = stream.patchPool(pool, newData)
	}
}

// =============================================================================
// Cache Benchmarks
// =============================================================================

// BenchmarkSetCacheNew benchmarks setting cache for a new key
func BenchmarkSetCacheNew(b *testing.B) {
	stream := &Stream{
		Console:   coat.NewConsole(benchDomain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
	}

	data := []byte(`{"id":"1","name":"test"}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := "test/" + strconv.Itoa(i)
		_ = stream.setCache(key, data)
	}
}

// BenchmarkSetCacheExisting benchmarks updating cache for an existing key
func BenchmarkSetCacheExisting(b *testing.B) {
	stream := &Stream{
		Console:   coat.NewConsole(benchDomain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
	}

	key := "test/existing"
	data := []byte(`{"id":"1","name":"test"}`)
	stream.setCache(key, data)

	newData := []byte(`{"id":"1","name":"updated"}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = stream.setCache(key, newData)
	}
}

// BenchmarkGetCacheVersion benchmarks getting cache version
func BenchmarkGetCacheVersion(b *testing.B) {
	stream := &Stream{
		Console:   coat.NewConsole(benchDomain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
	}

	key := "test/version"
	data := []byte(`{"id":"1","name":"test"}`)
	stream.setCache(key, data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = stream.GetCacheVersion(key)
	}
}

// =============================================================================
// Broadcast Benchmarks
// =============================================================================

// BenchmarkBroadcastSinglePool benchmarks broadcasting to a single pool
func BenchmarkBroadcastSinglePool(b *testing.B) {
	stream := &Stream{
		Console:   coat.NewConsole(benchDomain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
		NoPatch:   true, // Use snapshot to isolate broadcast overhead
		OnSubscribe: func(key string) error {
			return nil
		},
		OnUnsubscribe: func(key string) {},
	}

	// Create a pool without actual websocket connections
	pool := &Pool{
		Key:         "test/single",
		connections: []*Conn{}, // Empty connections to avoid websocket writes
		cache: Cache{
			Data:    []byte(`{"id":"1"}`),
			Version: 1,
		},
	}
	stream.pools["test/single"] = pool
	stream.poolIndex.insert("test/single", pool)

	data := []byte(`{"id":"1","updated":true}`)
	getFn := func(key string) ([]byte, error) {
		return data, nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stream.Broadcast("test/single", BroadcastOpt{Get: getFn})
	}
}

// BenchmarkBroadcastWildcard benchmarks broadcasting with wildcard matching
func BenchmarkBroadcastWildcard(b *testing.B) {
	stream := &Stream{
		Console:   coat.NewConsole(benchDomain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
		NoPatch:   true,
		OnSubscribe: func(key string) error {
			return nil
		},
		OnUnsubscribe: func(key string) {},
	}

	// Create 100 pools under users/*
	for i := 0; i < 100; i++ {
		key := "users/" + strconv.Itoa(i)
		pool := &Pool{
			Key:         key,
			connections: []*Conn{},
			cache: Cache{
				Data:    []byte(`{"id":"` + strconv.Itoa(i) + `"}`),
				Version: 1,
			},
		}
		stream.pools[key] = pool
		stream.poolIndex.insert(key, pool)
	}

	// Also add a wildcard pool
	wildcardPool := &Pool{
		Key:         "users/*",
		connections: []*Conn{},
		cache: Cache{
			Data:    []byte(`[]`),
			Version: 1,
		},
	}
	stream.pools["users/*"] = wildcardPool
	stream.poolIndex.insert("users/*", wildcardPool)

	data := []byte(`{"id":"50","updated":true}`)
	getFn := func(key string) ([]byte, error) {
		return data, nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Broadcast to a specific user - should match both exact and wildcard pool
		stream.Broadcast("users/50", BroadcastOpt{Get: getFn})
	}
}

// BenchmarkBroadcastToWildcard benchmarks broadcasting with a wildcard path
func BenchmarkBroadcastToWildcard(b *testing.B) {
	stream := &Stream{
		Console:   coat.NewConsole(benchDomain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
		NoPatch:   true,
		OnSubscribe: func(key string) error {
			return nil
		},
		OnUnsubscribe: func(key string) {},
	}

	// Create 100 pools under users/*
	for i := 0; i < 100; i++ {
		key := "users/" + strconv.Itoa(i)
		pool := &Pool{
			Key:         key,
			connections: []*Conn{},
			cache: Cache{
				Data:    []byte(`{"id":"` + strconv.Itoa(i) + `"}`),
				Version: 1,
			},
		}
		stream.pools[key] = pool
		stream.poolIndex.insert(key, pool)
	}

	data := []byte(`[{"id":"1"},{"id":"2"}]`)
	getFn := func(key string) ([]byte, error) {
		return data, nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Broadcast with wildcard - should match all 100 pools
		stream.Broadcast("users/*", BroadcastOpt{Get: getFn})
	}
}

// BenchmarkBroadcastWithPatch benchmarks broadcasting with JSON patch generation
func BenchmarkBroadcastWithPatch(b *testing.B) {
	stream := &Stream{
		Console:   coat.NewConsole(benchDomain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
		OnSubscribe: func(key string) error {
			return nil
		},
		OnUnsubscribe: func(key string) {},
	}

	// Create a pool with initial data
	pool := &Pool{
		Key:         "users/*",
		connections: []*Conn{},
		cache: Cache{
			Data:    []byte(`[{"id":"1","name":"Alice"},{"id":"2","name":"Bob"}]`),
			Version: 1,
		},
	}
	stream.pools["users/*"] = pool
	stream.poolIndex.insert("users/*", pool)

	// Simulate updating one item in the list
	newData := []byte(`[{"id":"1","name":"Alice Updated"},{"id":"2","name":"Bob"}]`)
	getFn := func(key string) ([]byte, error) {
		return newData, nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Reset cache to original state
		pool.cache.Data = []byte(`[{"id":"1","name":"Alice"},{"id":"2","name":"Bob"}]`)
		stream.Broadcast("users/*", BroadcastOpt{Get: getFn})
	}
}

// BenchmarkBroadcastListAppend benchmarks broadcasting when appending to a list
func BenchmarkBroadcastListAppend(b *testing.B) {
	stream := &Stream{
		Console:   coat.NewConsole(benchDomain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
		OnSubscribe: func(key string) error {
			return nil
		},
		OnUnsubscribe: func(key string) {},
	}

	oldData := []byte(`[{"id":"1","name":"Alice"},{"id":"2","name":"Bob"}]`)
	pool := &Pool{
		Key:         "users/*",
		connections: []*Conn{},
		cache: Cache{
			Data:    oldData,
			Version: 1,
		},
	}
	stream.pools["users/*"] = pool
	stream.poolIndex.insert("users/*", pool)

	// Append a new item
	newData := []byte(`[{"id":"1","name":"Alice"},{"id":"2","name":"Bob"},{"id":"3","name":"Charlie"}]`)
	getFn := func(key string) ([]byte, error) {
		return newData, nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.cache.Data = oldData
		stream.Broadcast("users/*", BroadcastOpt{Get: getFn})
	}
}

// =============================================================================
// Connection Benchmarks
// =============================================================================

// BenchmarkNewConnection benchmarks creating a new stream connection
func BenchmarkNewConnection(b *testing.B) {
	stream := &Stream{
		Console:   coat.NewConsole(benchDomain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
		OnSubscribe: func(key string) error {
			return nil
		},
		OnUnsubscribe: func(key string) {},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := "test/" + strconv.Itoa(i%1000) // Cycle through 1000 keys
		// Directly call internal new() to avoid websocket upgrade
		stream.new(key, nil)
	}
}

// BenchmarkNewConnectionPreallocated benchmarks adding connection to pre-allocated pools
func BenchmarkNewConnectionPreallocated(b *testing.B) {
	stream := &Stream{
		Console:   coat.NewConsole(benchDomain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
		OnSubscribe: func(key string) error {
			return nil
		},
		OnUnsubscribe: func(key string) {},
	}

	// Pre-allocate 1000 pools
	paths := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		paths[i] = "test/" + strconv.Itoa(i)
	}
	stream.PreallocatePools(paths)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := "test/" + strconv.Itoa(i%1000)
		stream.new(key, nil)
	}
}

// BenchmarkNewConnectionExistingPool benchmarks adding connection to existing pool
func BenchmarkNewConnectionExistingPool(b *testing.B) {
	stream := &Stream{
		Console:   coat.NewConsole(benchDomain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
		OnSubscribe: func(key string) error {
			return nil
		},
		OnUnsubscribe: func(key string) {},
	}

	// Create the pool first
	key := "test/existing"
	stream.new(key, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stream.new(key, nil)
	}
}

// =============================================================================
// Refresh Benchmarks
// =============================================================================

// BenchmarkRefresh benchmarks the Refresh function
func BenchmarkRefresh(b *testing.B) {
	stream := &Stream{
		Console:   coat.NewConsole(benchDomain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
	}

	data := []byte(`{"id":"1","name":"test"}`)
	getFn := func(key string) ([]byte, error) {
		return data, nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := "test/" + strconv.Itoa(i%100) // Cycle through 100 keys
		_, _ = stream.Refresh(key, getFn)
	}
}

// BenchmarkRefreshExisting benchmarks Refresh for existing cache
func BenchmarkRefreshExisting(b *testing.B) {
	stream := &Stream{
		Console:   coat.NewConsole(benchDomain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
	}

	key := "test/existing"
	data := []byte(`{"id":"1","name":"test"}`)
	getFn := func(key string) ([]byte, error) {
		return data, nil
	}

	// Prime the cache
	stream.setCache(key, data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = stream.Refresh(key, getFn)
	}
}

// =============================================================================
// Concurrent Benchmarks
// =============================================================================

// BenchmarkConcurrentBroadcast benchmarks concurrent broadcasts to different pools
func BenchmarkConcurrentBroadcast(b *testing.B) {
	stream := &Stream{
		Console:   coat.NewConsole(benchDomain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
		NoPatch:   true,
		OnSubscribe: func(key string) error {
			return nil
		},
		OnUnsubscribe: func(key string) {},
	}

	// Create 10 pools
	for i := 0; i < 10; i++ {
		key := "pool/" + strconv.Itoa(i)
		pool := &Pool{
			Key:         key,
			connections: []*Conn{},
			cache: Cache{
				Data:    []byte(`{"id":"` + strconv.Itoa(i) + `"}`),
				Version: 1,
			},
		}
		stream.pools[key] = pool
		stream.poolIndex.insert(key, pool)
	}

	data := []byte(`{"updated":true}`)
	getFn := func(key string) ([]byte, error) {
		return data, nil
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := "pool/" + strconv.Itoa(i%10)
			stream.Broadcast(key, BroadcastOpt{Get: getFn})
			i++
		}
	})
}

// BenchmarkConcurrentBroadcastWithPatch benchmarks concurrent broadcasts with patch generation
func BenchmarkConcurrentBroadcastWithPatch(b *testing.B) {
	stream := &Stream{
		Console:   coat.NewConsole(benchDomain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
		OnSubscribe: func(key string) error {
			return nil
		},
		OnUnsubscribe: func(key string) {},
	}

	// Create 10 pools with list data
	for i := 0; i < 10; i++ {
		key := "pool/" + strconv.Itoa(i)
		pool := &Pool{
			Key:         key,
			connections: []*Conn{},
			cache: Cache{
				Data:    []byte(`[{"id":"1"},{"id":"2"}]`),
				Version: 1,
			},
		}
		stream.pools[key] = pool
		stream.poolIndex.insert(key, pool)
	}

	newData := []byte(`[{"id":"1"},{"id":"2"},{"id":"3"}]`)
	getFn := func(key string) ([]byte, error) {
		return newData, nil
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := "pool/" + strconv.Itoa(i%10)
			stream.Broadcast(key, BroadcastOpt{Get: getFn})
			i++
		}
	})
}

// =============================================================================
// Build Message Benchmarks
// =============================================================================

// BenchmarkBuildMessage benchmarks the buildMessage function
func BenchmarkBuildMessage(b *testing.B) {
	data := []byte(`{"id":"1","name":"test","nested":{"key":"value"}}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildMessage(data, true, int64(i))
	}
}

// BenchmarkBuildMessageLarge benchmarks buildMessage with large data
func BenchmarkBuildMessageLarge(b *testing.B) {
	// Build a large JSON array
	data := make([]byte, 0, 10000)
	data = append(data, '[')
	for i := 0; i < 100; i++ {
		if i > 0 {
			data = append(data, ',')
		}
		data = append(data, `{"id":"`...)
		data = strconv.AppendInt(data, int64(i), 10)
		data = append(data, `","name":"User`...)
		data = strconv.AppendInt(data, int64(i), 10)
		data = append(data, `"}`...)
	}
	data = append(data, ']')

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildMessage(data, false, int64(i))
	}
}
