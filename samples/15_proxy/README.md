# 15 - Proxy Routes

Demonstrates using the proxy package to expose routes from remote OOO servers with path remapping.

## Overview

This sample shows how to:
- Create a proxy server that forwards requests to a remote server
- **Remap paths** - expose different paths on proxy than on source (e.g., `settings/{deviceID}` → `settings`)
- Use `proxy.Route` for single-key paths
- Use `proxy.RouteList` for glob paths
- Share WebSocket subscriptions among multiple local clients

## Files

- `remote_server.go` - The source server with actual data
- `proxy_server.go` - The proxy that forwards requests with path remapping

## Running

1. Start the remote server:
```bash
go run remote_server.go
```

2. In another terminal, start the proxy server:
```bash
go run proxy_server.go
```

3. Access data through the proxy (note the path remapping):
```bash
# Get settings for device "abc123" via proxy
# Proxy path: /settings/abc123 → Remote path: /settings
curl http://localhost:8801/settings/abc123

# Get items for device "abc123" via proxy  
# Proxy path: /items/abc123/* → Remote path: /items/*
curl http://localhost:8801/items/abc123/*

# Create item via proxy (forwarded to remote)
curl -X POST http://localhost:8801/items/abc123/* -d '{"name":"test"}'
```

## Architecture

```
┌─────────────┐      HTTP/WS       ┌─────────────┐      HTTP/WS       ┌─────────────┐
│   Client    │ ─────────────────▶ │   Proxy     │ ─────────────────▶ │   Remote    │
│             │ ◀───────────────── │   :8801     │ ◀───────────────── │   :8800     │
└─────────────┘                    └─────────────┘                    └─────────────┘

Path Remapping:
  Proxy Path              →    Remote Path
  ─────────────────────────────────────────
  /settings/{deviceID}    →    /settings
  /items/{deviceID}/*     →    /items/*
```

### WebSocket Subscription Fan-out

```
                                   ┌─────────────┐
                              ┌───▶│  Client A   │  subscribes to /settings/dev1
                              │    └─────────────┘
┌─────────────┐           ┌───┴───┐               
│   Remote    │──────────▶│ Proxy │  single upstream subscription to /settings
│   :8800     │           └───┬───┘               
└─────────────┘               │    ┌─────────────┐
                              └───▶│  Client B   │  subscribes to /settings/dev2
                                   └─────────────┘
```
