# filters

Data transformation and validation filters for OOO server operations.

## Overview

The filters package provides hooks into the OOO server's data pipeline, allowing:

- Write validation and transformation
- Read filtering for single objects and lists
- Delete blocking/validation
- Post-write notifications

## Architecture

```
          Request Flow
              │
              ▼
┌─────────────────────────┐
│     Write Filters       │ ◀── Validate/Transform incoming data
│  func(key, data) data   │
└───────────┬─────────────┘
            │
            ▼
┌─────────────────────────┐
│       Storage           │
└───────────┬─────────────┘
            │
            ▼
┌─────────────────────────┐
│     Read Filters        │ ◀── Transform outgoing data
│  ObjectFilter/ListFilter│
└───────────┬─────────────┘
            │
            ▼
         Response
```

## Usage

### Write Filter (Validation)

```go
server.Filters.AddWrite("users/*", func(key string, data json.RawMessage) (json.RawMessage, error) {
    var user User
    if err := json.Unmarshal(data, &user); err != nil {
        return nil, err
    }
    if user.Email == "" {
        return nil, errors.New("email required")
    }
    return data, nil
})
```

### Write Filter (Transformation)

```go
server.Filters.AddWrite("posts/*", func(key string, data json.RawMessage) (json.RawMessage, error) {
    var post Post
    json.Unmarshal(data, &post)
    post.Slug = slugify(post.Title)
    return json.Marshal(post)
})
```

### Read Filter (Single Object)

```go
server.Filters.AddReadObject("secrets/*", func(key string, obj meta.Object) (meta.Object, error) {
    var secret Secret
    json.Unmarshal(obj.Data, &secret)
    secret.Value = "***" // redact
    obj.Data, _ = json.Marshal(secret)
    return obj, nil
})
```

### Read Filter (List)

```go
server.Filters.AddReadList("items/*", func(key string, objs []meta.Object) ([]meta.Object, error) {
    // Filter or sort the list
    return objs[:min(10, len(objs))], nil // limit to 10
})
```

### Delete Blocker

```go
server.Filters.AddDelete("protected/*", func(key string) error {
    return errors.New("deletion not allowed")
})
```

### After Write Notification

```go
server.Filters.AddAfterWrite("events/*", func(key string) {
    log.Printf("Event written: %s", key)
})
```

### LimitFilter

LimitFilter maintains count and/or time constraints on glob paths.

```go
lf, err := filters.NewLimitFilter("logs/*", filters.LimitFilterConfig{
    Limit:  100,                    // max entries (count constraint)
    MaxAge: 24 * time.Hour,         // max age (time constraint)
    Order:  filters.OrderDesc,      // sort order (default: desc)
    Cleanup: filters.CleanupConfig{ // optional periodic background cleanup
        Enabled:  true,
        Interval: 10 * time.Minute, // default: 10min, minimum: 1min
    },
}, db)
```

Supports dynamic constraints via functions:

```go
lf, err := filters.NewLimitFilter("logs/*", filters.LimitFilterConfig{
    LimitFunc:  func() int { return currentLimit },
    MaxAgeFunc: func() time.Duration { return currentRetention },
}, db)
```

## Types

| Type | Signature | Purpose |
|------|-----------|---------|
| `Apply` | `func(key string, data json.RawMessage) (json.RawMessage, error)` | Write filter |
| `ApplyObject` | `func(key string, obj meta.Object) (meta.Object, error)` | Single read filter |
| `ApplyList` | `func(key string, objs []meta.Object) ([]meta.Object, error)` | List read filter |
| `Block` | `func(key string) error` | Delete blocker |
| `Notify` | `func(key string)` | Post-write callback |
| `LimitFunc` | `func() int` | Dynamic count limit |
| `MaxAgeFunc` | `func() time.Duration` | Dynamic time limit |
