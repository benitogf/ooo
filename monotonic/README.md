# monotonic

NTP-resilient monotonic clock for unique timestamp generation.

## Overview

The monotonic package provides a clock that generates strictly increasing timestamps even when the system clock is adjusted (NTP/PTP corrections). This is critical for:

- Generating unique, ordered indices
- Ensuring causality in distributed systems
- Avoiding timestamp collisions during clock adjustments

## Architecture

```
                    System Clock
                         │
                         │ (startup only)
                         ▼
┌─────────────────────────────────────────┐
│            Monotonic Clock              │
│                                         │
│  startWall ◀── Wall time at startup     │
│  startMono ◀── Monotonic ref at startup │
│  offset    ◀── Drift correction         │
│  lastTime  ◀── Ensures monotonicity     │
└─────────────────────────────────────────┘
         │
         │ Now() = startWall + elapsed + offset
         │         (always increasing)
         ▼
    ┌─────────┐
    │ 1234567 │ ◀── Unique nanosecond timestamp
    └─────────┘
```

## How It Works

1. **Startup**: Captures wall clock time and Go's monotonic reference
2. **Now()**: Calculates `startWall + time.Since(startMono) + offset`
3. **Monotonicity**: If calculated time ≤ last time, returns last + 1
4. **Drift Correction**: Background goroutine slowly corrects offset toward real time

## Usage

### Global Clock (Recommended)

```go
// Initialize once at startup
monotonic.Init()

// Get current timestamp
ts := monotonic.Now()
```

### Instance Clock

```go
clock := monotonic.New()
defer clock.Stop()

ts := clock.Now()
```

### In OOO

The monotonic clock is used internally by:
- `key.NewIndex()` for generating storage indices
- `stream.Clock` for WebSocket version tracking

## Benefits

| Scenario | Standard Clock | Monotonic Clock |
|----------|---------------|-----------------|
| NTP jump backward | Duplicate timestamps | Unique timestamps |
| NTP jump forward | Gap in sequence | Continuous sequence |
| High concurrency | Potential collisions | Guaranteed unique |

## Configuration

| Constant | Value | Description |
|----------|-------|-------------|
| `CorrectionRate` | 1ms/s | Drift correction speed |
| `CorrectionInterval` | 1s | How often correction runs |

## Functions

| Function | Description |
|----------|-------------|
| `Init()` | Initialize global clock (safe to call multiple times) |
| `Now()` | Get timestamp from global clock |
| `New()` | Create new clock instance |
| `(c *Clock) Now()` | Get timestamp from instance |
| `(c *Clock) Stop()` | Stop correction goroutine |
