# OOO Samples

This directory contains example code demonstrating various features of the ooo library.

## Core Samples

| Sample | Description |
|--------|-------------|
| [01_basic_server](./01_basic_server) | Minimal server setup |
| [02_static_routes_filters_audit](./02_static_routes_filters_audit) | Static mode, filters, and API key authentication |
| [03_storage_api](./03_storage_api) | Direct storage read/write operations |
| [04_io_operations](./04_io_operations) | Typed I/O helpers (Get, Set, Push, GetList) |
| [05_websocket_subscribe_list](./05_websocket_subscribe_list) | Real-time subscription to list paths |
| [06_websocket_subscribe_single](./06_websocket_subscribe_single) | Real-time subscription to single items |
| [07_websocket_subscribe_multiple](./07_websocket_subscribe_multiple) | Subscribe to multiple paths with different types |
| [08_remote_io_operations](./08_remote_io_operations) | Remote HTTP operations with retry support |
| [09_custom_endpoints](./09_custom_endpoints) | Custom HTTP endpoints with typed schemas for UI |

## Advanced Features

| Sample | Description | Requires |
|--------|-------------|----------|
| [10_persistent_storage_ko](./10_persistent_storage_ko) | Persistent storage with LevelDB | `go get github.com/benitogf/ko` |
| [11_jwt_auth](./11_jwt_auth) | JWT authentication | `go get github.com/benitogf/auth github.com/benitogf/ko` |
| [12_limit_filter](./12_limit_filter) | Capped collections with auto-cleanup | (none) |
| [13_limit_filter_with_validation](./13_limit_filter_with_validation) | LimitFilter with strict schema validation | (none) |

## Ecosystem Integration Samples

These samples demonstrate integration with other packages in the ecosystem.

| Sample | Description | Requires |
|--------|-------------|----------|
| [14_custom_endpoints_nopog](./14_custom_endpoints_nopog) | Custom endpoints with PostgreSQL storage | `go get github.com/benitogf/nopog` |
| [15_pivot_synchronization](./15_pivot_synchronization) | Multi-instance synchronization | `go get github.com/benitogf/pivot` |
| [16_proxy](./16_proxy) | Proxy routes to remote ooo servers | (none) |

## Running Samples

Each sample is a standalone Go program. To run a sample:

```bash
cd samples/01_basic_server
go run main.go
```

For ecosystem samples, install dependencies first:

```bash
# For persistent storage
go get github.com/benitogf/ko

# For JWT auth
go get github.com/benitogf/auth github.com/benitogf/ko
```

## Related Projects

- [ooo](https://github.com/benitogf/ooo) - Core server (in-memory state with WebSocket/REST)
- [ko](https://github.com/benitogf/ko) - Persistent storage adapter (LevelDB)
- [ooo-client](https://github.com/benitogf/ooo-client) - JavaScript client
- [auth](https://github.com/benitogf/auth) - JWT authentication middleware
- [mono](https://github.com/benitogf/mono) - Full-stack boilerplate (Go + React)
- [nopog](https://github.com/benitogf/nopog) - PostgreSQL adapter for large-scale storage
- [pivot](https://github.com/benitogf/pivot) - Multi-instance synchronization
