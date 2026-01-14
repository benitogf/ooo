package stream

import (
	"testing"

	"github.com/goccy/go-json"

	"github.com/benitogf/ooo/meta"
)

func noopFilterObject(key string, obj meta.Object) (meta.Object, error) {
	return obj, nil
}

func noopFilterList(key string, objs []meta.Object) ([]meta.Object, error) {
	return objs, nil
}

func TestProcessBroadcast_ObjectSet(t *testing.T) {
	cache := &Cache{}
	obj := &meta.Object{
		Path:    "test/1",
		Created: 100,
		Updated: 100,
		Data:    json.RawMessage(`"test data"`),
	}

	result := ProcessBroadcast(cache, "test/1", "set", obj, noopFilterObject, noopFilterList, false)

	if result.Skip {
		t.Error("expected result not to be skipped")
	}
	if !result.Snapshot {
		t.Error("expected snapshot for first object set")
	}
	if cache.Object == nil {
		t.Error("expected cache.Object to be set")
	}
}

func TestProcessBroadcast_ObjectUpdate(t *testing.T) {
	oldObj := &meta.Object{
		Path:    "test/1",
		Created: 100,
		Updated: 100,
		Data:    json.RawMessage(`"old data"`),
	}
	cache := &Cache{Object: oldObj}

	newObj := &meta.Object{
		Path:    "test/1",
		Created: 100,
		Updated: 200,
		Data:    json.RawMessage(`"new data"`),
	}

	result := ProcessBroadcast(cache, "test/1", "set", newObj, noopFilterObject, noopFilterList, false)

	if result.Skip {
		t.Error("expected result not to be skipped")
	}
	if result.Snapshot {
		t.Error("expected patch for object update, got snapshot")
	}
	if result.Data == nil {
		t.Error("expected data to be set")
	}
}

func TestProcessBroadcast_ObjectDel(t *testing.T) {
	oldObj := &meta.Object{
		Path:    "test/1",
		Created: 100,
		Data:    json.RawMessage(`"data"`),
	}
	cache := &Cache{Object: oldObj}

	result := ProcessBroadcast(cache, "test/1", "del", nil, noopFilterObject, noopFilterList, false)

	if result.Skip {
		t.Error("expected result not to be skipped")
	}
	if cache.Object.Created != 0 {
		t.Error("expected cache.Object to be empty after delete")
	}
}

func TestProcessBroadcast_ListAdd(t *testing.T) {
	cache := &Cache{Objects: []meta.Object{}}
	obj := &meta.Object{
		Path:    "items/1",
		Created: 100,
		Data:    json.RawMessage(`"item1"`),
	}

	result := ProcessBroadcast(cache, "items/*", "set", obj, noopFilterObject, noopFilterList, false)

	if result.Skip {
		t.Error("expected result not to be skipped")
	}
	if result.Snapshot {
		t.Error("expected patch for list add")
	}
	if len(cache.Objects) != 1 {
		t.Errorf("expected 1 item in cache, got %d", len(cache.Objects))
	}
}

func TestProcessBroadcast_ListUpdate(t *testing.T) {
	cache := &Cache{
		Objects: []meta.Object{
			{Path: "items/1", Created: 100, Data: json.RawMessage(`"old"`)},
		},
	}
	obj := &meta.Object{
		Path:    "items/1",
		Created: 100,
		Updated: 200,
		Data:    json.RawMessage(`"new"`),
	}

	result := ProcessBroadcast(cache, "items/*", "set", obj, noopFilterObject, noopFilterList, false)

	if result.Skip {
		t.Error("expected result not to be skipped")
	}
	if string(cache.Objects[0].Data) != `"new"` {
		t.Errorf("expected updated data, got %s", cache.Objects[0].Data)
	}
}

func TestProcessBroadcast_ListRemove(t *testing.T) {
	cache := &Cache{
		Objects: []meta.Object{
			{Path: "items/1", Created: 100},
			{Path: "items/2", Created: 200},
		},
	}
	obj := &meta.Object{Path: "items/1"}

	result := ProcessBroadcast(cache, "items/*", "del", obj, noopFilterObject, noopFilterList, false)

	if result.Skip {
		t.Error("expected result not to be skipped")
	}
	if len(cache.Objects) != 1 {
		t.Errorf("expected 1 item in cache after delete, got %d", len(cache.Objects))
	}
	if cache.Objects[0].Path != "items/2" {
		t.Error("wrong item was removed")
	}
}

func TestProcessBroadcast_NoPatchMode(t *testing.T) {
	oldObj := &meta.Object{
		Path:    "test/1",
		Created: 100,
		Data:    json.RawMessage(`"old"`),
	}
	cache := &Cache{Object: oldObj}

	newObj := &meta.Object{
		Path:    "test/1",
		Created: 100,
		Updated: 200,
		Data:    json.RawMessage(`"new"`),
	}

	// With noPatch=true, should always return snapshot
	result := ProcessBroadcast(cache, "test/1", "set", newObj, noopFilterObject, noopFilterList, true)

	if result.Skip {
		t.Error("expected result not to be skipped")
	}
	if !result.Snapshot {
		t.Error("expected snapshot in noPatch mode")
	}
}

func TestProcessBroadcast_NilObject(t *testing.T) {
	cache := &Cache{}

	result := ProcessBroadcast(cache, "test/1", "set", nil, noopFilterObject, noopFilterList, false)

	if !result.Skip {
		t.Error("expected skip for nil object")
	}
}

func TestProcessBroadcast_ListNoPatchMode(t *testing.T) {
	cache := &Cache{Objects: []meta.Object{}}
	obj := &meta.Object{
		Path:    "items/1",
		Created: 100,
		Data:    json.RawMessage(`"item1"`),
	}

	result := ProcessBroadcast(cache, "items/*", "set", obj, noopFilterObject, noopFilterList, true)

	if result.Skip {
		t.Error("expected result not to be skipped")
	}
	if !result.Snapshot {
		t.Error("expected snapshot in noPatch mode for list")
	}
}

func TestIsGlobKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"users/*", true},
		{"items/*/data", true},
		{"*", true},
		{"users/123", false},
		{"items/abc/data", false},
		{"", false},
		{"no-glob-here", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := isGlobKey(tt.key); got != tt.want {
				t.Errorf("isGlobKey(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestInsertSorted(t *testing.T) {
	tests := []struct {
		name    string
		list    []meta.Object
		obj     meta.Object
		wantPos int
		wantLen int
	}{
		{
			name:    "insert into empty list",
			list:    []meta.Object{},
			obj:     meta.Object{Path: "a", Created: 100},
			wantPos: 0,
			wantLen: 1,
		},
		{
			name: "insert at beginning",
			list: []meta.Object{
				{Path: "b", Created: 200},
			},
			obj:     meta.Object{Path: "a", Created: 50},
			wantPos: 0,
			wantLen: 2,
		},
		{
			name: "insert at end",
			list: []meta.Object{
				{Path: "a", Created: 100},
			},
			obj:     meta.Object{Path: "b", Created: 200},
			wantPos: 1,
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list := make([]meta.Object, len(tt.list))
			copy(list, tt.list)

			result, pos := insertSorted(list, tt.obj)
			if pos != tt.wantPos {
				t.Errorf("insertSorted() pos = %d, want %d", pos, tt.wantPos)
			}
			if len(result) != tt.wantLen {
				t.Errorf("insertSorted() len = %d, want %d", len(result), tt.wantLen)
			}
		})
	}
}

func TestUpdateInList(t *testing.T) {
	list := []meta.Object{
		{Path: "users/1", Created: 100},
		{Path: "users/2", Created: 200},
	}

	t.Run("update existing", func(t *testing.T) {
		testList := make([]meta.Object, len(list))
		copy(testList, list)

		updated := meta.Object{Path: "users/2", Created: 200, Data: json.RawMessage(`"new"`)}
		_, pos, found := updateInList(testList, updated)

		if !found || pos != 1 {
			t.Error("updateInList() failed to find existing item")
		}
	})

	t.Run("not found", func(t *testing.T) {
		testList := make([]meta.Object, len(list))
		copy(testList, list)

		_, pos, found := updateInList(testList, meta.Object{Path: "users/999"})

		if found || pos != -1 {
			t.Error("updateInList() should not find non-existent item")
		}
	})
}

func TestRemoveFromList(t *testing.T) {
	t.Run("remove existing", func(t *testing.T) {
		list := []meta.Object{
			{Path: "a", Created: 100},
			{Path: "b", Created: 200},
		}

		result, pos, found := removeFromList(list, meta.Object{Path: "a"})

		if !found || pos != 0 || len(result) != 1 {
			t.Error("removeFromList() failed")
		}
	})

	t.Run("not found", func(t *testing.T) {
		list := []meta.Object{{Path: "a"}}

		_, pos, found := removeFromList(list, meta.Object{Path: "z"})

		if found || pos != -1 {
			t.Error("removeFromList() should not find non-existent item")
		}
	})
}

// Test for LimitFilter behavior: when an item is added and another is pushed out
func TestProcessBroadcast_ListAddWithLimit(t *testing.T) {
	// Create a filter that limits to 3 items, keeping newest (sorted desc for display)
	limitFilter := func(key string, objs []meta.Object) ([]meta.Object, error) {
		// Sort by Created descending (newest first)
		for i := 0; i < len(objs)-1; i++ {
			for j := i + 1; j < len(objs); j++ {
				if objs[i].Created < objs[j].Created {
					objs[i], objs[j] = objs[j], objs[i]
				}
			}
		}
		// Limit to 3
		if len(objs) > 3 {
			return objs[:3], nil
		}
		return objs, nil
	}

	// Start with 3 items at limit
	cache := &Cache{
		Objects: []meta.Object{
			{Path: "items/3", Created: 300, Data: json.RawMessage(`"item3"`)},
			{Path: "items/2", Created: 200, Data: json.RawMessage(`"item2"`)},
			{Path: "items/1", Created: 100, Data: json.RawMessage(`"item1"`)},
		},
	}

	// Add a new item that is newest (should push out oldest)
	newObj := &meta.Object{
		Path:    "items/4",
		Created: 400,
		Data:    json.RawMessage(`"item4"`),
	}

	result := ProcessBroadcast(cache, "items/*", "set", newObj, noopFilterObject, limitFilter, false)

	if result.Skip {
		t.Error("expected result not to be skipped when new item stays in list")
	}
	if len(cache.Objects) != 3 {
		t.Errorf("expected 3 items in cache, got %d", len(cache.Objects))
	}
	// Newest item should be in the list
	found := false
	for _, obj := range cache.Objects {
		if obj.Path == "items/4" {
			found = true
			break
		}
	}
	if !found {
		t.Error("new item should be in the filtered list")
	}
}

// Test for LimitFilter behavior with OrderAsc: newest item pushed out (edge case)
func TestProcessBroadcast_ListAddWithLimitOrderAsc(t *testing.T) {
	// Create a filter that limits to 3 items, sorted asc (oldest first for display)
	// but KEEPS the newest items (correct behavior after fix)
	limitFilter := func(key string, objs []meta.Object) ([]meta.Object, error) {
		// Sort by Created descending to find newest
		for i := 0; i < len(objs)-1; i++ {
			for j := i + 1; j < len(objs); j++ {
				if objs[i].Created < objs[j].Created {
					objs[i], objs[j] = objs[j], objs[i]
				}
			}
		}
		// Limit to 3 newest
		if len(objs) > 3 {
			objs = objs[:3]
		}
		// Re-sort ascending for display
		for i := 0; i < len(objs)-1; i++ {
			for j := i + 1; j < len(objs); j++ {
				if objs[i].Created > objs[j].Created {
					objs[i], objs[j] = objs[j], objs[i]
				}
			}
		}
		return objs, nil
	}

	// Start with 3 items at limit (sorted asc)
	cache := &Cache{
		Objects: []meta.Object{
			{Path: "items/1", Created: 100, Data: json.RawMessage(`"item1"`)},
			{Path: "items/2", Created: 200, Data: json.RawMessage(`"item2"`)},
			{Path: "items/3", Created: 300, Data: json.RawMessage(`"item3"`)},
		},
	}

	// Add a new item that is newest
	newObj := &meta.Object{
		Path:    "items/4",
		Created: 400,
		Data:    json.RawMessage(`"item4"`),
	}

	result := ProcessBroadcast(cache, "items/*", "set", newObj, noopFilterObject, limitFilter, false)

	if result.Skip {
		t.Error("expected result not to be skipped when newest item is added")
	}
	if len(cache.Objects) != 3 {
		t.Errorf("expected 3 items in cache, got %d", len(cache.Objects))
	}
	// New item should be at the end (sorted asc, newest last)
	if cache.Objects[2].Path != "items/4" {
		t.Errorf("expected items/4 at end (newest), got %s", cache.Objects[2].Path)
	}
	// Oldest item should be pushed out
	for _, obj := range cache.Objects {
		if obj.Path == "items/1" {
			t.Error("oldest item (items/1) should have been pushed out")
		}
	}
}

// Test for the Skip case: item added but immediately pushed out by limit
func TestProcessBroadcast_ListAddPushedOutByLimit(t *testing.T) {
	// Create a filter that keeps only items with Created >= 200
	// This simulates a case where the new item doesn't pass the filter
	limitFilter := func(key string, objs []meta.Object) ([]meta.Object, error) {
		filtered := make([]meta.Object, 0)
		for _, obj := range objs {
			if obj.Created >= 200 {
				filtered = append(filtered, obj)
			}
		}
		return filtered, nil
	}

	// Start with 2 items that pass the filter
	cache := &Cache{
		Objects: []meta.Object{
			{Path: "items/2", Created: 200, Data: json.RawMessage(`"item2"`)},
			{Path: "items/3", Created: 300, Data: json.RawMessage(`"item3"`)},
		},
	}

	// Add an item with Created=100 that won't pass the filter
	newObj := &meta.Object{
		Path:    "items/1",
		Created: 100,
		Data:    json.RawMessage(`"item1"`),
	}

	result := ProcessBroadcast(cache, "items/*", "set", newObj, noopFilterObject, limitFilter, false)

	// Item was filtered out but nothing else changed, should skip
	if !result.Skip {
		t.Error("expected skip when item is immediately filtered out")
	}
	if len(cache.Objects) != 2 {
		t.Errorf("expected 2 items in cache (unchanged), got %d", len(cache.Objects))
	}
}

// Test add+remove patch generation when at limit
func TestProcessBroadcast_ListAddRemovePatch(t *testing.T) {
	// Create a filter that limits to 3 items (newest first)
	limitFilter := func(key string, objs []meta.Object) ([]meta.Object, error) {
		// Sort by Created descending
		for i := 0; i < len(objs)-1; i++ {
			for j := i + 1; j < len(objs); j++ {
				if objs[i].Created < objs[j].Created {
					objs[i], objs[j] = objs[j], objs[i]
				}
			}
		}
		if len(objs) > 3 {
			return objs[:3], nil
		}
		return objs, nil
	}

	// Start with exactly 3 items
	cache := &Cache{
		Objects: []meta.Object{
			{Path: "items/3", Created: 300, Data: json.RawMessage(`"item3"`)},
			{Path: "items/2", Created: 200, Data: json.RawMessage(`"item2"`)},
			{Path: "items/1", Created: 100, Data: json.RawMessage(`"item1"`)},
		},
	}

	// Add a new newest item
	newObj := &meta.Object{
		Path:    "items/4",
		Created: 400,
		Data:    json.RawMessage(`"item4"`),
	}

	result := ProcessBroadcast(cache, "items/*", "set", newObj, noopFilterObject, limitFilter, false)

	if result.Skip {
		t.Error("expected add+remove result, not skip")
	}
	if result.Snapshot {
		t.Error("expected patch, not snapshot")
	}
	// Verify the patch contains both add and remove operations
	var patch []map[string]interface{}
	if err := json.Unmarshal(result.Data, &patch); err != nil {
		t.Fatalf("failed to unmarshal patch: %v", err)
	}
	if len(patch) != 2 {
		t.Errorf("expected 2 operations (add+remove), got %d", len(patch))
	}
	// First operation should be remove, second should be add
	if patch[0]["op"] != "remove" {
		t.Errorf("expected first op to be 'remove', got %v", patch[0]["op"])
	}
	if patch[1]["op"] != "add" {
		t.Errorf("expected second op to be 'add', got %v", patch[1]["op"])
	}
}

func TestGenerateObjectPatch(t *testing.T) {
	t.Run("nil old object", func(t *testing.T) {
		newObj := &meta.Object{Path: "test", Created: 100}
		_, snapshot := generateObjectPatch(nil, newObj)

		if !snapshot {
			t.Error("expected snapshot for nil old object")
		}
	})

	t.Run("zero created", func(t *testing.T) {
		oldObj := &meta.Object{Path: "test", Created: 0}
		newObj := &meta.Object{Path: "test", Created: 100}
		_, snapshot := generateObjectPatch(oldObj, newObj)

		if !snapshot {
			t.Error("expected snapshot for zero-created old object")
		}
	})

	t.Run("valid patch", func(t *testing.T) {
		oldObj := &meta.Object{
			Path:    "test/1",
			Created: 100,
			Updated: 100,
			Data:    json.RawMessage(`"old"`),
		}
		newObj := &meta.Object{
			Path:    "test/1",
			Created: 100,
			Updated: 200,
			Data:    json.RawMessage(`"new"`),
		}

		patch, snapshot := generateObjectPatch(oldObj, newObj)

		if snapshot {
			t.Error("expected patch, got snapshot")
		}
		if patch == nil {
			t.Error("expected non-nil patch")
		}
	})
}
