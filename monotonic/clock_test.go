package monotonic

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMonotonicNow(t *testing.T) {
	Init()
	// Test that Now() returns increasing values
	prev := Now()
	for range 1000 {
		curr := Now()
		require.Greater(t, curr, prev, "timestamp should always increase")
		prev = curr
	}
}

func TestMonotonicNowConcurrent(t *testing.T) {
	Init()
	// Test concurrent access - all values should be unique and increasing per goroutine
	const numGoroutines = 10
	const numIterations = 1000

	results := make([][]int64, numGoroutines)
	var wg sync.WaitGroup

	for g := 0; g < numGoroutines; g++ {
		results[g] = make([]int64, numIterations)
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := range numIterations {
				results[gid][i] = Now()
			}
		}(g)
	}

	wg.Wait()

	// Verify each goroutine got strictly increasing values
	for g := 0; g < numGoroutines; g++ {
		for i := 1; i < numIterations; i++ {
			require.Greater(t, results[g][i], results[g][i-1],
				"goroutine %d: timestamp at %d should be greater than %d", g, i, i-1)
		}
	}

	// Collect all values and verify no duplicates
	allValues := make(map[int64]bool)
	for g := 0; g < numGoroutines; g++ {
		for i := range numIterations {
			v := results[g][i]
			require.False(t, allValues[v], "duplicate timestamp found: %d", v)
			allValues[v] = true
		}
	}
}

func TestClockCorrection(t *testing.T) {
	// Create a new clock for isolated testing
	c := New()
	defer c.Stop()

	// Get initial time
	t1 := c.Now()

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	// Get another time - should be greater
	t2 := c.Now()
	require.Greater(t, t2, t1)

	// The difference should be approximately 10ms (within reasonable bounds)
	diff := t2 - t1
	require.Greater(t, diff, int64(5*time.Millisecond), "elapsed time should be at least 5ms")
	require.Less(t, diff, int64(100*time.Millisecond), "elapsed time should be less than 100ms")
}

func BenchmarkMonotonicNow(b *testing.B) {
	Init()
	for b.Loop() {
		Now()
	}
}

func BenchmarkMonotonicNowParallel(b *testing.B) {
	Init()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			Now()
		}
	})
}

// TestResetRaceWithConcurrentNow asserts the package-level
// globalClock pointer is safe to swap (via Reset) while other
// goroutines are calling Now(). Pre-fix Reset assigned globalClock
// as a bare *Clock field while Now() read the same field without
// synchronization — the race detector fires on any concurrent
// Reset + Now. The fix wraps globalClock in atomic.Pointer so the
// swap is atomic and stale readers continue safely on the old
// clock until they finish their call.
func TestResetRaceWithConcurrentNow(t *testing.T) {
	Init()
	const readers = 8
	const iterations = 200

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(readers)
	for range readers {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					Now()
				}
			}
		}()
	}

	for range iterations {
		Reset()
	}
	close(stop)
	wg.Wait()
}

func TestNowPanicWithoutInit(t *testing.T) {
	// Save current globalClock and restore after test
	original := globalClock.Swap(nil)
	defer globalClock.Store(original)

	require.Panics(t, func() {
		Now()
	}, "Now() should panic when globalClock is nil")
}

func TestStopPanicWithoutInit(t *testing.T) {
	// Save current globalClock and restore after test
	original := globalClock.Swap(nil)
	defer globalClock.Store(original)

	require.Panics(t, func() {
		Stop()
	}, "Stop() should panic when globalClock is nil")
}
