# io

HTTP client operations for remote OOO server communication.

## Overview

The io package provides HTTP-based CRUD operations for interacting with remote OOO servers. Features include:

- Retry logic with exponential backoff
- Context support for cancellation
- Response size limiting
- Type-safe generic operations

## Architecture

```
┌─────────────────┐         ┌─────────────────┐
│   Local App     │         │  Remote OOO     │
│                 │  HTTP   │                 │
│  RemoteGet ─────┼────────▶│  GET /key       │
│  RemoteSet ─────┼────────▶│  POST /key      │
│  RemotePush ────┼────────▶│  POST /glob/*   │
│  RemoteDelete ──┼────────▶│  DELETE /key    │
│                 │         │                 │
└─────────────────┘         └─────────────────┘
        │
        ▼
   ┌─────────┐
   │  Retry  │ ◀── Exponential backoff on 5xx errors
   └─────────┘
```

## Usage

### Configuration

```go
cfg := io.RemoteConfig{
    Client: &http.Client{Timeout: 10 * time.Second},
    Host:   "localhost:8080",
    SSL:    false,
    Header: http.Header{"Authorization": []string{"Bearer token"}},
    Retry: io.RetryConfig{
        MaxRetries: 3,
        RetryDelay: 100 * time.Millisecond,
    },
}
```

### Get Single Item

```go
var settings Settings
obj, err := io.RemoteGet[Settings](cfg, "settings")
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Settings: %+v\n", obj.Data)
```

### Get List

```go
items, err := io.RemoteGetList[Thing](cfg, "things/*")
if err != nil {
    log.Fatal(err)
}
for _, item := range items {
    fmt.Printf("Thing: %+v\n", item.Data)
}
```

### Set Item

```go
settings := Settings{Theme: "dark", Language: "en"}
err := io.RemoteSet(cfg, "settings", settings)
```

### Push to List

```go
thing := Thing{Name: "New Item"}
index, err := io.RemotePush(cfg, "things/*", thing)
fmt.Printf("Created with index: %s\n", index)
```

### Delete Item

```go
err := io.RemoteDelete(cfg, "things/abc123")
```

### With Context

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

err := io.RemoteSetWithContext(ctx, cfg, "settings", settings)
```

## Types

| Type | Description |
|------|-------------|
| `RemoteConfig` | HTTP client configuration |
| `RetryConfig` | Retry timing settings |
| `IndexResponse` | Response from POST operations |

## Defaults

| Setting | Value |
|---------|-------|
| `DefaultMaxRetries` | 3 |
| `DefaultRetryDelay` | 100ms |
| `DefaultMaxResponseSize` | 10MB |
