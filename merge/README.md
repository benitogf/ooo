# merge

JSON deep merge utilities for combining objects.

## Overview

The merge package provides RFC 7396-style JSON merge patch functionality with tracking of replacements. It's used internally for applying partial updates to cached data.

## Architecture

```
┌──────────────┐     ┌──────────────┐
│  Base JSON   │     │  Patch JSON  │
│              │     │              │
│ {"a":1,"b":2}│     │ {"b":3,"c":4}│
└──────┬───────┘     └──────┬───────┘
       │                    │
       └────────┬───────────┘
                │
                ▼
        ┌───────────────┐
        │    Merge()    │
        └───────┬───────┘
                │
                ▼
        ┌───────────────┐
        │ {"a":1,"b":3, │
        │  "c":4}       │
        └───────────────┘
```

## Usage

### Basic Merge

```go
base := []byte(`{"name":"old","count":1}`)
patch := []byte(`{"name":"new","active":true}`)

result, info, err := merge.Merge(base, patch)
// result: {"name":"new","count":1,"active":true}
```

### Tracking Replacements

```go
result, info, err := merge.Merge(base, patch)
for path, value := range info.Replaced {
    fmt.Printf("Replaced %s with %v\n", path, value)
}
// Output: Replaced name with "new"
```

### Nested Objects

```go
base := []byte(`{"user":{"name":"alice","age":30}}`)
patch := []byte(`{"user":{"age":31}}`)

result, _, _ := merge.Merge(base, patch)
// result: {"user":{"name":"alice","age":31}}
```

### Arrays (Replace, Not Merge)

```go
base := []byte(`{"tags":["a","b"]}`)
patch := []byte(`{"tags":["c"]}`)

result, _, _ := merge.Merge(base, patch)
// result: {"tags":["c"]} - arrays are replaced entirely
```

## Types

| Type | Description |
|------|-------------|
| `Info` | Merge result with errors and replacements |
| `Info.Errors` | Non-critical merge errors |
| `Info.Replaced` | Map of paths to replaced values |

## Errors

| Error | Description |
|-------|-------------|
| `ErrDataJSON` | Invalid base JSON |
| `ErrPatchJSON` | Invalid patch JSON |
| `ErrMergedJSON` | Error writing result |
| `ErrPatchObject` | Patch must be object |
