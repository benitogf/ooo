# client

WebSocket subscription client for OOO servers with automatic reconnection and retry logic.

## Overview

The client package provides type-safe WebSocket subscriptions to OOO server endpoints. It handles:

- Automatic reconnection with exponential backoff
- JSON patch application for efficient updates
- Generic type support for strongly-typed data
- Both single-key and glob (list) subscriptions

## Architecture

```
┌─────────────────┐         ┌─────────────────┐
│   Application   │         │   OOO Server    │
│                 │         │                 │
│  ┌───────────┐  │   WS    │  ┌───────────┐  │
│  │ Subscribe │──┼────────▶│  │  Stream   │  │
│  └───────────┘  │         │  └───────────┘  │
│        │        │         │        │        │
│        ▼        │         │        ▼        │
│  ┌───────────┐  │  Patch  │  ┌───────────┐  │
│  │   Cache   │◀─┼─────────┤  │  Storage  │  │
│  └───────────┘  │         │  └───────────┘  │
│        │        │         └─────────────────┘
│        ▼        │
│  ┌───────────┐  │
│  │ OnMessage │  │
│  └───────────┘  │
└─────────────────┘
```

## Usage

### Single Key Subscription

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

client.Subscribe(client.SubscribeConfig{
    Ctx:    ctx,
    Server: client.Server{Protocol: "ws", Host: "localhost:8080"},
}, "settings", client.SubscribeEvents[Settings]{
    OnMessage: func(m client.Meta[Settings]) {
        fmt.Printf("Settings updated: %+v\n", m.Data)
    },
})
```

### List Subscription (Glob)

```go
client.SubscribeList(client.SubscribeConfig{
    Ctx:    ctx,
    Server: client.Server{Protocol: "ws", Host: "localhost:8080"},
}, "things/*", client.SubscribeListEvents[Thing]{
    OnMessage: func(items []client.Meta[Thing]) {
        fmt.Printf("Things count: %d\n", len(items))
    },
})
```

### With Authentication

```go
header := http.Header{}
header.Set("Authorization", "Bearer "+token)

client.Subscribe(client.SubscribeConfig{
    Ctx:    ctx,
    Server: client.Server{Protocol: "wss", Host: "secure.example.com"},
    Header: header,
}, "protected/data", client.SubscribeEvents[Data]{
    OnMessage: func(m client.Meta[Data]) { /* ... */ },
})
```

## Types

| Type | Description |
|------|-------------|
| `Server` | Protocol and host for WebSocket connection |
| `SubscribeConfig` | Connection settings (context, server, headers, timeouts) |
| `SubscribeEvents[T]` | Callbacks for single-key subscriptions |
| `SubscribeListEvents[T]` | Callbacks for glob subscriptions |
| `Meta[T]` | Wrapper with metadata (created, updated, index) |
| `RetryConfig` | Retry timing configuration |

## Retry Behavior

| Retry Count | Delay |
|-------------|-------|
| 0-29 | 300ms |
| 30-99 | 2s |
| 100+ | 10s |
