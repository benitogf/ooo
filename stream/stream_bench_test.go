package stream

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/benitogf/coat"
	"github.com/benitogf/ooo/meta"
)

const benchDomain = "http://example.com"

// =============================================================================
// List Manipulation Benchmarks
// =============================================================================

// BenchmarkInsertSorted benchmarks inserting into a sorted list
func BenchmarkInsertSorted(b *testing.B) {
	// Create a base list of 100 objects
	baseList := make([]meta.Object, 100)
	for i := 0; i < 100; i++ {
		baseList[i] = meta.Object{
			Created: int64(i * 1000),
			Index:   strconv.Itoa(i),
			Path:    "users/" + strconv.Itoa(i),
			Data:    json.RawMessage(`{"id":"` + strconv.Itoa(i) + `"}`),
		}
	}

	newObj := meta.Object{
		Created: 50500, // Insert in the middle
		Index:   "new",
		Path:    "users/new",
		Data:    json.RawMessage(`{"id":"new"}`),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list := make([]meta.Object, len(baseList))
		copy(list, baseList)
		_, _ = insertSorted(list, newObj)
	}
}

// BenchmarkInsertSortedAppend benchmarks appending to end of sorted list
func BenchmarkInsertSortedAppend(b *testing.B) {
	baseList := make([]meta.Object, 100)
	for i := 0; i < 100; i++ {
		baseList[i] = meta.Object{
			Created: int64(i * 1000),
			Index:   strconv.Itoa(i),
			Path:    "users/" + strconv.Itoa(i),
			Data:    json.RawMessage(`{"id":"` + strconv.Itoa(i) + `"}`),
		}
	}

	newObj := meta.Object{
		Created: 200000, // Append at end
		Index:   "new",
		Path:    "users/new",
		Data:    json.RawMessage(`{"id":"new"}`),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list := make([]meta.Object, len(baseList))
		copy(list, baseList)
		_, _ = insertSorted(list, newObj)
	}
}

// BenchmarkUpdateInList benchmarks updating an item in a list
func BenchmarkUpdateInList(b *testing.B) {
	baseList := make([]meta.Object, 100)
	for i := 0; i < 100; i++ {
		baseList[i] = meta.Object{
			Created: int64(i * 1000),
			Index:   strconv.Itoa(i),
			Path:    "users/" + strconv.Itoa(i),
			Data:    json.RawMessage(`{"id":"` + strconv.Itoa(i) + `","status":"active"}`),
		}
	}

	updatedObj := meta.Object{
		Created: 50000,
		Index:   "50",
		Path:    "users/50",
		Data:    json.RawMessage(`{"id":"50","status":"inactive"}`),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list := make([]meta.Object, len(baseList))
		copy(list, baseList)
		_, _, _ = updateInList(list, updatedObj)
	}
}

// BenchmarkRemoveFromList benchmarks removing an item from a list
func BenchmarkRemoveFromList(b *testing.B) {
	baseList := make([]meta.Object, 100)
	for i := 0; i < 100; i++ {
		baseList[i] = meta.Object{
			Created: int64(i * 1000),
			Index:   strconv.Itoa(i),
			Path:    "users/" + strconv.Itoa(i),
			Data:    json.RawMessage(`{"id":"` + strconv.Itoa(i) + `"}`),
		}
	}

	objToRemove := meta.Object{
		Path: "users/50",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list := make([]meta.Object, len(baseList))
		copy(list, baseList)
		_, _, _ = removeFromList(list, objToRemove)
	}
}

// =============================================================================
// Patch Generation Benchmarks
// =============================================================================

// BenchmarkGenerateListPatchAdd benchmarks generating an add patch for lists
func BenchmarkGenerateListPatchAdd(b *testing.B) {
	obj := meta.Object{
		Created: 1000,
		Index:   "new",
		Path:    "users/new",
		Data:    json.RawMessage(`{"id":"new","name":"New User","status":"active"}`),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = generateListPatch("add", 5, &obj)
	}
}

// BenchmarkGenerateListPatchReplace benchmarks generating a replace patch for lists
func BenchmarkGenerateListPatchReplace(b *testing.B) {
	obj := meta.Object{
		Created: 1000,
		Index:   "50",
		Path:    "users/50",
		Data:    json.RawMessage(`{"id":"50","name":"Updated User","status":"inactive"}`),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = generateListPatch("replace", 50, &obj)
	}
}

// BenchmarkGenerateListPatchRemove benchmarks generating a remove patch for lists
func BenchmarkGenerateListPatchRemove(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = generateListPatch("remove", 50, nil)
	}
}

// BenchmarkGenerateObjectPatch benchmarks generating a patch for single objects
func BenchmarkGenerateObjectPatch(b *testing.B) {
	oldObj := &meta.Object{
		Created: 1000,
		Updated: 1000,
		Index:   "user1",
		Path:    "users/user1",
		Data:    json.RawMessage(`{"id":"user1","name":"Alice","status":"active"}`),
	}
	newObj := &meta.Object{
		Created: 1000,
		Updated: 2000,
		Index:   "user1",
		Path:    "users/user1",
		Data:    json.RawMessage(`{"id":"user1","name":"Alice","status":"inactive"}`),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = generateObjectPatch(oldObj, newObj)
	}
}

// BenchmarkGenerateObjectPatchLarge benchmarks patch generation for large objects
func BenchmarkGenerateObjectPatchLarge(b *testing.B) {
	// Build large nested object
	largeData := `{"id":"user1","profile":{"name":"Alice","email":"alice@example.com","settings":{"theme":"dark","notifications":true,"language":"en","timezone":"UTC"}},"metadata":{"created":"2024-01-01","updated":"2024-01-02","version":1}}`
	oldObj := &meta.Object{
		Created: 1000,
		Updated: 1000,
		Index:   "user1",
		Path:    "users/user1",
		Data:    json.RawMessage(largeData),
	}

	updatedData := `{"id":"user1","profile":{"name":"Alice","email":"alice@example.com","settings":{"theme":"light","notifications":true,"language":"en","timezone":"UTC"}},"metadata":{"created":"2024-01-01","updated":"2024-01-02","version":2}}`
	newObj := &meta.Object{
		Created: 1000,
		Updated: 2000,
		Index:   "user1",
		Path:    "users/user1",
		Data:    json.RawMessage(updatedData),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = generateObjectPatch(oldObj, newObj)
	}
}

// =============================================================================
// Cache Benchmarks
// =============================================================================

// BenchmarkInitCacheObject benchmarks initializing cache for a single object
func BenchmarkInitCacheObject(b *testing.B) {
	stream := &Stream{
		Console:   coat.NewConsole(benchDomain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
	}

	obj := &meta.Object{
		Created: 1000,
		Index:   "1",
		Path:    "test/1",
		Data:    json.RawMessage(`{"id":"1","name":"test"}`),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := "test/" + strconv.Itoa(i)
		_ = stream.InitCacheObjectWithVersion(key, obj)
	}
}

// BenchmarkInitCacheObjects benchmarks initializing cache for a list
func BenchmarkInitCacheObjects(b *testing.B) {
	stream := &Stream{
		Console:   coat.NewConsole(benchDomain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
	}

	objects := make([]meta.Object, 10)
	for i := 0; i < 10; i++ {
		objects[i] = meta.Object{
			Created: int64(i * 1000),
			Index:   strconv.Itoa(i),
			Path:    "test/" + strconv.Itoa(i),
			Data:    json.RawMessage(`{"id":"` + strconv.Itoa(i) + `"}`),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := "test/*"
		_ = stream.InitCacheObjectsWithVersion(key, objects)
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
	obj := &meta.Object{
		Created: 1000,
		Index:   "version",
		Path:    key,
		Data:    json.RawMessage(`{"id":"1","name":"test"}`),
	}
	stream.InitCacheObjectWithVersion(key, obj)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = stream.GetCacheVersion(key)
	}
}

// =============================================================================
// Broadcast Benchmarks
// =============================================================================

// BenchmarkBroadcastSinglePool benchmarks broadcasting to a single pool (object)
func BenchmarkBroadcastSinglePool(b *testing.B) {
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

	obj := &meta.Object{
		Created: 1000,
		Index:   "single",
		Path:    "test/single",
		Data:    json.RawMessage(`{"id":"1"}`),
	}

	pool := &Pool{
		Key:         "test/single",
		connections: []*Conn{},
		cache: Cache{
			Object:  obj,
			Version: 1,
		},
	}
	stream.pools["test/single"] = pool
	stream.poolIndex.insert("test/single", pool)

	newObj := &meta.Object{
		Created: 1000,
		Updated: 2000,
		Index:   "single",
		Path:    "test/single",
		Data:    json.RawMessage(`{"id":"1","updated":true}`),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stream.Broadcast("test/single", BroadcastOpt{
			Key:       "test/single",
			Operation: "set",
			Object:    newObj,
			FilterObject: func(key string, obj meta.Object) (meta.Object, error) {
				return obj, nil
			},
			FilterList: func(key string, objs []meta.Object) ([]meta.Object, error) {
				return objs, nil
			},
		})
	}
}

// BenchmarkBroadcastListAdd benchmarks broadcasting an add to a list pool
func BenchmarkBroadcastListAdd(b *testing.B) {
	stream := &Stream{
		Console:   coat.NewConsole(benchDomain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
		OnSubscribe: func(key string) error {
			return nil
		},
		OnUnsubscribe: func(key string) {},
	}

	// Create initial list
	objects := make([]meta.Object, 10)
	for i := 0; i < 10; i++ {
		objects[i] = meta.Object{
			Created: int64(i * 1000),
			Index:   strconv.Itoa(i),
			Path:    "users/" + strconv.Itoa(i),
			Data:    json.RawMessage(`{"id":"` + strconv.Itoa(i) + `"}`),
		}
	}

	pool := &Pool{
		Key:         "users/*",
		connections: []*Conn{},
		cache: Cache{
			Objects: objects,
			Version: 1,
		},
	}
	stream.pools["users/*"] = pool
	stream.poolIndex.insert("users/*", pool)

	newObj := &meta.Object{
		Created: 20000,
		Index:   "new",
		Path:    "users/new",
		Data:    json.RawMessage(`{"id":"new","name":"New User"}`),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Reset cache
		pool.cache.Objects = make([]meta.Object, len(objects))
		copy(pool.cache.Objects, objects)

		stream.Broadcast("users/new", BroadcastOpt{
			Key:       "users/new",
			Operation: "set",
			Object:    newObj,
			FilterObject: func(key string, obj meta.Object) (meta.Object, error) {
				return obj, nil
			},
			FilterList: func(key string, objs []meta.Object) ([]meta.Object, error) {
				return objs, nil
			},
		})
	}
}

// BenchmarkBroadcastListUpdate benchmarks broadcasting an update to a list pool
func BenchmarkBroadcastListUpdate(b *testing.B) {
	stream := &Stream{
		Console:   coat.NewConsole(benchDomain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
		OnSubscribe: func(key string) error {
			return nil
		},
		OnUnsubscribe: func(key string) {},
	}

	objects := make([]meta.Object, 100)
	for i := 0; i < 100; i++ {
		objects[i] = meta.Object{
			Created: int64(i * 1000),
			Index:   strconv.Itoa(i),
			Path:    "users/" + strconv.Itoa(i),
			Data:    json.RawMessage(`{"id":"` + strconv.Itoa(i) + `","status":"active"}`),
		}
	}

	pool := &Pool{
		Key:         "users/*",
		connections: []*Conn{},
		cache: Cache{
			Objects: objects,
			Version: 1,
		},
	}
	stream.pools["users/*"] = pool
	stream.poolIndex.insert("users/*", pool)

	updatedObj := &meta.Object{
		Created: 50000,
		Updated: 60000,
		Index:   "50",
		Path:    "users/50",
		Data:    json.RawMessage(`{"id":"50","status":"inactive"}`),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Reset cache
		pool.cache.Objects = make([]meta.Object, len(objects))
		copy(pool.cache.Objects, objects)

		stream.Broadcast("users/50", BroadcastOpt{
			Key:       "users/50",
			Operation: "set",
			Object:    updatedObj,
			FilterObject: func(key string, obj meta.Object) (meta.Object, error) {
				return obj, nil
			},
			FilterList: func(key string, objs []meta.Object) ([]meta.Object, error) {
				return objs, nil
			},
		})
	}
}

// BenchmarkBroadcastListRemove benchmarks broadcasting a remove from a list pool
func BenchmarkBroadcastListRemove(b *testing.B) {
	stream := &Stream{
		Console:   coat.NewConsole(benchDomain, true),
		pools:     make(map[string]*Pool),
		poolIndex: newPoolTrie(),
		OnSubscribe: func(key string) error {
			return nil
		},
		OnUnsubscribe: func(key string) {},
	}

	objects := make([]meta.Object, 100)
	for i := 0; i < 100; i++ {
		objects[i] = meta.Object{
			Created: int64(i * 1000),
			Index:   strconv.Itoa(i),
			Path:    "users/" + strconv.Itoa(i),
			Data:    json.RawMessage(`{"id":"` + strconv.Itoa(i) + `"}`),
		}
	}

	pool := &Pool{
		Key:         "users/*",
		connections: []*Conn{},
		cache: Cache{
			Objects: objects,
			Version: 1,
		},
	}
	stream.pools["users/*"] = pool
	stream.poolIndex.insert("users/*", pool)

	objToRemove := &meta.Object{
		Path: "users/50",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Reset cache
		pool.cache.Objects = make([]meta.Object, len(objects))
		copy(pool.cache.Objects, objects)

		stream.Broadcast("users/50", BroadcastOpt{
			Key:       "users/50",
			Operation: "del",
			Object:    objToRemove,
			FilterObject: func(key string, obj meta.Object) (meta.Object, error) {
				return obj, nil
			},
			FilterList: func(key string, objs []meta.Object) ([]meta.Object, error) {
				return objs, nil
			},
		})
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
		key := "test/" + strconv.Itoa(i%1000)
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

	key := "test/existing"
	stream.new(key, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stream.new(key, nil)
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

	for i := 0; i < 10; i++ {
		key := "pool/" + strconv.Itoa(i)
		obj := &meta.Object{
			Created: int64(i * 1000),
			Index:   strconv.Itoa(i),
			Path:    key,
			Data:    json.RawMessage(`{"id":"` + strconv.Itoa(i) + `"}`),
		}
		pool := &Pool{
			Key:         key,
			connections: []*Conn{},
			cache: Cache{
				Object:  obj,
				Version: 1,
			},
		}
		stream.pools[key] = pool
		stream.poolIndex.insert(key, pool)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := "pool/" + strconv.Itoa(i%10)
			obj := &meta.Object{
				Created: int64(i * 1000),
				Index:   strconv.Itoa(i % 10),
				Path:    key,
				Data:    json.RawMessage(`{"updated":true}`),
			}
			stream.Broadcast(key, BroadcastOpt{
				Key:       key,
				Operation: "set",
				Object:    obj,
				FilterObject: func(key string, obj meta.Object) (meta.Object, error) {
					return obj, nil
				},
				FilterList: func(key string, objs []meta.Object) ([]meta.Object, error) {
					return objs, nil
				},
			})
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
