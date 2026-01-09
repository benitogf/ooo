package stream

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/benitogf/ooo/meta"
)

// =============================================================================
// ProcessBroadcast Benchmarks
// =============================================================================

func BenchmarkProcessBroadcast_ObjectSet(b *testing.B) {
	obj := &meta.Object{
		Path:    "test/1",
		Created: 100,
		Data:    json.RawMessage(`"data"`),
	}

	b.ResetTimer()
	for b.Loop() {
		cache := &Cache{}
		ProcessBroadcast(cache, "test/1", "set", obj, noopFilterObject, noopFilterList, false)
	}
}

func BenchmarkProcessBroadcast_ObjectUpdate(b *testing.B) {
	obj := &meta.Object{
		Path:    "test/1",
		Created: 100,
		Updated: 200,
		Data:    json.RawMessage(`"new data"`),
	}

	b.ResetTimer()
	for b.Loop() {
		oldObj := &meta.Object{
			Path:    "test/1",
			Created: 100,
			Updated: 100,
			Data:    json.RawMessage(`"old data"`),
		}
		cache := &Cache{Object: oldObj}
		ProcessBroadcast(cache, "test/1", "set", obj, noopFilterObject, noopFilterList, false)
	}
}

func BenchmarkProcessBroadcast_ObjectDel(b *testing.B) {
	b.ResetTimer()
	for range b.N {
		oldObj := &meta.Object{
			Path:    "test/1",
			Created: 100,
			Data:    json.RawMessage(`"data"`),
		}
		cache := &Cache{Object: oldObj}
		ProcessBroadcast(cache, "test/1", "del", nil, noopFilterObject, noopFilterList, false)
	}
}

func BenchmarkProcessBroadcast_ListAdd(b *testing.B) {
	obj := &meta.Object{
		Path:    "items/1",
		Created: 100,
		Data:    json.RawMessage(`"data"`),
	}

	b.ResetTimer()
	for b.Loop() {
		cache := &Cache{Objects: []meta.Object{}}
		ProcessBroadcast(cache, "items/*", "set", obj, noopFilterObject, noopFilterList, false)
	}
}

func BenchmarkProcessBroadcast_ListUpdate(b *testing.B) {
	obj := &meta.Object{
		Path:    "items/5",
		Created: 5000,
		Updated: 6000,
		Data:    json.RawMessage(`"updated"`),
	}

	baseObjects := make([]meta.Object, 10)
	for i := range 10 {
		baseObjects[i] = meta.Object{
			Path:    "items/" + strconv.Itoa(i),
			Created: int64(i * 1000),
			Data:    json.RawMessage(`"item` + strconv.Itoa(i) + `"`),
		}
	}

	b.ResetTimer()
	for b.Loop() {
		objects := make([]meta.Object, len(baseObjects))
		copy(objects, baseObjects)
		cache := &Cache{Objects: objects}
		ProcessBroadcast(cache, "items/*", "set", obj, noopFilterObject, noopFilterList, false)
	}
}

func BenchmarkProcessBroadcast_ListRemove(b *testing.B) {
	obj := &meta.Object{Path: "items/5"}

	baseObjects := make([]meta.Object, 10)
	for i := range 10 {
		baseObjects[i] = meta.Object{
			Path:    "items/" + strconv.Itoa(i),
			Created: int64(i * 1000),
		}
	}

	b.ResetTimer()
	for b.Loop() {
		objects := make([]meta.Object, len(baseObjects))
		copy(objects, baseObjects)
		cache := &Cache{Objects: objects}
		ProcessBroadcast(cache, "items/*", "del", obj, noopFilterObject, noopFilterList, false)
	}
}

func BenchmarkProcessBroadcast_LargeList(b *testing.B) {
	obj := &meta.Object{
		Path:    "items/500",
		Created: 500000,
		Updated: 600000,
		Data:    json.RawMessage(`"updated"`),
	}

	baseObjects := make([]meta.Object, 1000)
	for i := range 1000 {
		baseObjects[i] = meta.Object{
			Path:    "items/" + strconv.Itoa(i),
			Created: int64(i * 1000),
			Data:    json.RawMessage(`"item` + strconv.Itoa(i) + `"`),
		}
	}

	b.ResetTimer()
	for b.Loop() {
		objects := make([]meta.Object, len(baseObjects))
		copy(objects, baseObjects)
		cache := &Cache{Objects: objects}
		ProcessBroadcast(cache, "items/*", "set", obj, noopFilterObject, noopFilterList, false)
	}
}

// =============================================================================
// List Manipulation Benchmarks
// =============================================================================

func BenchmarkInsertSorted(b *testing.B) {
	baseList := make([]meta.Object, 100)
	for i := range 100 {
		baseList[i] = meta.Object{
			Created: int64(i * 1000),
			Path:    "users/" + strconv.Itoa(i),
		}
	}

	newObj := meta.Object{Created: 50500, Path: "users/new"}

	b.ResetTimer()
	for b.Loop() {
		list := make([]meta.Object, len(baseList))
		copy(list, baseList)
		_, _ = insertSorted(list, newObj)
	}
}

func BenchmarkInsertSortedAppend(b *testing.B) {
	baseList := make([]meta.Object, 100)
	for i := range 100 {
		baseList[i] = meta.Object{
			Created: int64(i * 1000),
			Path:    "users/" + strconv.Itoa(i),
		}
	}

	newObj := meta.Object{Created: 200000, Path: "users/new"}

	b.ResetTimer()
	for b.Loop() {
		list := make([]meta.Object, len(baseList))
		copy(list, baseList)
		_, _ = insertSorted(list, newObj)
	}
}

func BenchmarkInsertSortedLargeList(b *testing.B) {
	baseList := make([]meta.Object, 1000)
	for i := range 1000 {
		baseList[i] = meta.Object{
			Created: int64(i * 1000),
			Path:    "users/" + strconv.Itoa(i),
		}
	}

	newObj := meta.Object{Created: 500500, Path: "users/new"}

	b.ResetTimer()
	for b.Loop() {
		list := make([]meta.Object, len(baseList))
		copy(list, baseList)
		_, _ = insertSorted(list, newObj)
	}
}

func BenchmarkUpdateInList(b *testing.B) {
	baseList := make([]meta.Object, 100)
	for i := range 100 {
		baseList[i] = meta.Object{
			Created: int64(i * 1000),
			Path:    "users/" + strconv.Itoa(i),
		}
	}

	updatedObj := meta.Object{Path: "users/50", Created: 50000}

	b.ResetTimer()
	for b.Loop() {
		list := make([]meta.Object, len(baseList))
		copy(list, baseList)
		_, _, _ = updateInList(list, updatedObj)
	}
}

func BenchmarkUpdateInListLarge(b *testing.B) {
	baseList := make([]meta.Object, 1000)
	for i := range 1000 {
		baseList[i] = meta.Object{
			Created: int64(i * 1000),
			Path:    "users/" + strconv.Itoa(i),
		}
	}

	updatedObj := meta.Object{Path: "users/500", Created: 500000}

	b.ResetTimer()
	for b.Loop() {
		list := make([]meta.Object, len(baseList))
		copy(list, baseList)
		_, _, _ = updateInList(list, updatedObj)
	}
}

func BenchmarkRemoveFromList(b *testing.B) {
	baseList := make([]meta.Object, 100)
	for i := range 100 {
		baseList[i] = meta.Object{
			Created: int64(i * 1000),
			Path:    "users/" + strconv.Itoa(i),
		}
	}

	objToRemove := meta.Object{Path: "users/50"}

	b.ResetTimer()
	for b.Loop() {
		list := make([]meta.Object, len(baseList))
		copy(list, baseList)
		_, _, _ = removeFromList(list, objToRemove)
	}
}

func BenchmarkRemoveFromListLarge(b *testing.B) {
	baseList := make([]meta.Object, 1000)
	for i := range 1000 {
		baseList[i] = meta.Object{
			Created: int64(i * 1000),
			Path:    "users/" + strconv.Itoa(i),
		}
	}

	objToRemove := meta.Object{Path: "users/500"}

	b.ResetTimer()
	for b.Loop() {
		list := make([]meta.Object, len(baseList))
		copy(list, baseList)
		_, _, _ = removeFromList(list, objToRemove)
	}
}

// =============================================================================
// Patch Generation Benchmarks
// =============================================================================

func BenchmarkGenerateListPatchAdd(b *testing.B) {
	obj := meta.Object{
		Created: 1000,
		Path:    "users/new",
		Data:    json.RawMessage(`{"id":"new"}`),
	}

	b.ResetTimer()
	for b.Loop() {
		_, _ = generateListPatch("add", 5, &obj)
	}
}

func BenchmarkGenerateListPatchReplace(b *testing.B) {
	obj := meta.Object{
		Created: 1000,
		Path:    "users/50",
		Data:    json.RawMessage(`{"id":"50"}`),
	}

	b.ResetTimer()
	for b.Loop() {
		_, _ = generateListPatch("replace", 50, &obj)
	}
}

func BenchmarkGenerateListPatchRemove(b *testing.B) {
	for b.Loop() {
		_, _ = generateListPatch("remove", 50, nil)
	}
}

func BenchmarkGenerateObjectPatch(b *testing.B) {
	oldObj := &meta.Object{
		Created: 1000,
		Updated: 1000,
		Path:    "users/user1",
		Data:    json.RawMessage(`{"id":"user1","status":"active"}`),
	}
	newObj := &meta.Object{
		Created: 1000,
		Updated: 2000,
		Path:    "users/user1",
		Data:    json.RawMessage(`{"id":"user1","status":"inactive"}`),
	}

	b.ResetTimer()
	for b.Loop() {
		_, _ = generateObjectPatch(oldObj, newObj)
	}
}

func BenchmarkGenerateObjectPatchLarge(b *testing.B) {
	largeData := `{"id":"user1","profile":{"name":"Alice","email":"alice@example.com","settings":{"theme":"dark","notifications":true,"language":"en","timezone":"UTC"}},"metadata":{"created":"2024-01-01","updated":"2024-01-02","version":1}}`
	oldObj := &meta.Object{
		Created: 1000,
		Updated: 1000,
		Path:    "users/user1",
		Data:    json.RawMessage(largeData),
	}

	updatedData := `{"id":"user1","profile":{"name":"Alice","email":"alice@example.com","settings":{"theme":"light","notifications":true,"language":"en","timezone":"UTC"}},"metadata":{"created":"2024-01-01","updated":"2024-01-02","version":2}}`
	newObj := &meta.Object{
		Created: 1000,
		Updated: 2000,
		Path:    "users/user1",
		Data:    json.RawMessage(updatedData),
	}

	b.ResetTimer()
	for b.Loop() {
		_, _ = generateObjectPatch(oldObj, newObj)
	}
}

func BenchmarkGenerateAddRemovePatch(b *testing.B) {
	obj := &meta.Object{
		Created: 1000,
		Path:    "users/new",
		Data:    json.RawMessage(`{"id":"new"}`),
	}

	b.ResetTimer()
	for b.Loop() {
		_, _ = generateAddRemovePatch(0, obj, 99)
	}
}

// =============================================================================
// Helper Function Benchmarks
// =============================================================================

func BenchmarkIsGlobKey(b *testing.B) {
	keys := []string{"users/*", "items/123", "data/*/nested", "simple"}

	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		_ = isGlobKey(keys[i%len(keys)])
	}
}

func BenchmarkFindPosition(b *testing.B) {
	list := make([]meta.Object, 100)
	for i := range 100 {
		list[i] = meta.Object{Path: "items/" + strconv.Itoa(i)}
	}

	b.ResetTimer()
	for b.Loop() {
		_ = findPosition(list, "items/50")
	}
}

func BenchmarkFindPositionLarge(b *testing.B) {
	list := make([]meta.Object, 1000)
	for i := range 1000 {
		list[i] = meta.Object{Path: "items/" + strconv.Itoa(i)}
	}

	for b.Loop() {
		_ = findPosition(list, "items/500")
	}
}
