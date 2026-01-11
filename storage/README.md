# storage

Pluggable storage backend with layered architecture.

## Overview

The storage package provides a flexible, layered storage system for OOO servers:

- **Layered Architecture**: Memory + optional persistent layer
- **Event Broadcasting**: Notify subscribers of changes
- **Sharded Channels**: Per-key ordering for concurrent operations
- **Locking**: Path-level locks for atomic operations

## Architecture

```
┌─────────────────────────────────────────────┐
│                  Layered                    │
│  ┌─────────────────────────────────────┐   │
│  │          Memory Layer               │   │ ◀── Fast reads
│  │  (map[string]meta.Object + RWMutex) │   │
│  └─────────────────────────────────────┘   │
│                    │                        │
│                    │ write-through          │
│                    ▼                        │
│  ┌─────────────────────────────────────┐   │
│  │       Persistent Layer (optional)   │   │ ◀── Durable writes
│  │  (ko, badger, sqlite, etc.)         │   │
│  └─────────────────────────────────────┘   │
└─────────────────────────────────────────────┘
                    │
                    │ Events
                    ▼
            ┌───────────────┐
            │ ShardedChan   │ ◀── Per-key ordering
            └───────────────┘
```

## Usage

### Memory-Only Storage

```go
storage := storage.New(storage.LayeredConfig{
    Memory: storage.NewMemoryLayer(),
})
```

### With Persistent Layer

```go
storage := storage.New(storage.LayeredConfig{
    Memory:     storage.NewMemoryLayer(),
    Persistent: myPersistentAdapter,
})
```

### Basic Operations

```go
// Set
index, err := store.Set("users/123", jsonData)

// Get
obj, err := store.Get("users/123")

// Get list (glob)
objs, err := store.GetN("users/*", 0, 100)

// Delete
err := store.Del("users/123")
```

### With Options

```go
storage := storage.New(storage.LayeredConfig{
    Memory: storage.NewMemoryLayer(),
}, storage.Options{
    AfterWrite: func(key string) {
        log.Printf("Written: %s", key)
    },
    BeforeRead: func(key string) {
        log.Printf("Reading: %s", key)
    },
})
```

### Watching Changes

```go
store.Watch(func(event storage.Event) {
    switch event.Operation {
    case "set":
        fmt.Printf("Set: %s\n", event.Key)
    case "del":
        fmt.Printf("Deleted: %s\n", event.Key)
    }
})
```

### Locking

```go
err := store.Lock("users/123")
defer store.Unlock("users/123")

// Atomic operations on users/123
```

## Types

| Type | Description |
|------|-------------|
| `Layered` | Main storage implementation |
| `LayeredConfig` | Configuration with memory + persistent layers |
| `Options` | Callbacks and settings |
| `Event` | Storage change event |
| `ShardedChan` | Per-key ordered event channels |
| `MemoryLayer` | In-memory storage layer |
| `Adapter` | Interface for persistent backends |

## Event Structure

```go
type Event struct {
    Key       string       // The affected key
    Operation string       // "set" or "del"
    Object    *meta.Object // The object (nil for delete)
}
```

## Errors

| Error | Description |
|-------|-------------|
| `ErrNotFound` | Key does not exist |
| `ErrInvalidPath` | Invalid key format |
| `ErrGlobNotAllowed` | Glob used where not allowed |
| `ErrGlobRequired` | Glob required but not provided |
| `ErrNotActive` | Storage not started |
