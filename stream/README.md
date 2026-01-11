# stream

WebSocket connection pooling and broadcast management.

## Overview

The stream package manages WebSocket subscriptions on the server side:

- **Connection Pooling**: Groups connections by subscription key
- **Efficient Broadcasting**: Parallel broadcast for many connections
- **JSON Patch Generation**: Sends minimal diffs instead of full data
- **Trie-based Routing**: O(k) path matching for glob patterns

## Architecture

```
                    Storage Event
                         │
                         ▼
┌─────────────────────────────────────────────┐
│                   Stream                    │
│  ┌─────────────────────────────────────┐   │
│  │              poolIndex              │   │
│  │          (Trie for routing)         │   │
│  └─────────────────────────────────────┘   │
│         │                    │              │
│         ▼                    ▼              │
│  ┌─────────────┐      ┌─────────────┐      │
│  │ Pool[key1]  │      │ Pool[key2]  │      │
│  │ ┌─────────┐ │      │ ┌─────────┐ │      │
│  │ │  Cache  │ │      │ │  Cache  │ │      │
│  │ └─────────┘ │      │ └─────────┘ │      │
│  │ [ws1,ws2]   │      │ [ws3,ws4]   │      │
│  └─────────────┘      └─────────────┘      │
└─────────────────────────────────────────────┘
         │                     │
         ▼                     ▼
    ┌─────────┐           ┌─────────┐
    │  Patch  │           │  Patch  │
    │  or     │           │  or     │
    │Snapshot │           │Snapshot │
    └────┬────┘           └────┬────┘
         │                     │
         ▼                     ▼
      Clients               Clients
```

## Key Features

### Patch vs Snapshot

- **First message**: Full snapshot of current data
- **Subsequent**: JSON patch (RFC 6902) with only changes
- **Configurable**: `NoPatch` for always snapshot, `ForcePatch` for always patch

### Parallel Broadcasting

When connection count exceeds `ParallelThreshold` (default: 6), broadcasts run in parallel goroutines for better performance.

### Trie-based Routing

The `poolIndex` trie enables efficient O(k) matching of storage events to subscription pools, even with glob patterns.

## Usage

### Server Integration

The stream is automatically managed by `ooo.Server`. Direct usage is rare:

```go
// Subscribe callback
server.Stream.OnSubscribe = func(key string) error {
    if !isAllowed(key) {
        return errors.New("subscription denied")
    }
    return nil
}

// Unsubscribe callback
server.Stream.OnUnsubscribe = func(key string) {
    log.Printf("Client left: %s", key)
}
```

### Configuration

```go
server.Stream.WriteTimeout = 30 * time.Second
server.Stream.ParallelThreshold = 10
server.Stream.NoPatch = true  // Always send snapshots
```

### Manual Broadcasting

```go
server.Stream.Broadcast(stream.BroadcastOpt{
    Key:       "things/123",
    Operation: "set",
    Object:    &obj,
})
```

## Types

| Type | Description |
|------|-------------|
| `Stream` | Main manager for all subscription pools |
| `Pool` | Connections subscribed to a specific key |
| `Conn` | Thread-safe WebSocket connection wrapper |
| `Cache` | Cached data and version for a pool |
| `BroadcastOpt` | Options for broadcast operations |
| `Subscribe` | Callback type for subscription events |
| `Unsubscribe` | Callback type for unsubscription events |

## Defaults

| Setting | Value |
|---------|-------|
| `DefaultWriteTimeout` | 15s |
| `DefaultParallelThreshold` | 6 connections |
