package storage

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/benitogf/go-json"
	"github.com/benitogf/ooo/meta"
	"github.com/benitogf/ooo/monotonic"
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

func TestMemoryStorageGetN(t *testing.T) {
	t.Parallel()

	storage := New(LayeredConfig{
		Memory: NewMemoryLayer(),
	})
	err := storage.Start(Options{})
	require.NoError(t, err)
	defer storage.Close()

	// Add entries with different created times
	for i := range 5 {
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
		Memory:   memory,
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

func TestWatchWithCallback(t *testing.T) {
	t.Parallel()

	storage := New(LayeredConfig{
		Memory: NewMemoryLayer(),
	})
	err := storage.Start(Options{})
	require.NoError(t, err)
	defer storage.Close()

	// Track events received
	var events []Event
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Start watching with callback
	WatchWithCallback(storage, func(event Event) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
		wg.Done()
	})

	// Expect 3 events: set, set, delete
	wg.Add(3)

	// Trigger events
	_, err = storage.Set("test/1", testData)
	require.NoError(t, err)

	_, err = storage.Set("test/2", testDataUpdate)
	require.NoError(t, err)

	err = storage.Del("test/1")
	require.NoError(t, err)

	// Wait for all events
	wg.Wait()

	// Verify events (order may vary due to sharded channels)
	mu.Lock()
	require.Equal(t, 3, len(events))

	// Count operations by type
	setCount := 0
	delCount := 0
	for _, e := range events {
		switch e.Operation {
		case "set":
			setCount++
		case "del":
			delCount++
		}
	}
	require.Equal(t, 2, setCount, "should have 2 set events")
	require.Equal(t, 1, delCount, "should have 1 delete event")
	mu.Unlock()
}

func TestWatchStorageNoop(t *testing.T) {
	t.Parallel()

	storage := New(LayeredConfig{
		Memory: NewMemoryLayer(),
	})
	err := storage.Start(Options{})
	require.NoError(t, err)
	defer storage.Close()

	// Start noop watcher - should drain events without blocking
	WatchStorageNoop(storage)

	// These should not block even though no one is reading events
	_, err = storage.Set("test/1", testData)
	require.NoError(t, err)

	_, err = storage.Set("test/2", testDataUpdate)
	require.NoError(t, err)

	err = storage.Del("test/1")
	require.NoError(t, err)

	// Verify storage operations completed
	obj, err := storage.Get("test/2")
	require.NoError(t, err)
	require.Equal(t, testDataUpdate, json.RawMessage(obj.Data))
}

func TestSetBeforeRead(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var callCount int
	var lastKey string

	storage := New(LayeredConfig{
		Memory: NewMemoryLayer(),
	})

	// Start WITHOUT BeforeRead callback initially
	err := storage.Start(Options{})
	require.NoError(t, err)
	defer storage.Close()

	// Set some data (no callback yet)
	_, err = storage.Set("test/1", testData)
	require.NoError(t, err)

	// Now set BeforeRead using SetBeforeRead on active storage
	callback := func(key string) {
		mu.Lock()
		callCount++
		lastKey = key
		mu.Unlock()
	}
	storage.SetBeforeRead(callback)

	// Read should trigger BeforeRead
	_, err = storage.Get("test/1")
	require.NoError(t, err)

	mu.Lock()
	require.Equal(t, 1, callCount)
	require.Equal(t, "test/1", lastKey)
	mu.Unlock()

	// Test with GetList
	_, err = storage.GetList("test/*")
	require.NoError(t, err)

	mu.Lock()
	require.Equal(t, 2, callCount)
	require.Equal(t, "test/*", lastKey)
	mu.Unlock()

	// Update BeforeRead to a new callback
	var newCallCount int
	var newLastKey string
	newCallback := func(key string) {
		mu.Lock()
		newCallCount++
		newLastKey = key
		mu.Unlock()
	}
	storage.SetBeforeRead(newCallback)

	// Read should now trigger the new callback
	_, err = storage.Get("test/1")
	require.NoError(t, err)

	mu.Lock()
	require.Equal(t, 2, callCount, "old callback count should remain unchanged")
	require.Equal(t, 1, newCallCount, "new callback should be called")
	require.Equal(t, "test/1", newLastKey)
	mu.Unlock()

	// Set BeforeRead to nil should disable callback
	storage.SetBeforeRead(nil)

	_, err = storage.Get("test/1")
	require.NoError(t, err)

	mu.Lock()
	require.Equal(t, 1, newCallCount, "callback count should not increase when nil")
	mu.Unlock()
}

func TestSetBeforeReadConcurrent(t *testing.T) {
	t.Parallel()

	storage := New(LayeredConfig{
		Memory: NewMemoryLayer(),
	})
	err := storage.Start(Options{})
	require.NoError(t, err)
	defer storage.Close()

	// Set some data
	_, err = storage.Set("test/1", testData)
	require.NoError(t, err)

	var wg sync.WaitGroup
	var callCount int
	var mu sync.Mutex

	// Concurrently update BeforeRead and perform reads
	for i := range 10 {
		wg.Add(2)

		// Goroutine to update BeforeRead
		go func(idx int) {
			defer wg.Done()
			storage.SetBeforeRead(func(key string) {
				mu.Lock()
				callCount++
				mu.Unlock()
			})
		}(i)

		// Goroutine to perform reads
		go func() {
			defer wg.Done()
			storage.Get("test/1")
		}()
	}

	wg.Wait()

	// Should complete without race conditions or panics
	// The exact call count depends on timing, but should be <= 10
	mu.Lock()
	require.LessOrEqual(t, callCount, 10)
	mu.Unlock()
}

// TestSendTimeoutPreventsWatchSelfDeadlock verifies that Send with a timeout
// prevents the watch goroutine from deadlocking when it writes back to storage
// (e.g. version vector updates) while processing an event.
//
// The deadlock scenario:
//  1. Watch goroutine for shard X receives an event
//  2. During processing (OnStorageEvent), it writes to storage (VV increment)
//  3. That write calls sendEvent(), which sends to a shard channel
//  4. If that event hashes to shard X AND the buffer is full, the watch
//     goroutine blocks trying to send to its own channel — self-deadlock
//
// Uses SendWithTimeout directly with a short timeout to keep the test fast.
func TestSendTimeoutPreventsWatchSelfDeadlock(t *testing.T) {
	t.Parallel()

	sc := NewShardedChan(1) // single shard = guaranteed self-send
	defer sc.Close()

	// Fill the channel buffer completely (capacity 100)
	for i := range 100 {
		sc.Send(Event{Key: "fill/" + string(rune('a'+i%26)) + string(rune('0'+i/26))})
	}

	// Now simulate the self-deadlock: the watch goroutine (consumer of shard 0)
	// tries to send to shard 0 while the buffer is full.
	// Without timeout, this blocks forever.
	// With timeout, it returns false.
	ok := sc.SendWithTimeout(Event{Key: "self/deadlock"}, 1*time.Millisecond)
	require.False(t, ok, "send to full channel should time out, not block forever")
}

// holdingEmbeddedLayer wraps a MemoryLayer and pauses the first Set call to
// holdKey until release is closed, signalling blocked once that Set has parked.
// Used to deterministically interleave two concurrent writers across the
// memory→embedded boundary.
type holdingEmbeddedLayer struct {
	inner   *MemoryLayer
	holdKey string
	blocked chan struct{}
	release chan struct{}
	once    sync.Once
}

func (h *holdingEmbeddedLayer) Active() bool               { return h.inner.Active() }
func (h *holdingEmbeddedLayer) Start(o LayerOptions) error { return h.inner.Start(o) }
func (h *holdingEmbeddedLayer) Close()                     { h.inner.Close() }
func (h *holdingEmbeddedLayer) Get(k string) (meta.Object, error) {
	return h.inner.Get(k)
}
func (h *holdingEmbeddedLayer) GetList(p string) ([]meta.Object, error) {
	return h.inner.GetList(p)
}
func (h *holdingEmbeddedLayer) Set(k string, o *meta.Object) error {
	if k == h.holdKey {
		first := false
		h.once.Do(func() { first = true })
		if first {
			close(h.blocked)
			<-h.release
		}
	}
	return h.inner.Set(k, o)
}
func (h *holdingEmbeddedLayer) Del(k string) error      { return h.inner.Del(k) }
func (h *holdingEmbeddedLayer) Keys() ([]string, error) { return h.inner.Keys() }
func (h *holdingEmbeddedLayer) Clear()                  { h.inner.Clear() }
func (h *holdingEmbeddedLayer) Load() (map[string]*meta.Object, error) {
	keys, err := h.inner.Keys()
	if err != nil {
		return nil, err
	}
	data := make(map[string]*meta.Object, len(keys))
	for _, k := range keys {
		obj, err := h.inner.Get(k)
		if err != nil {
			continue
		}
		data[k] = &obj
	}
	return data, nil
}

// TestLayeredConcurrentSetMirrorInvariant drives two concurrent Set calls to
// the same key with the embedded layer artificially paused mid-write. Without
// per-key serialization across both layers, writer B can complete entirely
// while writer A is mid-write, leaving memory and embedded with different
// values — breaking the invariant that memory mirrors embedded.
func TestLayeredConcurrentSetMirrorInvariant(t *testing.T) {
	embedded := &holdingEmbeddedLayer{
		inner:   NewMemoryLayer(),
		holdKey: "k",
		blocked: make(chan struct{}),
		release: make(chan struct{}),
	}
	memory := NewMemoryLayer()
	s := New(LayeredConfig{Memory: memory, Embedded: embedded})
	require.NoError(t, s.Start(Options{}))
	defer s.Close()

	aDone := make(chan error, 1)
	go func() {
		_, err := s.Set("k", json.RawMessage(`"A"`))
		aDone <- err
	}()

	select {
	case <-embedded.blocked:
	case <-time.After(2 * time.Second):
		t.Fatal("writer A never reached embedded.Set")
	}

	bDone := make(chan error, 1)
	go func() {
		_, err := s.Set("k", json.RawMessage(`"B"`))
		bDone <- err
	}()

	bFinishedFirst := false
	select {
	case err := <-bDone:
		require.NoError(t, err)
		bFinishedFirst = true
	case <-time.After(300 * time.Millisecond):
	}

	close(embedded.release)
	require.NoError(t, <-aDone)
	if !bFinishedFirst {
		require.NoError(t, <-bDone)
	}

	memObj, err := memory.Get("k")
	require.NoError(t, err)
	embObj, err := embedded.inner.Get("k")
	require.NoError(t, err)
	require.Equal(t, string(memObj.Data), string(embObj.Data),
		"mirror invariant violated: memory=%s embedded=%s", string(memObj.Data), string(embObj.Data))
}

// TestLayeredCloseRaceWithBroadcast races concurrent writers (each broadcasts
// through the watcher) against Close (which nils the watcher). Without
// synchronization on the watcher field, `go test -race` flags the read in
// sendEvent against the write in Close. With the lock-protected read, the race
// is gone and an in-flight broadcast is held off long enough that Close's
// channel close cannot panic the sender.
func TestLayeredCloseRaceWithBroadcast(t *testing.T) {
	s := New(LayeredConfig{Memory: NewMemoryLayer()})
	require.NoError(t, s.Start(Options{}))

	const writers = 8
	const iters = 200
	started := make(chan struct{})
	var startOnce sync.Once
	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			defer func() { _ = recover() }()
			startOnce.Do(func() { close(started) })
			for n := range iters {
				_, _ = s.Set(fmt.Sprintf("k/%d/%d", id, n), json.RawMessage(`"v"`))
			}
		}(i)
	}

	<-started
	time.Sleep(2 * time.Millisecond) // let writers reach the broadcast path
	s.Close()
	wg.Wait()
}

// faultyEmbeddedLayer wraps a MemoryLayer but returns the configured errors
// from Set / Del so tests can simulate persistence failures.
type faultyEmbeddedLayer struct {
	inner  *MemoryLayer
	setErr error
	delErr error
}

func (f *faultyEmbeddedLayer) Active() bool                      { return f.inner.Active() }
func (f *faultyEmbeddedLayer) Start(o LayerOptions) error        { return f.inner.Start(o) }
func (f *faultyEmbeddedLayer) Close()                            { f.inner.Close() }
func (f *faultyEmbeddedLayer) Get(k string) (meta.Object, error) { return f.inner.Get(k) }
func (f *faultyEmbeddedLayer) GetList(p string) ([]meta.Object, error) {
	return f.inner.GetList(p)
}
func (f *faultyEmbeddedLayer) Set(k string, o *meta.Object) error {
	if f.setErr != nil {
		return f.setErr
	}
	return f.inner.Set(k, o)
}
func (f *faultyEmbeddedLayer) Del(k string) error {
	if f.delErr != nil {
		return f.delErr
	}
	return f.inner.Del(k)
}
func (f *faultyEmbeddedLayer) Keys() ([]string, error) { return f.inner.Keys() }
func (f *faultyEmbeddedLayer) Clear()                  { f.inner.Clear() }
func (f *faultyEmbeddedLayer) Load() (map[string]*meta.Object, error) {
	keys, err := f.inner.Keys()
	if err != nil {
		return nil, err
	}
	data := make(map[string]*meta.Object, len(keys))
	for _, k := range keys {
		obj, err := f.inner.Get(k)
		if err != nil {
			continue
		}
		data[k] = &obj
	}
	return data, nil
}

// TestLayeredSetReturnsEmbeddedError asserts the embedded layer's Set error is
// propagated to the caller of Layered.Set instead of being silently swallowed.
func TestLayeredSetReturnsEmbeddedError(t *testing.T) {
	t.Parallel()

	embeddedErr := errors.New("disk full")
	embedded := &faultyEmbeddedLayer{inner: NewMemoryLayer(), setErr: embeddedErr}
	s := New(LayeredConfig{Memory: NewMemoryLayer(), Embedded: embedded})
	require.NoError(t, s.Start(Options{}))
	defer s.Close()

	_, err := s.Set("k", json.RawMessage(`{"v":1}`))
	require.ErrorIs(t, err, embeddedErr,
		"Set must return the embedded layer's error")
}

// TestLayeredSetRollsBackMemoryOnEmbeddedFailure asserts that a Set call which
// fails at the embedded layer leaves storage state unchanged — the in-memory
// layer must not retain the rejected write. Pre-fix, memory was updated first
// and kept the uncommitted value when embedded returned an error: in-process
// Get returned data the durable store rejected, restart silently rolled back
// the value, and any caller relying on "Set returned an error → state
// unchanged" was wrong.
func TestLayeredSetRollsBackMemoryOnEmbeddedFailure(t *testing.T) {
	t.Parallel()

	// Seed an initial committed value so we can verify the rollback restores it.
	embedded := &faultyEmbeddedLayer{inner: NewMemoryLayer()}
	s := New(LayeredConfig{Memory: NewMemoryLayer(), Embedded: embedded})
	require.NoError(t, s.Start(Options{}))
	defer s.Close()

	_, err := s.Set("k", json.RawMessage(`{"v":1}`))
	require.NoError(t, err)

	before, err := s.Get("k")
	require.NoError(t, err)
	require.Contains(t, string(before.Data), `"v":1`)

	// Make the next Set fail at the embedded layer.
	embedded.setErr = errors.New("disk full")

	_, err = s.Set("k", json.RawMessage(`{"v":2}`))
	require.Error(t, err)

	// State must equal the prior committed value, not the rejected v:2.
	after, err := s.Get("k")
	require.NoError(t, err)
	require.Contains(t, string(after.Data), `"v":1`,
		"failed Set must not leave the rejected value in memory")
	require.NotContains(t, string(after.Data), `"v":2`,
		"memory must roll back when the embedded layer rejected the write")
}

// TestLayeredSetRollsBackOnFirstWriteEmbeddedFailure covers the case where the
// key did not exist before the failed Set: storage must end up empty, not
// holding the rejected value in memory.
func TestLayeredSetRollsBackOnFirstWriteEmbeddedFailure(t *testing.T) {
	t.Parallel()

	embedded := &faultyEmbeddedLayer{inner: NewMemoryLayer(), setErr: errors.New("disk full")}
	s := New(LayeredConfig{Memory: NewMemoryLayer(), Embedded: embedded})
	require.NoError(t, s.Start(Options{}))
	defer s.Close()

	_, err := s.Set("k", json.RawMessage(`{"v":1}`))
	require.Error(t, err)

	_, err = s.Get("k")
	require.Error(t, err, "Get must fail for a key whose Set was rejected by embedded")
}

// TestLayeredPushRollsBackOnEmbeddedFailure asserts a Push rejected by the
// embedded layer does not leave the newly-generated key visible in memory.
func TestLayeredPushRollsBackOnEmbeddedFailure(t *testing.T) {
	t.Parallel()

	embedded := &faultyEmbeddedLayer{inner: NewMemoryLayer(), setErr: errors.New("disk full")}
	s := New(LayeredConfig{Memory: NewMemoryLayer(), Embedded: embedded})
	require.NoError(t, s.Start(Options{}))
	defer s.Close()

	_, err := s.Push("things/*", json.RawMessage(`{"v":1}`))
	require.Error(t, err)

	items, err := s.GetList("things/*")
	require.NoError(t, err)
	require.Empty(t, items, "rejected Push must not leave the key in memory")
}

// TestLayeredSetWithMetaRollsBackOnEmbeddedFailure asserts SetWithMeta restores
// the prior committed value when embedded rejects the new one.
func TestLayeredSetWithMetaRollsBackOnEmbeddedFailure(t *testing.T) {
	t.Parallel()

	embedded := &faultyEmbeddedLayer{inner: NewMemoryLayer()}
	s := New(LayeredConfig{Memory: NewMemoryLayer(), Embedded: embedded})
	require.NoError(t, s.Start(Options{}))
	defer s.Close()

	_, err := s.SetWithMeta("k", json.RawMessage(`{"v":1}`), 1, 2)
	require.NoError(t, err)

	embedded.setErr = errors.New("disk full")
	_, err = s.SetWithMeta("k", json.RawMessage(`{"v":2}`), 1, 3)
	require.Error(t, err)

	got, err := s.Get("k")
	require.NoError(t, err)
	require.Contains(t, string(got.Data), `"v":1`,
		"failed SetWithMeta must not leave the rejected value in memory")
}

// TestLayeredDelRollsBackOnEmbeddedFailure asserts Del restores the prior
// value in memory when embedded refuses the delete.
func TestLayeredDelRollsBackOnEmbeddedFailure(t *testing.T) {
	t.Parallel()

	embedded := &faultyEmbeddedLayer{inner: NewMemoryLayer()}
	s := New(LayeredConfig{Memory: NewMemoryLayer(), Embedded: embedded})
	require.NoError(t, s.Start(Options{}))
	defer s.Close()

	_, err := s.Set("k", json.RawMessage(`{"v":1}`))
	require.NoError(t, err)

	embedded.delErr = errors.New("disk locked")
	err = s.Del("k")
	require.Error(t, err)

	got, err := s.Get("k")
	require.NoError(t, err, "rejected Del must leave the key visible in memory")
	require.Contains(t, string(got.Data), `"v":1`)
}

// TestLayeredDelSilentRollsBackOnEmbeddedFailure asserts DelSilent (non-glob)
// restores memory when embedded refuses the delete. The LimitFilter retention
// path uses DelSilent and silent rollback there would mid-evict-some-keys
// leaving memory ahead of embedded.
func TestLayeredDelSilentRollsBackOnEmbeddedFailure(t *testing.T) {
	t.Parallel()

	embedded := &faultyEmbeddedLayer{inner: NewMemoryLayer()}
	s := New(LayeredConfig{Memory: NewMemoryLayer(), Embedded: embedded})
	require.NoError(t, s.Start(Options{}))
	defer s.Close()

	_, err := s.Set("k", json.RawMessage(`{"v":1}`))
	require.NoError(t, err)

	embedded.delErr = errors.New("disk locked")
	err = s.DelSilent("k")
	require.Error(t, err)

	got, err := s.Get("k")
	require.NoError(t, err, "rejected DelSilent must leave the key visible in memory")
	require.Contains(t, string(got.Data), `"v":1`)
}

// TestLayeredDelGlobRollsBackOnEmbeddedFailure asserts the glob-delete path
// restores every removed item to memory when embedded refuses the delete.
func TestLayeredDelGlobRollsBackOnEmbeddedFailure(t *testing.T) {
	t.Parallel()

	embedded := &faultyEmbeddedLayer{inner: NewMemoryLayer()}
	s := New(LayeredConfig{Memory: NewMemoryLayer(), Embedded: embedded})
	require.NoError(t, s.Start(Options{}))
	defer s.Close()

	_, err := s.Set("items/a", json.RawMessage(`{"v":1}`))
	require.NoError(t, err)
	_, err = s.Set("items/b", json.RawMessage(`{"v":2}`))
	require.NoError(t, err)

	embedded.delErr = errors.New("disk locked")
	err = s.Del("items/*")
	require.Error(t, err)

	items, err := s.GetList("items/*")
	require.NoError(t, err)
	require.Len(t, items, 2, "rejected glob Del must leave both keys visible in memory")
}

// TestLayeredDelSilentGlobRollsBackOnEmbeddedFailure is the DelSilent variant
// of the glob rollback.
func TestLayeredDelSilentGlobRollsBackOnEmbeddedFailure(t *testing.T) {
	t.Parallel()

	embedded := &faultyEmbeddedLayer{inner: NewMemoryLayer()}
	s := New(LayeredConfig{Memory: NewMemoryLayer(), Embedded: embedded})
	require.NoError(t, s.Start(Options{}))
	defer s.Close()

	_, err := s.Set("items/a", json.RawMessage(`{"v":1}`))
	require.NoError(t, err)
	_, err = s.Set("items/b", json.RawMessage(`{"v":2}`))
	require.NoError(t, err)

	embedded.delErr = errors.New("disk locked")
	err = s.DelSilent("items/*")
	require.Error(t, err)

	items, err := s.GetList("items/*")
	require.NoError(t, err)
	require.Len(t, items, 2, "rejected DelSilent glob must leave both keys visible in memory")
}

// syncEmbedded wraps a MemoryLayer and lets a test pause embedded.Del or
// embedded.Clear between "started" and "executed" so a concurrent Set can
// race with the in-progress write barrier deterministically.
type syncEmbedded struct {
	inner        *MemoryLayer
	delStarted   chan struct{}
	delResume    chan struct{}
	clearStarted chan struct{}
	clearResume  chan struct{}
}

func (s *syncEmbedded) Active() bool                      { return s.inner.Active() }
func (s *syncEmbedded) Start(o LayerOptions) error        { return s.inner.Start(o) }
func (s *syncEmbedded) Close()                            { s.inner.Close() }
func (s *syncEmbedded) Get(k string) (meta.Object, error) { return s.inner.Get(k) }
func (s *syncEmbedded) GetList(p string) ([]meta.Object, error) {
	return s.inner.GetList(p)
}
func (s *syncEmbedded) Set(k string, o *meta.Object) error { return s.inner.Set(k, o) }
func (s *syncEmbedded) Del(k string) error {
	if s.delStarted != nil {
		select {
		case s.delStarted <- struct{}{}:
		default:
		}
	}
	if s.delResume != nil {
		<-s.delResume
	}
	return s.inner.Del(k)
}
func (s *syncEmbedded) Keys() ([]string, error) { return s.inner.Keys() }
func (s *syncEmbedded) Clear() {
	if s.clearStarted != nil {
		select {
		case s.clearStarted <- struct{}{}:
		default:
		}
	}
	if s.clearResume != nil {
		<-s.clearResume
	}
	s.inner.Clear()
}
func (s *syncEmbedded) Load() (map[string]*meta.Object, error) {
	keys, err := s.inner.Keys()
	if err != nil {
		return nil, err
	}
	data := make(map[string]*meta.Object, len(keys))
	for _, k := range keys {
		obj, err := s.inner.Get(k)
		if err != nil {
			continue
		}
		data[k] = &obj
	}
	return data, nil
}

// TestLayeredDelGlobSerializesWithConcurrentSet reproduces the race the audit
// flagged: a Set that commits between the memory and embedded halves of a
// glob delete used to leave the layers diverged. The fix serializes
// individual writes against glob deletes via a write-side RWMutex — Del(glob)
// takes the exclusive lock, Set takes the shared one — so the Set can no
// longer interleave with the in-progress delete and both layers end up
// agreeing on whether the key exists.
func TestLayeredDelGlobSerializesWithConcurrentSet(t *testing.T) {
	t.Parallel()

	delStarted := make(chan struct{}, 1)
	delResume := make(chan struct{})
	embedded := &syncEmbedded{
		inner:      NewMemoryLayer(),
		delStarted: delStarted,
		delResume:  delResume,
	}
	s := New(LayeredConfig{Memory: NewMemoryLayer(), Embedded: embedded})
	require.NoError(t, s.Start(Options{NoBroadcastKeys: []string{"items/a", "items/*"}}))
	defer s.Close()

	// Drain watcher events so Set/Del don't block on a 5s send timeout.
	sharded := s.WatchSharded()
	for i := range sharded.Count() {
		shard := sharded.Shard(i)
		go func() {
			for range shard {
			}
		}()
	}

	// Start a glob delete. It enters embedded.Del and blocks.
	delDone := make(chan error, 1)
	go func() {
		delDone <- s.Del("items/*")
	}()
	<-delStarted

	// Attempt a Set while the delete is mid-flight. With the fix, Set must
	// not commit until the glob delete releases its exclusive lock.
	setStarted := make(chan struct{})
	setDone := make(chan error, 1)
	go func() {
		close(setStarted)
		_, err := s.Set("items/a", json.RawMessage(`{"v":1}`))
		setDone <- err
	}()
	<-setStarted

	// Set must be blocked: it should not finish while Del is still paused.
	select {
	case err := <-setDone:
		t.Fatalf("Set must wait for Del(glob): unexpected early completion err=%v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(delResume)
	require.NoError(t, <-delDone)
	require.NoError(t, <-setDone)

	// After both finish: layers must agree.
	memItems, err := s.GetList("items/*")
	require.NoError(t, err)
	embKeys, err := embedded.inner.Keys()
	require.NoError(t, err)

	memHas := len(memItems) > 0
	embHas := len(embKeys) > 0
	require.Equal(t, memHas, embHas,
		"layers diverged: memory has key=%v, embedded has key=%v", memHas, embHas)
}

// TestLayeredClearSerializesWithConcurrentSet asserts Layered.Clear is
// serialized against concurrent Sets via the same writeMutex that
// Del(glob) takes. Pre-fix Clear called memory.Clear() then
// embedded.Clear() with no write barrier — a Set that committed between
// the two halves left memory holding the new value while embedded
// remained empty, diverging the layers.
//
// Mirrors TestLayeredDelGlobSerializesWithConcurrentSet using the same
// syncEmbedded hook, just on the Clear path.
func TestLayeredClearSerializesWithConcurrentSet(t *testing.T) {
	t.Parallel()

	clearStarted := make(chan struct{}, 1)
	clearResume := make(chan struct{})
	embedded := &syncEmbedded{
		inner:        NewMemoryLayer(),
		clearStarted: clearStarted,
		clearResume:  clearResume,
	}
	s := New(LayeredConfig{Memory: NewMemoryLayer(), Embedded: embedded})
	// Unlike the Del-glob sibling, Clear emits no watcher event, so the
	// "items/*" entry that test needs to mute delGlob's broadcast isn't
	// required here. Set on "items/a" still needs to skip broadcast.
	require.NoError(t, s.Start(Options{NoBroadcastKeys: []string{"items/a"}}))
	defer s.Close()

	// Drain watcher events so Set doesn't block on a 5s send timeout.
	sharded := s.WatchSharded()
	for i := range sharded.Count() {
		shard := sharded.Shard(i)
		go func() {
			for range shard {
			}
		}()
	}

	// Start Clear. memory.Clear() runs synchronously; embedded.Clear blocks
	// at the hook so a Set can race the "between halves" window.
	clearDone := make(chan struct{})
	go func() {
		s.Clear()
		close(clearDone)
	}()
	<-clearStarted

	// Attempt a Set while Clear is mid-flight. With the fix, Set takes the
	// shared writeMutex and must wait for the exclusive lock Clear holds.
	setStarted := make(chan struct{})
	setDone := make(chan error, 1)
	go func() {
		close(setStarted)
		_, err := s.Set("items/a", json.RawMessage(`{"v":1}`))
		setDone <- err
	}()
	<-setStarted

	// Set must be blocked: should not finish while Clear is still paused.
	select {
	case err := <-setDone:
		t.Fatalf("Set must wait for Clear: unexpected early completion err=%v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(clearResume)
	<-clearDone
	require.NoError(t, <-setDone)

	// After both finish: layers must agree on whether items/a exists.
	memItems, err := s.GetList("items/*")
	require.NoError(t, err)
	embKeys, err := embedded.inner.Keys()
	require.NoError(t, err)

	memHas := len(memItems) > 0
	embHas := len(embKeys) > 0
	require.Equal(t, memHas, embHas,
		"layers diverged after concurrent Clear+Set: memory has key=%v, embedded has key=%v", memHas, embHas)
}

// TestShardedChanDropIsObservable asserts that a send-timeout drop exposes
// both a counter and (when set) an OnDrop callback. Pre-fix the drop only
// logged — a stuck or recovered watcher silently desynced subscribers from
// storage with no programmatic signal for operators to monitor.
func TestShardedChanDropIsObservable(t *testing.T) {
	t.Parallel()

	sc := NewShardedChan(1)
	var dropped []Event
	var mu sync.Mutex
	sc.SetOnDrop(func(ev Event) {
		mu.Lock()
		dropped = append(dropped, ev)
		mu.Unlock()
	})

	// Fill the shard buffer (capacity 100 internally).
	for i := range 100 {
		require.True(t, sc.SendWithTimeout(Event{Key: fmt.Sprintf("ok-%d", i)}, time.Second))
	}

	// One more send with a tiny timeout: no consumer is reading, so it drops.
	sent := sc.SendWithTimeout(Event{Key: "drop-me"}, 10*time.Millisecond)
	require.False(t, sent, "send must time out when the shard buffer is saturated")

	require.Equal(t, int64(1), sc.Dropped(), "Dropped() must surface the drop")

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, dropped, 1, "OnDrop callback must fire on each drop")
	require.Equal(t, "drop-me", dropped[0].Key)
}

// TestShardedChanSetOnDropNilClears asserts SetOnDrop(nil) cleanly clears a
// previously-set callback.
func TestShardedChanSetOnDropNilClears(t *testing.T) {
	t.Parallel()

	sc := NewShardedChan(1)
	called := false
	sc.SetOnDrop(func(Event) { called = true })
	sc.SetOnDrop(nil)

	for i := range 100 {
		require.True(t, sc.SendWithTimeout(Event{Key: fmt.Sprintf("k-%d", i)}, time.Second))
	}
	_ = sc.SendWithTimeout(Event{Key: "drop"}, 10*time.Millisecond)
	require.False(t, called, "cleared OnDrop must not fire")
	require.Equal(t, int64(1), sc.Dropped(), "Dropped() still increments after SetOnDrop(nil)")
}

// TestLayeredPushReturnsEmbeddedError is the Push variant.
func TestLayeredPushReturnsEmbeddedError(t *testing.T) {
	t.Parallel()

	embeddedErr := errors.New("disk full")
	embedded := &faultyEmbeddedLayer{inner: NewMemoryLayer(), setErr: embeddedErr}
	s := New(LayeredConfig{Memory: NewMemoryLayer(), Embedded: embedded})
	require.NoError(t, s.Start(Options{}))
	defer s.Close()

	_, err := s.Push("things/*", json.RawMessage(`{"v":1}`))
	require.ErrorIs(t, err, embeddedErr,
		"Push must return the embedded layer's error")
}

// TestLayeredSetWithMetaReturnsEmbeddedError is the SetWithMeta variant.
func TestLayeredSetWithMetaReturnsEmbeddedError(t *testing.T) {
	t.Parallel()

	embeddedErr := errors.New("disk full")
	embedded := &faultyEmbeddedLayer{inner: NewMemoryLayer(), setErr: embeddedErr}
	s := New(LayeredConfig{Memory: NewMemoryLayer(), Embedded: embedded})
	require.NoError(t, s.Start(Options{}))
	defer s.Close()

	_, err := s.SetWithMeta("k", json.RawMessage(`{"v":1}`), 1, 2)
	require.ErrorIs(t, err, embeddedErr,
		"SetWithMeta must return the embedded layer's error")
}

// TestLayeredDelReturnsEmbeddedError asserts the embedded layer's Del error is
// propagated to the caller of Layered.Del.
func TestLayeredDelReturnsEmbeddedError(t *testing.T) {
	t.Parallel()

	embeddedErr := errors.New("disk locked")
	embedded := &faultyEmbeddedLayer{inner: NewMemoryLayer()}
	s := New(LayeredConfig{Memory: NewMemoryLayer(), Embedded: embedded})
	require.NoError(t, s.Start(Options{}))
	defer s.Close()

	_, err := s.Set("k", json.RawMessage(`{"v":1}`))
	require.NoError(t, err)

	embedded.delErr = embeddedErr

	err = s.Del("k")
	require.ErrorIs(t, err, embeddedErr,
		"Del must return the embedded layer's error")
}
