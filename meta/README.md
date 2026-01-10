# meta

Core data structures for OOO storage objects.

## Overview

The meta package defines the fundamental data structure used throughout OOO for storing and transmitting data. Every piece of data in OOO is wrapped in a `meta.Object` which includes:

- Creation timestamp
- Last update timestamp
- Unique index
- The actual data payload

## Architecture

```
┌─────────────────────────────────┐
│          meta.Object            │
├─────────────────────────────────┤
│  Created: int64  (nanoseconds)  │
│  Updated: int64  (nanoseconds)  │
│  Index:   string (unique ID)    │
│  Path:    string (storage key)  │
│  Data:    json.RawMessage       │
└─────────────────────────────────┘
            │
            │ Used by
            ▼
┌───────────┬───────────┬─────────┐
│  Storage  │  Stream   │  Client │
└───────────┴───────────┴─────────┘
```

## Usage

### Create Object

```go
obj := meta.Object{
    Created: time.Now().UnixNano(),
    Updated: time.Now().UnixNano(),
    Index:   "abc123",
    Data:    json.RawMessage(`{"name":"test"}`),
}
```

### Encode Object

```go
data, err := meta.Encode(obj)
```

### Decode Object

```go
var obj meta.Object
err := meta.Decode(data, &obj)
```

### Sorting

```go
objects := []meta.Object{...}

// Sort newest first
sort.Slice(objects, meta.SortDesc(objects))

// Sort oldest first
sort.Slice(objects, meta.SortAsc(objects))
```

### Object Pooling (High Performance)

```go
obj := meta.GetObject()
defer meta.PutObject(obj)

// Use obj...
```

## Types

| Type | Description |
|------|-------------|
| `Object` | Core data wrapper with metadata |
| `EmptyObject` | Pre-encoded empty object bytes |

## Object Fields

| Field | Type | Description |
|-------|------|-------------|
| `Created` | `int64` | Creation timestamp (nanoseconds) |
| `Updated` | `int64` | Last update timestamp (nanoseconds) |
| `Index` | `string` | Unique identifier |
| `Path` | `string` | Storage key (optional, omitted in JSON if empty) |
| `Data` | `json.RawMessage` | Actual payload |

## Functions

| Function | Description |
|----------|-------------|
| `Encode(v)` | Marshal to JSON |
| `EncodeToBuffer(v)` | Marshal using pooled buffer |
| `Decode(data, v)` | Unmarshal from JSON |
| `SortDesc(objs)` | Sort function for newest-first |
| `SortAsc(objs)` | Sort function for oldest-first |
| `GetObject()` | Get object from pool |
| `PutObject(obj)` | Return object to pool |
