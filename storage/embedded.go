package storage

import (
	"github.com/benitogf/ooo/meta"
)

// EmbeddedLayer is the interface for embedded storage backends (leveldb, etc.)
// Implementations should handle their own persistence
type EmbeddedLayer interface {
	Layer
	// Load reads all data from persistent storage (called during initialization)
	Load() (map[string]*meta.Object, error)
}

// EmbeddedWrapper wraps an EmbeddedLayer to provide Layer interface with caching disabled
// This is used when the embedded layer is the only layer or when caching is handled elsewhere
type EmbeddedWrapper struct {
	backend EmbeddedLayer
}

// NewEmbeddedWrapper creates a wrapper around an embedded layer
func NewEmbeddedWrapper(backend EmbeddedLayer) *EmbeddedWrapper {
	return &EmbeddedWrapper{backend: backend}
}

// Active returns whether the layer is active
func (e *EmbeddedWrapper) Active() bool {
	return e.backend.Active()
}

// Start initializes the embedded layer
func (e *EmbeddedWrapper) Start(opt LayerOptions) error {
	return e.backend.Start(opt)
}

// Close shuts down the embedded layer
func (e *EmbeddedWrapper) Close() {
	e.backend.Close()
}

// Get retrieves a single value by exact key
func (e *EmbeddedWrapper) Get(key string) (meta.Object, error) {
	return e.backend.Get(key)
}

// GetList retrieves all values matching a glob pattern
func (e *EmbeddedWrapper) GetList(path string) ([]meta.Object, error) {
	return e.backend.GetList(path)
}

// Set stores a value
func (e *EmbeddedWrapper) Set(key string, obj *meta.Object) error {
	return e.backend.Set(key, obj)
}

// Del deletes a key
func (e *EmbeddedWrapper) Del(key string) error {
	return e.backend.Del(key)
}

// Keys returns all keys
func (e *EmbeddedWrapper) Keys() ([]string, error) {
	return e.backend.Keys()
}

// Clear removes all data
func (e *EmbeddedWrapper) Clear() {
	e.backend.Clear()
}

// Load reads all data from persistent storage
func (e *EmbeddedWrapper) Load() (map[string]*meta.Object, error) {
	return e.backend.Load()
}
