package monotonic

import (
	"sync"
	"sync/atomic"
	"time"
)

const (
	// CorrectionRate is how many nanoseconds per second we drift back toward real time
	// 1ms per second = 1,000,000 ns/s, meaning ~3.6 seconds correction per hour
	CorrectionRate int64 = 1_000_000

	// CorrectionInterval is how often we check and apply corrections
	CorrectionInterval = time.Second
)

// Clock provides monotonically increasing timestamps that are resilient to
// system clock adjustments (NTP/PTP). It uses the system clock once at startup
// and then relies on Go's monotonic clock for elapsed time calculations.
type Clock struct {
	startWall int64     // Wall clock at startup (UnixNano)
	startMono time.Time // time.Time at startup (has monotonic component)
	offset    atomic.Int64
	lastTime  atomic.Int64
	stopCh    chan struct{}
}

// globalClock is the package-level singleton
var globalClock *Clock

// initOnce ensures Init() only creates the clock once
var initOnce sync.Once

// Init initializes the global monotonic clock instance.
// Safe to call multiple times; only the first call has effect.
func Init() {
	initOnce.Do(func() {
		globalClock = New()
	})
}

// New creates and starts a new monotonic clock
func New() *Clock {
	now := time.Now()
	c := &Clock{
		startWall: now.UTC().UnixNano(),
		startMono: now,
		stopCh:    make(chan struct{}),
	}
	go c.correctionLoop()
	return c
}

// Now returns a monotonically increasing timestamp in nanoseconds.
// This is the high-performance path - uses atomic operations only.
func (c *Clock) Now() int64 {
	// Elapsed time since startup using monotonic clock (no syscall for wall time)
	elapsed := time.Since(c.startMono).Nanoseconds()

	// Synthetic time = startup wall time + monotonic elapsed + correction offset
	syntheticTime := c.startWall + elapsed + c.offset.Load()

	// Ensure monotonicity using atomic CAS loop
	for {
		last := c.lastTime.Load()
		if syntheticTime <= last {
			// We need to increment past last
			newTime := last + 1
			if c.lastTime.CompareAndSwap(last, newTime) {
				return newTime
			}
			// CAS failed, another goroutine updated - retry
			continue
		}
		// syntheticTime > last, try to update
		if c.lastTime.CompareAndSwap(last, syntheticTime) {
			return syntheticTime
		}
		// CAS failed, retry with fresh values
	}
}

// correctionLoop gradually adjusts the offset to sync back to real time
func (c *Clock) correctionLoop() {
	ticker := time.NewTicker(CorrectionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.applyCorrection()
		}
	}
}

// applyCorrection checks drift and adjusts offset toward real time
func (c *Clock) applyCorrection() {
	// Get current synthetic time
	elapsed := time.Since(c.startMono).Nanoseconds()
	currentOffset := c.offset.Load()
	syntheticTime := c.startWall + elapsed + currentOffset

	// Get actual wall clock time
	realTime := time.Now().UTC().UnixNano()

	// Calculate drift: positive means we're ahead of real time
	drift := syntheticTime - realTime

	if drift > 0 {
		// We're ahead of real time (NTP moved clock backwards)
		// Gradually reduce offset to drift back toward real time
		correction := min(CorrectionRate, drift)
		c.offset.Add(-correction)
	} else if drift < -CorrectionRate {
		// We're significantly behind real time (NTP moved clock forward)
		// We can safely jump forward since it won't cause collisions
		// But we do it gradually to avoid large jumps
		c.offset.Add(CorrectionRate)
	}
	// If drift is small (within CorrectionRate), we're close enough - do nothing
}

// Stop stops the correction loop
func (c *Clock) Stop() {
	close(c.stopCh)
}

// Now returns a monotonically increasing timestamp using the global clock.
// Panics if Init() has not been called.
func Now() int64 {
	if globalClock == nil {
		panic("monotonic: clock not initialized, call monotonic.Init() first")
	}
	return globalClock.Now()
}

// Stop stops the global clock's correction loop.
// Panics if Init() has not been called.
func Stop() {
	if globalClock == nil {
		panic("monotonic: clock not initialized, call monotonic.Init() first")
	}
	globalClock.Stop()
}

// Reset reinitializes the global clock (useful for testing).
// This bypasses sync.Once to allow reinitialization.
func Reset() {
	if globalClock != nil {
		globalClock.Stop()
	}
	globalClock = New()
	// Reset initOnce so Init() can be called again if needed
	initOnce = sync.Once{}
}
