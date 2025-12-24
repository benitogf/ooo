package stream

import (
	"strconv"
	"strings"
	"sync"

	"github.com/goccy/go-json"

	"github.com/benitogf/jsonpatch"
	"github.com/benitogf/ooo/meta"
)

// Pool for reusing single-operation patch slices
var patchOpPool = sync.Pool{
	New: func() any {
		return make([]jsonpatch.Operation, 1)
	},
}

// Pool for reusing two-operation patch slices (add+remove)
var patchOp2Pool = sync.Pool{
	New: func() any {
		return make([]jsonpatch.Operation, 2)
	},
}

// BroadcastResult contains the message to send after processing a broadcast event.
// This is returned by ProcessBroadcast and used by Stream to send to connections.
type BroadcastResult struct {
	Data     []byte // The data to send (patch or snapshot)
	Snapshot bool   // Whether this is a snapshot (true) or patch (false)
	Skip     bool   // Whether to skip sending (no change or filtered out)
}

// FilterObjectFn is a function type for filtering single objects.
type FilterObjectFn func(key string, obj meta.Object) (meta.Object, error)

// FilterListFn is a function type for filtering object lists.
type FilterListFn func(key string, objs []meta.Object) ([]meta.Object, error)

// isGlobKey checks if a pool key contains a glob pattern (list subscription)
func isGlobKey(key string) bool {
	return strings.Contains(key, "*")
}

// ProcessBroadcast processes a broadcast event and returns the message to send.
// It updates the cache and generates the appropriate patch or snapshot.
// noPatch forces snapshot mode (no patches).
func ProcessBroadcast(cache *Cache, poolKey string, operation string, obj *meta.Object, filterObject FilterObjectFn, filterList FilterListFn, noPatch bool) BroadcastResult {
	if isGlobKey(poolKey) {
		return processListBroadcast(cache, poolKey, operation, obj, filterObject, filterList, noPatch)
	}
	return processObjectBroadcast(cache, poolKey, operation, obj, filterObject, noPatch)
}

// processListBroadcast handles broadcasting to list subscriptions (glob paths)
func processListBroadcast(cache *Cache, poolKey string, operation string, obj *meta.Object, filterObject FilterObjectFn, filterList FilterListFn, noPatch bool) BroadcastResult {
	// Handle glob delete (obj is nil) - clear all items
	if obj == nil && operation == "del" {
		return processListDel(cache, poolKey, nil, noPatch)
	}

	if obj == nil {
		return BroadcastResult{Skip: true}
	}

	// Apply object filter first to see if this object passes
	filtered, filterErr := filterObject(poolKey, *obj)

	switch operation {
	case "set":
		return processListSet(cache, poolKey, obj, &filtered, filterErr, filterList, noPatch)
	case "del":
		return processListDel(cache, poolKey, obj, noPatch)
	}
	return BroadcastResult{Skip: true}
}

// processListSet handles set operations on list caches
func processListSet(cache *Cache, poolKey string, obj *meta.Object, filtered *meta.Object, filterErr error, filterList FilterListFn, noPatch bool) BroadcastResult {
	if filterErr != nil {
		// Object filtered out - remove from list if it exists
		newList, pos, found := removeFromList(cache.Objects, *obj)
		if found {
			cache.Objects = newList
			// Apply list filter after removal
			finalList, _ := filterList(poolKey, cache.Objects)
			cache.Objects = finalList
			return generateListResult(cache.Objects, "remove", pos, nil, noPatch)
		}
		return BroadcastResult{Skip: true}
	}

	// Check if this is an update or insert
	newList, _, found := updateInList(cache.Objects, *filtered)
	if found {
		// Update existing item
		cache.Objects = newList
		// Apply list filter (may re-sort)
		finalList, _ := filterList(poolKey, cache.Objects)
		cache.Objects = finalList
		// Find actual position after filter (may have re-sorted)
		actualPos := findPosition(finalList, filtered.Path)
		return generateListResult(cache.Objects, "replace", actualPos, filtered, noPatch)
	}

	// Insert new item
	oldLen := len(cache.Objects)
	newList, _ = insertSorted(cache.Objects, *filtered)
	// Apply list filter (may limit size and re-sort)
	finalList, _ := filterList(poolKey, newList)
	cache.Objects = finalList

	// Find actual position of our item in the filtered list
	actualPos := findPosition(finalList, filtered.Path)

	// Check if an item was pushed out due to limit
	itemPushedOut := len(finalList) < len(newList)

	if actualPos >= 0 && itemPushedOut {
		// Item is in the filtered list AND another was pushed out
		return generateAddRemoveResult(cache.Objects, actualPos, filtered, oldLen-1, noPatch)
	} else if actualPos >= 0 {
		// Item is in the filtered list, no item pushed out
		return generateListResult(cache.Objects, "add", actualPos, filtered, noPatch)
	} else if itemPushedOut && len(finalList) == oldLen {
		// Item was added but filtered out, and another was pushed out due to limit
		return generateListResult(cache.Objects, "remove", len(finalList), nil, noPatch)
	}
	return BroadcastResult{Skip: true}
}

// processListDel handles delete operations on list caches
func processListDel(cache *Cache, poolKey string, obj *meta.Object, noPatch bool) BroadcastResult {
	// For glob delete (obj is nil), clear all items from the cache
	if obj == nil {
		cache.Objects = nil
		// Return empty list as snapshot
		data, _ := meta.Encode([]meta.Object{})
		return BroadcastResult{Data: data, Snapshot: true}
	}

	newList, pos, found := removeFromList(cache.Objects, *obj)
	if found {
		cache.Objects = newList
		return generateListResult(cache.Objects, "remove", pos, nil, noPatch)
	}
	return BroadcastResult{Skip: true}
}

// processObjectBroadcast handles broadcasting to single object subscriptions (non-glob paths)
func processObjectBroadcast(cache *Cache, poolKey string, operation string, obj *meta.Object, filterObject FilterObjectFn, noPatch bool) BroadcastResult {
	switch operation {
	case "set":
		if obj == nil {
			return BroadcastResult{Skip: true}
		}
		filtered, err := filterObject(poolKey, *obj)
		if err != nil {
			// Filtered out - send empty object
			filtered = meta.Object{}
		}

		// Save old object before updating cache
		oldObj := cache.Object
		cache.Object = &filtered

		// Encode the filtered object
		data, encErr := meta.Encode(filtered)
		if encErr != nil {
			data = meta.EmptyObject
		}

		// NoPatch mode - send snapshot
		if noPatch {
			return BroadcastResult{Data: data, Snapshot: true}
		}

		patch, snapshot := generateObjectPatch(oldObj, &filtered)
		if snapshot || patch == nil {
			return BroadcastResult{Data: data, Snapshot: true}
		}
		return BroadcastResult{Data: patch, Snapshot: false}

	case "del":
		empty := meta.Object{Data: json.RawMessage("{}")}
		oldObj := cache.Object
		cache.Object = &empty

		if noPatch {
			return BroadcastResult{Data: meta.EmptyObject, Snapshot: true}
		}

		patch, snapshot := generateObjectPatch(oldObj, &empty)
		if snapshot || patch == nil {
			return BroadcastResult{Data: meta.EmptyObject, Snapshot: true}
		}
		return BroadcastResult{Data: patch, Snapshot: false}
	}
	return BroadcastResult{Skip: true}
}

// findPosition finds the position of an item by path in a list, returns -1 if not found
func findPosition(list []meta.Object, path string) int {
	for i, item := range list {
		if item.Path == path {
			return i
		}
	}
	return -1
}

// generateListResult creates a BroadcastResult for list operations
func generateListResult(objects []meta.Object, op string, pos int, obj *meta.Object, noPatch bool) BroadcastResult {
	if noPatch {
		data, err := meta.Encode(objects)
		if err != nil {
			return BroadcastResult{Skip: true}
		}
		return BroadcastResult{Data: data, Snapshot: true}
	}

	patch, err := generateListPatch(op, pos, obj)
	if err != nil {
		return BroadcastResult{Skip: true}
	}
	return BroadcastResult{Data: patch, Snapshot: false}
}

// generateAddRemoveResult creates a BroadcastResult for combined add+remove operations
func generateAddRemoveResult(objects []meta.Object, addPos int, obj *meta.Object, removePos int, noPatch bool) BroadcastResult {
	if noPatch {
		data, err := meta.Encode(objects)
		if err != nil {
			return BroadcastResult{Skip: true}
		}
		return BroadcastResult{Data: data, Snapshot: true}
	}

	patch, err := generateAddRemovePatch(addPos, obj, removePos)
	if err != nil {
		return BroadcastResult{Skip: true}
	}
	return BroadcastResult{Data: patch, Snapshot: false}
}

// insertSorted inserts obj into list maintaining ascending Created order (oldest first).
// Uses binary search for O(log n) position finding.
func insertSorted(list []meta.Object, obj meta.Object) ([]meta.Object, int) {
	n := len(list)
	// Binary search for insertion position
	lo, hi := 0, n
	for lo < hi {
		mid := lo + (hi-lo)/2
		if list[mid].Created < obj.Created {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	pos := lo

	// Grow slice and insert
	list = append(list, meta.Object{})
	copy(list[pos+1:], list[pos:])
	list[pos] = obj
	return list, pos
}

// updateInList finds and updates obj in list by matching Path.
func updateInList(list []meta.Object, obj meta.Object) ([]meta.Object, int, bool) {
	for i, item := range list {
		if item.Path == obj.Path {
			list[i] = obj
			return list, i, true
		}
	}
	return list, -1, false
}

// removeFromList removes obj from list by matching Path.
func removeFromList(list []meta.Object, obj meta.Object) ([]meta.Object, int, bool) {
	for i, item := range list {
		if item.Path == obj.Path {
			return append(list[:i], list[i+1:]...), i, true
		}
	}
	return list, -1, false
}

// generateListPatch creates a JSON patch for a single list operation.
// Uses sync.Pool to reduce allocations.
func generateListPatch(op string, pos int, obj *meta.Object) ([]byte, error) {
	// Get pooled slice
	patch := patchOpPool.Get().([]jsonpatch.Operation)
	defer patchOpPool.Put(patch)

	// Build path string efficiently
	path := "/" + strconv.Itoa(pos)

	switch op {
	case "add":
		objBytes, err := meta.Encode(*obj)
		if err != nil {
			return nil, err
		}
		patch[0] = jsonpatch.Operation{Operation: "add", Path: path, Value: json.RawMessage(objBytes)}
		return json.Marshal(patch)
	case "replace":
		objBytes, err := meta.Encode(*obj)
		if err != nil {
			return nil, err
		}
		patch[0] = jsonpatch.Operation{Operation: "replace", Path: path, Value: json.RawMessage(objBytes)}
		return json.Marshal(patch)
	case "remove":
		patch[0] = jsonpatch.Operation{Operation: "remove", Path: path, Value: nil}
		return json.Marshal(patch)
	}
	return nil, nil
}

// generateAddRemovePatch creates a JSON patch that adds an item and removes another.
// Uses sync.Pool to reduce allocations.
func generateAddRemovePatch(addPos int, obj *meta.Object, removePos int) ([]byte, error) {
	objBytes, err := meta.Encode(*obj)
	if err != nil {
		return nil, err
	}

	// Get pooled slice
	patch := patchOp2Pool.Get().([]jsonpatch.Operation)
	defer patchOp2Pool.Put(patch)

	patch[0] = jsonpatch.Operation{Operation: "remove", Path: "/" + strconv.Itoa(removePos), Value: nil}
	patch[1] = jsonpatch.Operation{Operation: "add", Path: "/" + strconv.Itoa(addPos), Value: json.RawMessage(objBytes)}
	return json.Marshal(patch)
}

// generateObjectPatch creates a JSON patch for a single object change.
// Returns patch bytes and whether to send as snapshot instead.
func generateObjectPatch(oldObj, newObj *meta.Object) ([]byte, bool) {
	if oldObj == nil || oldObj.Created == 0 {
		return nil, true
	}
	oldBytes, err := meta.Encode(*oldObj)
	if err != nil {
		return nil, true
	}
	newBytes, err := meta.Encode(*newObj)
	if err != nil {
		return nil, true
	}
	patch, err := jsonpatch.CreatePatch(oldBytes, newBytes)
	if err != nil {
		return nil, true
	}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return nil, true
	}
	return patchBytes, false
}
