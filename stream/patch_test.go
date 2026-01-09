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
