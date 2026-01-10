# messages

WebSocket message encoding/decoding and JSON patch application.

## Overview

The messages package handles the wire format for OOO WebSocket communications:

- Message encoding/decoding
- JSON patch application for efficient updates
- Snapshot vs patch differentiation
- Object pooling for reduced allocations

## Architecture

### Snapshot Flow (Initial Connection)

First message on subscription is always a **snapshot** - the complete current state:

```
Server                              Client
   │                                   │
   │  ┌───────────────────────────┐    │
   │  │ Message{                  │    │
   │  │   Data: [full state],     │    │
   │  │   Version: "v1",          │───▶│
   │  │   Snapshot: true          │    │
   │  │ }                         │    │
   │  └───────────────────────────┘    │
   │                                   │
   │                              ┌────▼────┐
   │                              │ Decode  │
   │                              └────┬────┘
   │                                   │
   │                              ┌────▼────┐
   │                              │ Replace │ ◀── Cache = Data
   │                              │  Cache  │
   │                              └─────────┘
```

### Patch Flow (Subsequent Updates)

After the initial snapshot, updates are sent as **JSON patches** (RFC 6902):

```
Server                              Client
   │                                   │
   │  ┌───────────────────────────┐    │
   │  │ Message{                  │    │
   │  │   Data: [patch ops],      │    │
   │  │   Version: "v2",          │───▶│
   │  │   Snapshot: false         │    │
   │  │ }                         │    │
   │  └───────────────────────────┘    │
   │                                   │
   │                              ┌────▼────┐
   │                              │ Decode  │
   │                              └────┬────┘
   │                                   │
   │      ┌───────────┐           ┌────▼────┐
   │      │   Cache   │──────────▶│  Apply  │
   │      │ (current) │           │  Patch  │
   │      └───────────┘           └────┬────┘
   │                                   │
   │                              ┌────▼────┐
   │                              │ Updated │
   │                              │  Cache  │
   │                              └─────────┘
```

### Patch Operations Example

```json
// Snapshot (initial)
{"data": {"name": "alice", "score": 100}, "version": "1", "snapshot": true}

// Patch (update score)
{"data": [{"op": "replace", "path": "/score", "value": 150}], "version": "2", "snapshot": false}

// Patch (add field)
{"data": [{"op": "add", "path": "/level", "value": 5}], "version": "3", "snapshot": false}
```

## Message Format

```json
{
  "data": [...],
  "version": "1234567890",
  "snapshot": true
}
```

- **snapshot=true**: `data` is the complete state
- **snapshot=false**: `data` is a JSON patch to apply

## Usage

### Decode Message

```go
msg, err := messages.DecodeBuffer(wsData)
if err != nil {
    return err
}
fmt.Printf("Version: %s, Snapshot: %v\n", msg.Version, msg.Snapshot)
```

### Apply Patch (Single Object)

```go
newCache, changed, err := messages.Patch(wsData, currentCache)
if changed {
    // Cache was updated
}
```

### Apply Patch (List)

```go
newCache, changed, err := messages.PatchList(wsData, currentCache)
```

### With Pooled Messages (High Performance)

```go
msg, err := messages.DecodeBufferPooled(wsData)
if err != nil {
    return err
}
defer messages.ReleaseMessage(msg) // Return to pool

// Use msg...
```

## Types

| Type | Description |
|------|-------------|
| `Message` | WebSocket message with data, version, snapshot flag |

## Functions

| Function | Description |
|----------|-------------|
| `DecodeBuffer(data)` | Decode message from bytes |
| `DecodeBufferPooled(data)` | Decode with pooling (call ReleaseMessage after) |
| `DecodeReader(r)` | Decode from io.Reader |
| `Patch(msg, cache)` | Apply message to single-object cache |
| `PatchList(msg, cache)` | Apply message to list cache |
| `ReleaseMessage(m)` | Return pooled message |
