# OOO Samples

This directory contains example code demonstrating various features of the ooo library.

## Core Samples

| Sample | Description |
|--------|-------------|
| [basic_server](./basic_server) | Minimal server setup |
| [static_routes_filters_audit](./static_routes_filters_audit) | Static mode, filters, and API key authentication |
| [storage_api](./storage_api) | Direct storage read/write operations |
| [io_operations](./io_operations) | Typed I/O helpers (Get, Set, Push, GetList) |
| [websocket_subscribe_list](./websocket_subscribe_list) | Real-time subscription to list paths |
| [websocket_subscribe_single](./websocket_subscribe_single) | Real-time subscription to single items |
| [remote_io_operations](./remote_io_operations) | Remote HTTP operations with retry support |
| [custom_endpoints](./custom_endpoints) | Custom HTTP endpoints with typed schemas for UI |

## Advanced Features

| Sample | Description | Requires |
|--------|-------------|----------|
| [persistent_storage_ko](./persistent_storage_ko) | Persistent storage with LevelDB | `go get github.com/benitogf/ko` |
| [jwt_auth](./jwt_auth) | JWT authentication | `go get github.com/benitogf/auth github.com/benitogf/ko` |
| [limit_filter](./limit_filter) | Capped collections with auto-cleanup | (none) |
| [limit_filter_with_validation](./limit_filter_with_validation) | LimitFilter with strict schema validation | (none) |

## Ecosystem Integration Samples

These samples demonstrate integration with other packages in the ecosystem.

| Sample | Description | Requires |
|--------|-------------|----------|
| [custom_endpoints_nopog](./custom_endpoints_nopog) | Custom endpoints with PostgreSQL storage | `go get github.com/benitogf/nopog` |
| [pivot_synchronization](./pivot_synchronization) | Multi-instance synchronization | `go get github.com/benitogf/pivot` |
| [proxy](./proxy) | Proxy routes to remote ooo servers | (none) |

## Running Samples

Each sample is a standalone Go program. To run a sample:

```bash
cd samples/basic_server
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
