package storage

import (
	"container/list"
	"sort"
	"strings"
	"sync"

	"github.com/benitogf/ooo/key"
	"github.com/benitogf/ooo/meta"
)

// MemoryLayer is an in-memory storage layer with optional LRU eviction
type MemoryLayer struct {
	data       map[string]*meta.Object
	lruList    *list.List
	lruMap     map[string]*list.Element
	maxEntries int
	mutex      sync.RWMutex
	active     bool
}

// NewMemoryLayer creates a new memory layer
func NewMemoryLayer() *MemoryLayer {
	return &MemoryLayer{
		data:    make(map[string]*meta.Object),
		lruList: list.New(),
		lruMap:  make(map[string]*list.Element),
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
	m.maxEntries = opt.MaxEntries
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
	m.mutex.Lock()
	defer m.mutex.Unlock()

	obj, found := m.data[k]
	if !found {
		return meta.Object{}, ErrNotFound
	}

	// Update LRU
	if elem, ok := m.lruMap[k]; ok {
		m.lruList.MoveToFront(elem)
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

	res := []meta.Object{}
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

	// Check if key exists
	_, exists := m.data[k]

	// Store the object
	m.data[k] = obj

	// Update LRU
	if exists {
		if elem, ok := m.lruMap[k]; ok {
			m.lruList.MoveToFront(elem)
		}
	} else {
		elem := m.lruList.PushFront(k)
		m.lruMap[k] = elem
	}

	// Evict if necessary
	m.evictIfNeeded()

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
		if elem, ok := m.lruMap[k]; ok {
			m.lruList.Remove(elem)
			delete(m.lruMap, k)
		}
		return nil
	}

	// Glob delete
	for dk := range m.data {
		if key.Match(k, dk) {
			delete(m.data, dk)
			if elem, ok := m.lruMap[dk]; ok {
				m.lruList.Remove(elem)
				delete(m.lruMap, dk)
			}
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
	m.lruList = list.New()
	m.lruMap = make(map[string]*list.Element)
}

// evictIfNeeded removes the least recently used entry if over capacity
// Must be called with mutex held
func (m *MemoryLayer) evictIfNeeded() {
	if m.maxEntries <= 0 {
		return // No limit
	}

	for len(m.data) > m.maxEntries {
		elem := m.lruList.Back()
		if elem == nil {
			break
		}
		k := elem.Value.(string)
		delete(m.data, k)
		m.lruList.Remove(elem)
		delete(m.lruMap, k)
	}
}

// Len returns the number of entries (for testing)
func (m *MemoryLayer) Len() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return len(m.data)
}

// SetMaxEntries updates the max entries limit
func (m *MemoryLayer) SetMaxEntries(max int) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.maxEntries = max
	m.evictIfNeeded()
}
