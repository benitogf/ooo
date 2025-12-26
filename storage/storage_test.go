package storage

import (
	"testing"

	"github.com/benitogf/ooo/meta"
	"github.com/benitogf/ooo/monotonic"
	"github.com/goccy/go-json"
	"github.com/stretchr/testify/require"
)

func init() {
	monotonic.Init()
}

var testData = json.RawMessage(`{"name": "test", "value": 123}`)
var testDataUpdate = json.RawMessage(`{"name": "test", "value": 456}`)

func TestMemoryLayer(t *testing.T) {
	t.Parallel()

	layer := NewMemoryLayer()
	err := layer.Start(LayerOptions{})
	require.NoError(t, err)
	require.True(t, layer.Active())
	defer layer.Close()

	// Test Set and Get
	_, err = layer.Get("test")
	require.Error(t, err)
	require.Equal(t, ErrNotFound, err)

	storage := New(LayeredConfig{
		Memory: layer,
	})
	err = storage.Start(Options{})
	require.NoError(t, err)
	defer storage.Close()

	index, err := storage.Set("test", testData)
	require.NoError(t, err)
	require.Equal(t, "test", index)

	obj, err := storage.Get("test")
	require.NoError(t, err)
	require.Equal(t, testData, json.RawMessage(obj.Data))

	// Test Update
	index, err = storage.Set("test", testDataUpdate)
	require.NoError(t, err)
	require.Equal(t, "test", index)

	obj, err = storage.Get("test")
	require.NoError(t, err)
	require.Equal(t, testDataUpdate, json.RawMessage(obj.Data))
	require.NotEqual(t, int64(0), obj.Updated)

	// Test Delete
	err = storage.Del("test")
	require.NoError(t, err)

	_, err = storage.Get("test")
	require.Error(t, err)
	require.Equal(t, ErrNotFound, err)
}

func TestMemoryStorageList(t *testing.T) {
	t.Parallel()

	storage := New(LayeredConfig{
		Memory: NewMemoryLayer(),
	})
	err := storage.Start(Options{})
	require.NoError(t, err)
	defer storage.Close()

	// Add multiple entries
	_, err = storage.Set("test/123", testData)
	require.NoError(t, err)

	_, err = storage.Set("test/456", testDataUpdate)
	require.NoError(t, err)

	// Test GetList
	objs, err := storage.GetList("test/*")
	require.NoError(t, err)
	require.Equal(t, 2, len(objs))

	// Test Keys
	keys, err := storage.Keys()
	require.NoError(t, err)
	require.Equal(t, []string{"test/123", "test/456"}, keys)

	// Test glob delete
	err = storage.Del("test/*")
	require.NoError(t, err)

	objs, err = storage.GetList("test/*")
	require.NoError(t, err)
	require.Equal(t, 0, len(objs))
}

func TestMemoryStoragePush(t *testing.T) {
	t.Parallel()

	storage := New(LayeredConfig{
		Memory: NewMemoryLayer(),
	})
	err := storage.Start(Options{})
	require.NoError(t, err)
	defer storage.Close()

	// Push creates new key from glob pattern
	index, err := storage.Push("test/*", testData)
	require.NoError(t, err)
	require.NotEmpty(t, index)

	// Verify it was stored
	objs, err := storage.GetList("test/*")
	require.NoError(t, err)
	require.Equal(t, 1, len(objs))
	require.Equal(t, index, objs[0].Index)
}

func TestMemoryStoragePatch(t *testing.T) {
	t.Parallel()

	storage := New(LayeredConfig{
		Memory: NewMemoryLayer(),
	})
	err := storage.Start(Options{})
	require.NoError(t, err)
	defer storage.Close()

	// Set initial data
	_, err = storage.Set("test", json.RawMessage(`{"a": 1, "b": 2}`))
	require.NoError(t, err)

	// Patch
	_, err = storage.Patch("test", json.RawMessage(`{"b": 3, "c": 4}`))
	require.NoError(t, err)

	obj, err := storage.Get("test")
	require.NoError(t, err)

	var result map[string]int
	err = json.Unmarshal(obj.Data, &result)
	require.NoError(t, err)
	require.Equal(t, 1, result["a"])
	require.Equal(t, 3, result["b"])
	require.Equal(t, 4, result["c"])
}

func TestMemoryStorageGetN(t *testing.T) {
	t.Parallel()

	storage := New(LayeredConfig{
		Memory: NewMemoryLayer(),
	})
	err := storage.Start(Options{})
	require.NoError(t, err)
	defer storage.Close()

	// Add entries with different created times
	for i := 0; i < 5; i++ {
		key := "test/" + string(rune('0'+i))
		_, err := storage.SetWithMeta(key, testData, int64(i), 0)
		require.NoError(t, err)
	}

	// GetN descending (most recent first)
	objs, err := storage.GetN("test/*", 2)
	require.NoError(t, err)
	require.Equal(t, 2, len(objs))
	require.Equal(t, int64(4), objs[0].Created)
	require.Equal(t, int64(3), objs[1].Created)

	// GetNAscending (oldest first)
	objs, err = storage.GetNAscending("test/*", 2)
	require.NoError(t, err)
	require.Equal(t, 2, len(objs))
	require.Equal(t, int64(0), objs[0].Created)
	require.Equal(t, int64(1), objs[1].Created)
}

func TestMemoryStorageLocking(t *testing.T) {
	t.Parallel()

	storage := New(LayeredConfig{
		Memory: NewMemoryLayer(),
	})
	err := storage.Start(Options{})
	require.NoError(t, err)
	defer storage.Close()

	// Set initial data
	_, err = storage.Set("test", testData)
	require.NoError(t, err)

	// GetAndLock
	obj, err := storage.GetAndLock("test")
	require.NoError(t, err)
	require.Equal(t, testData, json.RawMessage(obj.Data))

	// SetAndUnlock
	_, err = storage.SetAndUnlock("test", testDataUpdate)
	require.NoError(t, err)

	obj, err = storage.Get("test")
	require.NoError(t, err)
	require.Equal(t, testDataUpdate, json.RawMessage(obj.Data))
}

func TestLayeredStorageMemoryOnly(t *testing.T) {
	t.Parallel()

	storage := New(LayeredConfig{
		Memory: NewMemoryLayer(),
	})
	err := storage.Start(Options{})
	require.NoError(t, err)
	defer storage.Close()

	// Test basic operations
	index, err := storage.Set("test", testData)
	require.NoError(t, err)
	require.Equal(t, "test", index)

	obj, err := storage.Get("test")
	require.NoError(t, err)
	require.Equal(t, testData, json.RawMessage(obj.Data))

	err = storage.Del("test")
	require.NoError(t, err)

	_, err = storage.Get("test")
	require.Error(t, err)
}

func TestLayeredStorageAllLayersNil(t *testing.T) {
	t.Parallel()

	storage := New(LayeredConfig{})
	err := storage.Start(Options{})
	require.Error(t, err)
	require.Equal(t, ErrAllLayersNil, err)
}

func TestLayeredStorageCachePopulation(t *testing.T) {
	t.Parallel()

	// Create a mock embedded layer using memory
	embedded := NewMemoryLayer()
	err := embedded.Start(LayerOptions{})
	require.NoError(t, err)

	// Pre-populate embedded layer
	obj1 := testObject("test/1", testData, 1)
	obj2 := testObject("test/2", testDataUpdate, 2)
	embedded.Set("test/1", &obj1)
	embedded.Set("test/2", &obj2)

	// Create memory layer
	memory := NewMemoryLayer()

	storage := New(LayeredConfig{
		Memory:   memory,
		Embedded: &mockEmbeddedLayer{layer: embedded},
	})
	err = storage.Start(Options{})
	require.NoError(t, err)
	defer storage.Close()

	// Memory should now have the data from embedded
	obj, err := memory.Get("test/1")
	require.NoError(t, err)
	require.Equal(t, testData, json.RawMessage(obj.Data))

	obj, err = memory.Get("test/2")
	require.NoError(t, err)
	require.Equal(t, testDataUpdate, json.RawMessage(obj.Data))
}

func TestLayeredStorageWriteThrough(t *testing.T) {
	t.Parallel()

	memory := NewMemoryLayer()
	embedded := NewMemoryLayer()

	storage := New(LayeredConfig{
		Memory: memory,
		MemoryOptions: LayerOptions{
			SkipLoadMemory: true,
		},
		Embedded: &mockEmbeddedLayer{layer: embedded},
	})
	err := storage.Start(Options{})
	require.NoError(t, err)
	defer storage.Close()

	// Write should go to both layers
	_, err = storage.Set("test", testData)
	require.NoError(t, err)

	// Verify in memory
	obj, err := memory.Get("test")
	require.NoError(t, err)
	require.Equal(t, testData, json.RawMessage(obj.Data))

	// Verify in embedded
	obj, err = embedded.Get("test")
	require.NoError(t, err)
	require.Equal(t, testData, json.RawMessage(obj.Data))

	// Delete should remove from both
	err = storage.Del("test")
	require.NoError(t, err)

	_, err = memory.Get("test")
	require.Error(t, err)

	_, err = embedded.Get("test")
	require.Error(t, err)
}

func TestLayeredStorageCacheMiss(t *testing.T) {
	t.Parallel()

	memory := NewMemoryLayer()
	embedded := NewMemoryLayer()

	// Pre-populate only embedded layer
	err := embedded.Start(LayerOptions{})
	require.NoError(t, err)
	obj := testObject("test", testData, 1)
	embedded.Set("test", &obj)

	storage := New(LayeredConfig{
		Memory: memory,
		MemoryOptions: LayerOptions{
			SkipLoadMemory: true, // Don't init cache
		},
		Embedded: &mockEmbeddedLayer{layer: embedded},
	})

	err = storage.Start(Options{})
	require.NoError(t, err)
	defer storage.Close()

	// Memory should be empty initially
	_, err = memory.Get("test")
	require.Error(t, err)

	// Get from layered should find it in embedded and populate memory
	obj, err = storage.Get("test")
	require.NoError(t, err)
	require.Equal(t, testData, json.RawMessage(obj.Data))

	// Now memory should have it
	obj, err = memory.Get("test")
	require.NoError(t, err)
	require.Equal(t, testData, json.RawMessage(obj.Data))
}

// Helper to create test objects
func testObject(path string, data json.RawMessage, created int64) meta.Object {
	return meta.Object{
		Created: created,
		Updated: 0,
		Index:   path,
		Path:    path,
		Data:    data,
	}
}

// mockEmbeddedLayer wraps a MemoryLayer to implement EmbeddedLayer
type mockEmbeddedLayer struct {
	layer *MemoryLayer
}

func (m *mockEmbeddedLayer) Active() bool {
	return m.layer.Active()
}

func (m *mockEmbeddedLayer) Start(opt LayerOptions) error {
	return m.layer.Start(opt)
}

func (m *mockEmbeddedLayer) Close() {
	m.layer.Close()
}

func (m *mockEmbeddedLayer) Get(key string) (meta.Object, error) {
	return m.layer.Get(key)
}

func (m *mockEmbeddedLayer) GetList(path string) ([]meta.Object, error) {
	return m.layer.GetList(path)
}

func (m *mockEmbeddedLayer) Set(key string, obj *meta.Object) error {
	return m.layer.Set(key, obj)
}

func (m *mockEmbeddedLayer) Del(key string) error {
	return m.layer.Del(key)
}

func (m *mockEmbeddedLayer) Keys() ([]string, error) {
	return m.layer.Keys()
}

func (m *mockEmbeddedLayer) Clear() {
	m.layer.Clear()
}

func (m *mockEmbeddedLayer) Load() (map[string]*meta.Object, error) {
	keys, err := m.layer.Keys()
	if err != nil {
		return nil, err
	}

	data := make(map[string]*meta.Object)
	for _, k := range keys {
		obj, err := m.layer.Get(k)
		if err != nil {
			continue
		}
		data[k] = &obj
	}
	return data, nil
}
