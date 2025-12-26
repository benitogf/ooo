package storage

import (
	"sort"
	"strings"
	"sync"

	"github.com/benitogf/ooo/key"
	"github.com/benitogf/ooo/meta"
)

// MemoryLayer is an in-memory storage layer
type MemoryLayer struct {
	data   map[string]*meta.Object
	mutex  sync.RWMutex
	active bool
}

// NewMemoryLayer creates a new memory layer
func NewMemoryLayer() *MemoryLayer {
	return &MemoryLayer{
		data: make(map[string]*meta.Object),
	}
}

// Active returns whether the layer is active
func (m *MemoryLayer) Active() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.active
}

// Start initializes the memory layer
func (m *MemoryLayer) Start(opt LayerOptions) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.active = true
	return nil
}

// Close shuts down the memory layer
func (m *MemoryLayer) Close() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.active = false
}

// Get retrieves a single value by exact key
func (m *MemoryLayer) Get(k string) (meta.Object, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	obj, found := m.data[k]
	if !found {
		return meta.Object{}, ErrNotFound
	}

	return *obj, nil
}

// GetList retrieves all values matching a glob pattern
func (m *MemoryLayer) GetList(path string) ([]meta.Object, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if !key.HasGlob(path) {
		return nil, ErrInvalidPattern
	}

	// Pre-allocate with reasonable capacity to reduce growslice overhead
	res := make([]meta.Object, 0, min(len(m.data), 64))
	for k, obj := range m.data {
		if key.Match(path, k) {
			res = append(res, *obj)
		}
	}

	return res, nil
}

// Set stores a value
func (m *MemoryLayer) Set(k string, obj *meta.Object) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.data[k] = obj
	return nil
}

// Del deletes a key
func (m *MemoryLayer) Del(k string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !key.HasGlob(k) {
		_, found := m.data[k]
		if !found {
			return ErrNotFound
		}
		delete(m.data, k)
		return nil
	}

	// Glob delete
	for dk := range m.data {
		if key.Match(k, dk) {
			delete(m.data, dk)
		}
	}
	return nil
}

// Keys returns all keys
func (m *MemoryLayer) Keys() ([]string, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	keys := make([]string, 0, len(m.data))
	for k := range m.data {
		keys = append(keys, k)
	}

	sort.Slice(keys, func(i, j int) bool {
		return strings.ToLower(keys[i]) < strings.ToLower(keys[j])
	})

	return keys, nil
}

// Clear removes all data
func (m *MemoryLayer) Clear() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.data = make(map[string]*meta.Object)
}

// Len returns the number of entries (for testing)
func (m *MemoryLayer) Len() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return len(m.data)
}
