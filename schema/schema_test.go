package schema_test

import (
	"reflect"
	"testing"

	"github.com/benitogf/ooo/schema"
	"github.com/stretchr/testify/require"
)

func TestReflectNilReturnsNil(t *testing.T) {
	require.Nil(t, schema.Reflect(nil))
}

func TestReflectNonStructReturnsKindOnly(t *testing.T) {
	out := schema.Reflect("hello")
	require.Equal(t, map[string]any{"type": "string"}, out)
}

func TestReflectStructUsesJSONTagsAndSkipsOmitempty(t *testing.T) {
	type inner struct {
		Name string `json:"name"`
	}
	type payload struct {
		ID       int    `json:"id"`
		Skip     string `json:"-"`
		Optional string `json:"optional,omitempty"`
		Nested   inner  `json:"nested"`
		Tags     []int  `json:"tags"`
		FreeForm map[string]any
		// unexported is intentionally lowercase to verify it's skipped
		unexported string
	}
	_ = payload{}.unexported // silence unused-field linter

	got := schema.Reflect(payload{})
	require.Equal(t, map[string]any{
		"id":       0,
		"nested":   map[string]any{"name": ""},
		"tags":     []any{0},
		"FreeForm": map[string]any{},
	}, got)
}

func TestReflectPointerDereferences(t *testing.T) {
	type body struct {
		X bool `json:"x"`
	}
	got := schema.Reflect((*body)(nil))
	require.Equal(t, map[string]any{"x": false}, got)
}

func TestDefaultValueCoversKinds(t *testing.T) {
	require.Equal(t, "", schema.DefaultValue(reflect.TypeOf("")))
	require.Equal(t, 0, schema.DefaultValue(reflect.TypeOf(int64(0))))
	require.Equal(t, 0.0, schema.DefaultValue(reflect.TypeOf(float32(0))))
	require.Equal(t, false, schema.DefaultValue(reflect.TypeOf(false)))
	require.Equal(t, map[string]any{}, schema.DefaultValue(reflect.TypeOf(map[string]int{})))
	require.Equal(t, []any{""}, schema.DefaultValue(reflect.TypeOf([]string{})))
}
