package storage

import (
	"strconv"
	"testing"

	"github.com/benitogf/ooo/meta"
	"github.com/goccy/go-json"
)

var benchData = json.RawMessage(`{"name": "benchmark", "value": 12345, "nested": {"a": 1, "b": 2}}`)

func BenchmarkMemoryLayerSet(b *testing.B) {
	layer := NewMemoryLayer()
	layer.Start(LayerOptions{})
	defer layer.Close()

	i := 0
	for b.Loop() {
		key := "test/" + strconv.Itoa(i)
		obj := &meta.Object{
			Created: int64(i),
			Updated: 0,
			Index:   strconv.Itoa(i),
			Path:    key,
			Data:    benchData,
		}
		layer.Set(key, obj)
		i++
	}
}

func BenchmarkMemoryLayerGet(b *testing.B) {
	layer := NewMemoryLayer()
	layer.Start(LayerOptions{})
	defer layer.Close()

	// Pre-populate
	for i := range 1000 {
		key := "test/" + strconv.Itoa(i)
		obj := &meta.Object{
			Created: int64(i),
			Updated: 0,
			Index:   strconv.Itoa(i),
			Path:    key,
			Data:    benchData,
		}
		layer.Set(key, obj)
	}

	i := 0
	for b.Loop() {
		key := "test/" + strconv.Itoa(i%1000)
		layer.Get(key)
		i++
	}
}

func BenchmarkMemoryLayerGetList(b *testing.B) {
	layer := NewMemoryLayer()
	layer.Start(LayerOptions{})
	defer layer.Close()

	// Pre-populate
	for i := range 100 {
		key := "test/" + strconv.Itoa(i)
		obj := &meta.Object{
			Created: int64(i),
			Updated: 0,
			Index:   strconv.Itoa(i),
			Path:    key,
			Data:    benchData,
		}
		layer.Set(key, obj)
	}

	for b.Loop() {
		layer.GetList("test/*")
	}
}

func BenchmarkMemoryStorageSetGetDel(b *testing.B) {
	storage := New(LayeredConfig{
		Memory: NewMemoryLayer(),
	})
	storage.Start(Options{})
	defer storage.Close()

	// Drain the watcher channel to prevent blocking
	watcher := storage.WatchSharded()
	done := make(chan struct{})
	go func() {
		for i := 0; i < watcher.Count(); i++ {
			go func(ch StorageChan) {
				for {
					select {
					case <-ch:
					case <-done:
						return
					}
				}
			}(watcher.Shard(i))
		}
	}()
	defer close(done)

	i := 0
	for b.Loop() {
		key := "test/" + strconv.Itoa(i)
		storage.Set(key, benchData)
		storage.Get(key)
		storage.Del(key)
		i++
	}
}

func BenchmarkLayeredStorageMemoryOnly(b *testing.B) {
	storage := New(LayeredConfig{
		Memory: NewMemoryLayer(),
	})
	storage.Start(Options{})
	defer storage.Close()

	// Drain the watcher channel to prevent blocking
	watcher := storage.WatchSharded()
	done := make(chan struct{})
	go func() {
		for i := 0; i < watcher.Count(); i++ {
			go func(ch StorageChan) {
				for {
					select {
					case <-ch:
					case <-done:
						return
					}
				}
			}(watcher.Shard(i))
		}
	}()
	defer close(done)

	i := 0
	for b.Loop() {
		key := "test/" + strconv.Itoa(i)
		storage.Set(key, benchData)
		storage.Get(key)
		storage.Del(key)
		i++
	}
}
