package filters

import (
	"errors"
	"testing"

	"github.com/goccy/go-json"
	"github.com/stretchr/testify/require"

	"github.com/benitogf/ooo/meta"
)

func TestFilters_AddWrite(t *testing.T) {
	f := &Filters{}
	f.AddWrite("test/*", NoopFilter)
	require.Len(t, f.Write, 1)
	require.Equal(t, "test/*", f.Write[0].path)
}

func TestFilters_AddReadObject(t *testing.T) {
	f := &Filters{}
	f.AddReadObject("test/item", NoopObjectFilter)
	require.Len(t, f.ReadObject, 1)
	require.Equal(t, "test/item", f.ReadObject[0].path)
}

func TestFilters_AddReadList(t *testing.T) {
	f := &Filters{}
	f.AddReadList("test/*", NoopListFilter)
	require.Len(t, f.ReadList, 1)
	require.Equal(t, "test/*", f.ReadList[0].path)
}

func TestFilters_AddDelete(t *testing.T) {
	f := &Filters{}
	f.AddDelete("test/*", NoopHook)
	require.Len(t, f.Delete, 1)
	require.Equal(t, "test/*", f.Delete[0].path)
}

func TestFilters_AddAfterWrite(t *testing.T) {
	f := &Filters{}
	called := false
	f.AddAfterWrite("test/*", func(key string) { called = true })
	require.Len(t, f.AfterWrite, 1)
	f.AfterWrite[0].apply("test/123")
	require.True(t, called)
}

func TestFilters_AddOpenObject(t *testing.T) {
	f := &Filters{}
	f.AddOpenObject("item")
	require.Len(t, f.Write, 1)
	require.Len(t, f.ReadObject, 1)
	require.Len(t, f.Delete, 1)
}

func TestFilters_AddOpenList(t *testing.T) {
	f := &Filters{}
	f.AddOpenList("items/*")
	require.Len(t, f.Write, 1)
	require.Len(t, f.ReadList, 1)
	require.Len(t, f.Delete, 1)
}

func TestFilter_Check_NoMatch_NonStatic(t *testing.T) {
	f := Filter{}
	data := json.RawMessage(`{"key":"value"}`)
	result, err := f.Check("unknown/path", data, false)
	require.NoError(t, err)
	require.Equal(t, data, result)
}

func TestFilter_Check_NoMatch_Static(t *testing.T) {
	f := Filter{}
	data := json.RawMessage(`{"key":"value"}`)
	_, err := f.Check("unknown/path", data, true)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrRouteNotDefined)
}

func TestFilter_Check_ExactMatch(t *testing.T) {
	f := Filter{}
	f = append(f, filter{
		path: "test/item",
		apply: func(key string, data json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"modified":true}`), nil
		},
	})
	result, err := f.Check("test/item", json.RawMessage(`{}`), false)
	require.NoError(t, err)
	require.Equal(t, `{"modified":true}`, string(result))
}

func TestFilter_Check_GlobMatch(t *testing.T) {
	f := Filter{}
	f = append(f, filter{
		path: "test/*",
		apply: func(key string, data json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"glob":true}`), nil
		},
	})
	result, err := f.Check("test/123", json.RawMessage(`{}`), false)
	require.NoError(t, err)
	require.Equal(t, `{"glob":true}`, string(result))
}

func TestFilter_Check_FilterError(t *testing.T) {
	f := Filter{}
	expectedErr := errors.New("filter error")
	f = append(f, filter{
		path: "test/*",
		apply: func(key string, data json.RawMessage) (json.RawMessage, error) {
			return nil, expectedErr
		},
	})
	_, err := f.Check("test/123", json.RawMessage(`{}`), false)
	require.Error(t, err)
	require.Equal(t, expectedErr, err)
}

func TestFilter_Check_InvalidResult(t *testing.T) {
	f := Filter{}
	f = append(f, filter{
		path: "test/*",
		apply: func(key string, data json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`invalid json`), nil
		},
	})
	_, err := f.Check("test/123", json.RawMessage(`{}`), false)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidFilterResult)
}

func TestFilter_Check_EmptyResult(t *testing.T) {
	f := Filter{}
	f = append(f, filter{
		path: "test/*",
		apply: func(key string, data json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage{}, nil
		},
	})
	_, err := f.Check("test/123", json.RawMessage(`{}`), false)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidFilterResult)
}

func TestFilter_HasMatch(t *testing.T) {
	f := Filter{}
	f = append(f, filter{path: "test/*", apply: NoopFilter})
	f = append(f, filter{path: "other/item", apply: NoopFilter})

	require.Equal(t, 0, f.HasMatch("test/123"))
	require.Equal(t, 1, f.HasMatch("other/item"))
	require.Equal(t, -1, f.HasMatch("unknown"))
}

func TestFilter_Apply(t *testing.T) {
	f := Filter{}
	f = append(f, filter{
		path: "test/*",
		apply: func(key string, data json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"applied":true}`), nil
		},
	})
	result, err := f.Apply(0, "test/123", json.RawMessage(`{}`))
	require.NoError(t, err)
	require.Equal(t, `{"applied":true}`, string(result))
}

func TestFilter_CheckStatic_NoMatch_NonStatic(t *testing.T) {
	f := Filter{}
	err := f.CheckStatic("unknown", false)
	require.NoError(t, err)
}

func TestFilter_CheckStatic_NoMatch_Static(t *testing.T) {
	f := Filter{}
	err := f.CheckStatic("unknown", true)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrRouteNotDefined)
}

func TestFilter_CheckStatic_Match(t *testing.T) {
	f := Filter{}
	f = append(f, filter{path: "test/*", apply: NoopFilter})
	err := f.CheckStatic("test/123", true)
	require.NoError(t, err)
}

func TestObjectFilter_Check_NoMatch_NonStatic(t *testing.T) {
	f := ObjectFilter{}
	obj := meta.Object{Index: "test"}
	result, err := f.Check("unknown", obj, false)
	require.NoError(t, err)
	require.Equal(t, obj, result)
}

func TestObjectFilter_Check_NoMatch_Static(t *testing.T) {
	f := ObjectFilter{}
	obj := meta.Object{Index: "test"}
	_, err := f.Check("unknown", obj, true)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrRouteNotDefined)
}

func TestObjectFilter_Check_Match(t *testing.T) {
	f := ObjectFilter{}
	f = append(f, objectFilter{
		path: "test/item",
		apply: func(key string, obj meta.Object) (meta.Object, error) {
			obj.Index = "modified"
			return obj, nil
		},
	})
	result, err := f.Check("test/item", meta.Object{Index: "original"}, false)
	require.NoError(t, err)
	require.Equal(t, "modified", result.Index)
}

func TestObjectFilter_Check_GlobMatch(t *testing.T) {
	f := ObjectFilter{}
	f = append(f, objectFilter{
		path: "test/*",
		apply: func(key string, obj meta.Object) (meta.Object, error) {
			obj.Path = "glob-matched"
			return obj, nil
		},
	})
	result, err := f.Check("test/123", meta.Object{}, false)
	require.NoError(t, err)
	require.Equal(t, "glob-matched", result.Path)
}

func TestObjectFilter_HasMatch(t *testing.T) {
	f := ObjectFilter{}
	f = append(f, objectFilter{path: "test/*", apply: NoopObjectFilter})
	require.Equal(t, 0, f.HasMatch("test/123"))
	require.Equal(t, -1, f.HasMatch("unknown"))
}

func TestListFilter_Check_NoMatch_NonStatic(t *testing.T) {
	f := ListFilter{}
	objs := []meta.Object{{Index: "1"}, {Index: "2"}}
	result, err := f.Check("unknown", objs, false)
	require.NoError(t, err)
	require.Equal(t, objs, result)
}

func TestListFilter_Check_NoMatch_Static(t *testing.T) {
	f := ListFilter{}
	objs := []meta.Object{{Index: "1"}}
	_, err := f.Check("unknown", objs, true)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrRouteNotDefined)
}

func TestListFilter_Check_Match(t *testing.T) {
	f := ListFilter{}
	f = append(f, listFilter{
		path: "items/*",
		apply: func(key string, objs []meta.Object) ([]meta.Object, error) {
			return objs[:1], nil // Return only first item
		},
	})
	result, err := f.Check("items/abc", []meta.Object{{Index: "1"}, {Index: "2"}}, false)
	require.NoError(t, err)
	require.Len(t, result, 1)
}

func TestListFilter_HasMatch(t *testing.T) {
	f := ListFilter{}
	f = append(f, listFilter{path: "items/*", apply: NoopListFilter})
	require.Equal(t, 0, f.HasMatch("items/123"))
	require.Equal(t, -1, f.HasMatch("unknown"))
}

func TestBlocker_Check_NoMatch_NonStatic(t *testing.T) {
	b := Blocker{}
	err := b.Check("unknown", false)
	require.NoError(t, err)
}

func TestBlocker_Check_NoMatch_Static(t *testing.T) {
	b := Blocker{}
	err := b.Check("unknown", true)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrRouteNotDefined)
}

func TestBlocker_Check_Match_Allow(t *testing.T) {
	b := Blocker{}
	b = append(b, block{path: "test/*", apply: NoopHook})
	err := b.Check("test/123", false)
	require.NoError(t, err)
}

func TestBlocker_Check_Match_Block(t *testing.T) {
	b := Blocker{}
	blockErr := errors.New("blocked")
	b = append(b, block{
		path: "test/*",
		apply: func(key string) error {
			return blockErr
		},
	})
	err := b.Check("test/123", false)
	require.Error(t, err)
	require.Equal(t, blockErr, err)
}

func TestWatcher_Check_NoMatch(t *testing.T) {
	w := Watcher{}
	called := false
	w = append(w, watch{
		path:  "other/*",
		apply: func(key string) { called = true },
	})
	w.Check("test/123")
	require.False(t, called)
}

func TestWatcher_Check_Match(t *testing.T) {
	w := Watcher{}
	var calledWith string
	w = append(w, watch{
		path:  "test/*",
		apply: func(key string) { calledWith = key },
	})
	w.Check("test/123")
	require.Equal(t, "test/123", calledWith)
}

func TestFilters_Paths(t *testing.T) {
	f := &Filters{}
	f.AddWrite("write/*", NoopFilter)
	f.AddReadObject("read/item", NoopObjectFilter)
	f.AddReadList("read/*", NoopListFilter)
	f.AddDelete("delete/*", NoopHook)
	f.AddAfterWrite("after/*", func(key string) {})

	paths := f.Paths()
	require.Len(t, paths, 5)

	// Check all paths are present (order not guaranteed)
	pathMap := make(map[string]bool)
	for _, p := range paths {
		pathMap[p] = true
	}
	require.True(t, pathMap["write/*"])
	require.True(t, pathMap["read/item"])
	require.True(t, pathMap["read/*"])
	require.True(t, pathMap["delete/*"])
	require.True(t, pathMap["after/*"])
}

func TestFilters_Paths_Deduplication(t *testing.T) {
	f := &Filters{}
	f.AddWrite("items/*", NoopFilter)
	f.AddReadList("items/*", NoopListFilter)
	f.AddDelete("items/*", NoopHook)

	paths := f.Paths()
	require.Len(t, paths, 1)
	require.Equal(t, "items/*", paths[0])
}

func TestNoopFilter(t *testing.T) {
	data := json.RawMessage(`{"test":true}`)
	result, err := NoopFilter("key", data)
	require.NoError(t, err)
	require.Equal(t, data, result)
}

func TestNoopObjectFilter(t *testing.T) {
	obj := meta.Object{Index: "test", Created: 123}
	result, err := NoopObjectFilter("key", obj)
	require.NoError(t, err)
	require.Equal(t, obj, result)
}

func TestNoopListFilter(t *testing.T) {
	objs := []meta.Object{{Index: "1"}, {Index: "2"}}
	result, err := NoopListFilter("key", objs)
	require.NoError(t, err)
	require.Equal(t, objs, result)
}

func TestNoopHook(t *testing.T) {
	err := NoopHook("any/key")
	require.NoError(t, err)
}

func TestFilter_FirstMatchOnly(t *testing.T) {
	f := Filter{}
	callOrder := []int{}
	f = append(f, filter{
		path: "test/*",
		apply: func(key string, data json.RawMessage) (json.RawMessage, error) {
			callOrder = append(callOrder, 1)
			return json.RawMessage(`{"first":true}`), nil
		},
	})
	f = append(f, filter{
		path: "test/*",
		apply: func(key string, data json.RawMessage) (json.RawMessage, error) {
			callOrder = append(callOrder, 2)
			return json.RawMessage(`{"second":true}`), nil
		},
	})

	result, err := f.Check("test/123", json.RawMessage(`{}`), false)
	require.NoError(t, err)
	require.Equal(t, `{"first":true}`, string(result))
	require.Len(t, callOrder, 1)
	require.Equal(t, 1, callOrder[0])
}

func BenchmarkFilter_Check(b *testing.B) {
	f := Filter{}
	for i := 0; i < 10; i++ {
		f = append(f, filter{path: "path" + string(rune('a'+i)) + "/*", apply: NoopFilter})
	}
	f = append(f, filter{path: "target/*", apply: NoopFilter})
	data := json.RawMessage(`{"key":"value"}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Check("target/123", data, false)
	}
}

func BenchmarkObjectFilter_Check(b *testing.B) {
	f := ObjectFilter{}
	for i := 0; i < 10; i++ {
		f = append(f, objectFilter{path: "path" + string(rune('a'+i)) + "/*", apply: NoopObjectFilter})
	}
	f = append(f, objectFilter{path: "target/*", apply: NoopObjectFilter})
	obj := meta.Object{Index: "test", Created: 123}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Check("target/123", obj, false)
	}
}

func BenchmarkListFilter_Check(b *testing.B) {
	f := ListFilter{}
	for i := 0; i < 10; i++ {
		f = append(f, listFilter{path: "path" + string(rune('a'+i)) + "/*", apply: NoopListFilter})
	}
	f = append(f, listFilter{path: "target/*", apply: NoopListFilter})
	objs := []meta.Object{{Index: "1"}, {Index: "2"}, {Index: "3"}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Check("target/123", objs, false)
	}
}
