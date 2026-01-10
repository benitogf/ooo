# key

Path and key utilities for OOO routing and storage.

## Overview

The key package provides utilities for working with OOO's hierarchical key paths:

- Key validation
- Glob pattern matching
- Path manipulation
- Index generation

## Key Format

Valid keys follow these rules:
- Characters: `a-z`, `A-Z`, `0-9`, `*`, `/`
- Cannot start or end with `/`
- Cannot contain `//` or `**`
- Glob `*` represents a single path segment

### Examples

| Key | Valid | Description |
|-----|-------|-------------|
| `settings` | ✓ | Single key |
| `users/123` | ✓ | Nested key |
| `things/*` | ✓ | Glob pattern |
| `a/*/b/*` | ✓ | Multi-level glob |
| `/settings` | ✗ | Starts with `/` |
| `users//123` | ✗ | Contains `//` |

## Usage

### Validation

```go
if key.IsValid("users/123") {
    // key is valid
}
```

### Glob Detection

```go
key.IsGlob("things/*")  // true - ends with /*
key.HasGlob("a/*/b")    // true - contains * anywhere
```

### Pattern Matching

```go
key.Match("things/*", "things/abc")     // true
key.Match("things/*", "things/a/b")     // false (too deep)
key.Match("a/*/b/*", "a/x/b/y")         // true
```

### Path Manipulation

```go
key.LastIndex("users/123/profile")  // "profile"
key.Build("users", "123")           // "users/123"
key.Parent("users/123/profile")     // "users/123"
```

### Index Generation

```go
index := key.NewIndex()  // monotonic nanosecond timestamp
```

## Functions

| Function | Description |
|----------|-------------|
| `IsValid(key)` | Check if key matches valid pattern |
| `IsGlob(path)` | Check if path ends with `/*` |
| `HasGlob(path)` | Check if path contains `*` |
| `Match(pattern, key)` | Check if key matches glob pattern |
| `LastIndex(path)` | Get last segment of path |
| `Build(parts...)` | Join parts with `/` |
| `Parent(path)` | Get parent path |
| `NewIndex()` | Generate monotonic index |
