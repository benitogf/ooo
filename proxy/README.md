# OOO Proxy Package

A transparent proxy package that allows an OOO server to expose routes from other OOO servers.

## Overview

The proxy package enables one server to act as a gateway to other servers. When a client connects to a proxy route, the proxy:

1. Resolves the target server address from the request path
2. Establishes a subscription to the remote server
3. Caches remote data in memory
4. Fans out updates to all local subscribers
5. Forwards CRUD operations to the remote server

## Architecture

```
Local Subscribers          Proxy State              Remote Server
     ┌─────┐               ┌──────────┐              ┌─────────┐
     │ WS1 │◄──────┐       │  cache   │◄─────────────│ settings│
     └─────┘       │       │ (JSON)   │   Subscribe  └─────────┘
     ┌─────┐       ├───────┤          │
     │ WS2 │◄──────┤       │ localSubs│
     └─────┘       │       │ [WS1,WS2]│
     ┌─────┐       │       └──────────┘
     │ WS3 │◄──────┘
     └─────┘
       Fan-out to all local subscribers
```

### Key Behaviors

- **Connection Sharing**: Multiple local subscribers to the same remote path share one remote subscription
- **Cache**: Remote data is cached in memory for immediate delivery to new subscribers
- **Transparent**: CRUD operations are forwarded to remote; updates cascade back via subscription
- **No Fallback**: If remote is unreachable, local connections are closed (transparent proxy)

## API

### Types

```go
// Resolver maps local path to remote server address and path
// Example: "settings/device123" -> ("192.168.1.100:8080", "settings")
type Resolver func(localPath string) (address, remotePath string, err error)

// Config for a proxy route
type Config struct {
    Resolve   Resolver
    Subscribe *client.SubscribeConfig // optional, for headers/timeouts
}
```

### Functions

```go
// Route proxies a single-key route pattern (e.g., "settings/*")
func Route(server *ooo.Server, localPath string, cfg Config) error

// RouteList proxies a glob route pattern (e.g., "devices/*/things/*")
func RouteList(server *ooo.Server, localPath string, cfg Config) error
```

## Usage

### Basic Example

```go
import (
    "github.com/benitogf/ooo"
    "github.com/benitogf/ooo/proxy"
)

server := &ooo.Server{}
// ... server setup ...

proxy.Route(server, "settings/*", proxy.Config{
    Resolve: func(localPath string) (string, string, error) {
        // localPath = "settings/device123"
        parts := strings.Split(localPath, "/")
        if len(parts) < 2 {
            return "", "", errors.New("device ID required")
        }
        deviceID := parts[1]
        addr := devices.GetAddress(deviceID) // your lookup logic
        return addr, "settings", nil
    },
})
```

### With Authentication

```go
proxy.Route(server, "settings/*", proxy.Config{
    Resolve: resolveDevice,
    Subscribe: &client.SubscribeConfig{
        Header: http.Header{"Authorization": []string{"Bearer " + token}},
    },
})
```

### Proxying a List Route

```go
proxy.RouteList(server, "devices/*/things/*", proxy.Config{
    Resolve: func(localPath string) (string, string, error) {
        // localPath = "devices/dev1/things/abc" or "devices/dev1/things"
        parts := strings.Split(localPath, "/")
        deviceID := parts[1]
        // Map to remote "things/*" or "things/abc"
        remotePath := strings.Join(parts[2:], "/")
        return devices.GetAddress(deviceID), remotePath, nil
    },
})
```

## Behavior Details

### Subscription Lifecycle

1. First local subscriber connects → starts remote subscription
2. Remote data received → cached and sent to subscriber
3. Additional subscribers → receive cached data immediately, then updates
4. Remote updates → fan-out to all local subscribers
5. Last subscriber disconnects → remote subscription closed, cache cleared

### CRUD Operations

- **GET**: Forwarded to remote, response returned to client
- **POST**: Forwarded to remote, triggers remote event, cascades to all subscribers
- **DELETE**: Forwarded to remote, triggers remote event, cascades to all subscribers

### Error Handling

- Remote unreachable: Local connections closed
- Remote subscription fails: Local connections closed
- No retry/fallback: This is a transparent proxy
