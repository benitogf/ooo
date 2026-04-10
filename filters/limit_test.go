package filters_test

import (
	"bytes"
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/benitogf/ooo"
	"github.com/benitogf/ooo/filters"
	"github.com/benitogf/ooo/key"
	"github.com/benitogf/ooo/meta"
	"github.com/stretchr/testify/require"
)

func TestLimitFilter_Errors(t *testing.T) {
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Test limit must be positive - should panic
	require.Panics(t, func() {
		server.LimitFilter("test/*", filters.LimitFilterConfig{Limit: 0})
	})

	require.Panics(t, func() {
		server.LimitFilter("test/*", filters.LimitFilterConfig{Limit: -1})
	})

	// Test path must be glob - should panic
	require.Panics(t, func() {
		server.LimitFilter("test/specific", filters.LimitFilterConfig{Limit: 5})
	})
}

func TestLimitFilter_Valid(t *testing.T) {
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Should not panic
	require.NotPanics(t, func() {
		server.LimitFilter("test/*", filters.LimitFilterConfig{Limit: 5})
	})
}

func TestLimitFilter_Check_UnderLimit(t *testing.T) {
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	lf, err := filters.NewLimitFilter("items/*", filters.LimitFilterConfig{Limit: 5}, server.Storage)
	require.NoError(t, err)

	// Add 3 items (under limit of 5)
	for i := range 3 {
		data, _ := json.Marshal(map[string]int{"value": i})
		_, err := server.Storage.Set(key.Build("items/*"), data)
		require.NoError(t, err)
	}

	// Check should not delete anything (under limit)
	lf.Check()

	// Verify all 3 items still exist
	items, err := server.Storage.GetList("items/*")
	require.NoError(t, err)
	require.Len(t, items, 3)
}

func TestLimitFilter_ViaHTTP(t *testing.T) {
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Use server.LimitFilter which registers ReadFilter + AfterWrite
	server.LimitFilter("logs/*", filters.LimitFilterConfig{Limit: 3})
	server.OpenFilter("logs/*")

	// Add 5 items via HTTP - ReadFilter limits view, AfterWrite cleans up
	for i := range 5 {
		data, _ := json.Marshal(map[string]string{"log": "entry " + strconv.Itoa(i)})
		resp, err := server.Client.Post("http://"+server.Address+"/logs/*", "application/json", bytes.NewBuffer(data))
		require.NoError(t, err)
		require.Equal(t, 200, resp.StatusCode)
		resp.Body.Close()
	}

	// Should have exactly 3 items (limit) after cleanup
	items, err := server.Storage.GetList("logs/*")
	require.NoError(t, err)
	require.Equal(t, 3, len(items))
}

func TestLimitFilter_Check_AtLimit(t *testing.T) {
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	lf, err := filters.NewLimitFilter("items/*", filters.LimitFilterConfig{Limit: 3}, server.Storage)
	require.NoError(t, err)

	// Add 3 items (at limit)
	for i := range 3 {
		data, _ := json.Marshal(map[string]int{"value": i})
		newPath := key.Build("items/*")
		_, err := server.Storage.Set(newPath, data)
		require.NoError(t, err)
		time.Sleep(time.Millisecond) // Ensure different timestamps
	}

	// Check should NOT delete anything when at limit
	lf.Check()

	// Verify all 3 items remain
	items, err := server.Storage.GetList("items/*")
	require.NoError(t, err)
	require.Len(t, items, 3)

	// Add one more item (now over limit)
	data, _ := json.Marshal(map[string]int{"value": 3})
	newPath := key.Build("items/*")
	server.Storage.Set(newPath, data)

	// Check should now delete the oldest item
	lf.Check()

	// Verify 3 items remain (limit maintained)
	items, err = server.Storage.GetList("items/*")
	require.NoError(t, err)
	require.Len(t, items, 3)
}

func TestLimitFilter_Check_OverLimit(t *testing.T) {
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	lf, err := filters.NewLimitFilter("items/*", filters.LimitFilterConfig{Limit: 3}, server.Storage)
	require.NoError(t, err)

	// Add 4 items (over limit of 3)
	for i := range 4 {
		data, _ := json.Marshal(map[string]int{"value": i})
		newPath := key.Build("items/*")
		_, err := server.Storage.Set(newPath, data)
		require.NoError(t, err)
		time.Sleep(time.Millisecond) // Ensure different timestamps
	}

	// Check should delete the oldest item (we have 4, limit is 3)
	lf.Check()

	// Verify 3 items remain (back to limit)
	items, err := server.Storage.GetList("items/*")
	require.NoError(t, err)
	require.Len(t, items, 3)
}

func TestLimitFilter_ManualCheck(t *testing.T) {
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	lf, err := filters.NewLimitFilter("logs/*", filters.LimitFilterConfig{Limit: 3}, server.Storage)
	require.NoError(t, err)

	// Open the filter for the path
	server.OpenFilter("logs/*")

	// Add 5 items with manual limit check after each
	for i := range 5 {
		data, _ := json.Marshal(map[string]string{"log": "entry " + strconv.Itoa(i)})
		_, err := server.Storage.Set(key.Build("logs/*"), data)
		require.NoError(t, err)
		time.Sleep(time.Millisecond) // Ensure different timestamps
		// Manually check limit after each write
		lf.Check()
	}

	// Should have exactly 3 items (limit)
	items, err := server.Storage.GetList("logs/*")
	require.NoError(t, err)
	require.Equal(t, 3, len(items))
}

func TestLimitFilter_SequentialAccess(t *testing.T) {
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	lf, err := filters.NewLimitFilter("concurrent/*", filters.LimitFilterConfig{Limit: 10}, server.Storage)
	require.NoError(t, err)

	// Sequential writes with limit checks
	for i := range 20 {
		data, _ := json.Marshal(map[string]int{"value": i})
		server.Storage.Set(key.Build("concurrent/*"), data)
		lf.Check()
	}

	// Should have exactly 10 items
	items, err := server.Storage.GetList("concurrent/*")
	require.NoError(t, err)
	require.Equal(t, 10, len(items))
}

func TestLimitFilter_EmptyList(t *testing.T) {
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	lf, err := filters.NewLimitFilter("empty/*", filters.LimitFilterConfig{Limit: 5}, server.Storage)
	require.NoError(t, err)

	// Check on empty list should not delete anything
	lf.Check()

	// Verify still empty
	items, err := server.Storage.GetList("empty/*")
	require.NoError(t, err)
	require.Len(t, items, 0)
}

func TestLimitFilter_OrderDesc(t *testing.T) {
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Default order is descending (most recent first)
	lf, err := filters.NewLimitFilter("items/*", filters.LimitFilterConfig{Limit: 3}, server.Storage)
	require.NoError(t, err)
	require.Equal(t, filters.OrderDesc, lf.Order())

	// Add items with known order
	for i := range 5 {
		data, _ := json.Marshal(map[string]int{"value": i})
		_, err := server.Storage.Set(key.Build("items/*"), data)
		require.NoError(t, err)
		time.Sleep(time.Millisecond)
	}

	// Get all items first
	allItems, err := server.Storage.GetList("items/*")
	require.NoError(t, err)
	require.Len(t, allItems, 5)

	// Apply filter - should return 3 most recent, sorted descending
	filtered, err := lf.ReadListFilter("items/*", allItems)
	require.NoError(t, err)
	require.Len(t, filtered, 3)

	// Verify descending order (most recent first)
	require.True(t, filtered[0].Created > filtered[1].Created)
	require.True(t, filtered[1].Created > filtered[2].Created)
}

func TestLimitFilter_OrderAsc(t *testing.T) {
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Create with ascending order
	lf, err := filters.NewLimitFilter("items/*", filters.LimitFilterConfig{
		Limit: 3,
		Order: filters.OrderAsc,
	}, server.Storage)
	require.NoError(t, err)
	require.Equal(t, filters.OrderAsc, lf.Order())

	// Add items with known order
	for i := range 5 {
		data, _ := json.Marshal(map[string]int{"value": i})
		_, err := server.Storage.Set(key.Build("items/*"), data)
		require.NoError(t, err)
		time.Sleep(time.Millisecond)
	}

	// Get all items
	allItems, err := server.Storage.GetList("items/*")
	require.NoError(t, err)
	require.Len(t, allItems, 5)

	// Apply filter - should return 3 most recent, sorted ascending
	filtered, err := lf.ReadListFilter("items/*", allItems)
	require.NoError(t, err)
	require.Len(t, filtered, 3)

	// Verify ascending order (oldest of the kept items first)
	require.True(t, filtered[0].Created < filtered[1].Created)
	require.True(t, filtered[1].Created < filtered[2].Created)
}

func TestLimitFilter_ViaServer_OrderAsc(t *testing.T) {
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Use server.LimitFilter for ascending order
	server.LimitFilter("logs/*", filters.LimitFilterConfig{
		Limit: 3,
		Order: filters.OrderAsc,
	})

	// Add 5 items via storage
	for i := range 5 {
		data, _ := json.Marshal(map[string]string{"log": "entry " + strconv.Itoa(i)})
		_, err := server.Storage.Set(key.Build("logs/*"), data)
		require.NoError(t, err)
		time.Sleep(time.Millisecond)
	}

	// Should have 5 items in storage (no cleanup yet)
	items, err := server.Storage.GetList("logs/*")
	require.NoError(t, err)
	require.Equal(t, 5, len(items))
}

func TestLimitFilter_DynamicLimitFunc(t *testing.T) {
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Dynamic limit that starts at 3 and can be changed
	dynamicLimit := 3

	lf, err := filters.NewLimitFilter("items/*", filters.LimitFilterConfig{
		LimitFunc: func() int { return dynamicLimit },
	}, server.Storage)
	require.NoError(t, err)

	// Add 5 items
	for i := range 5 {
		data, _ := json.Marshal(map[string]int{"value": i})
		_, err := server.Storage.Set(key.Build("items/*"), data)
		require.NoError(t, err)
		time.Sleep(time.Millisecond)
	}

	// Get all items
	allItems, err := server.Storage.GetList("items/*")
	require.NoError(t, err)
	require.Len(t, allItems, 5)

	// Apply filter with limit=3
	filtered, err := lf.ReadListFilter("items/*", allItems)
	require.NoError(t, err)
	require.Len(t, filtered, 3)

	// Change dynamic limit to 5
	dynamicLimit = 5
	filtered, err = lf.ReadListFilter("items/*", allItems)
	require.NoError(t, err)
	require.Len(t, filtered, 5)

	// Change dynamic limit to 2
	dynamicLimit = 2
	filtered, err = lf.ReadListFilter("items/*", allItems)
	require.NoError(t, err)
	require.Len(t, filtered, 2)

	// Check should delete based on current dynamic limit
	lf.Check()
	items, err := server.Storage.GetList("items/*")
	require.NoError(t, err)
	require.Len(t, items, 2)
}

func TestLimitFilter_MaxAgeOnly_ReadListFilter(t *testing.T) {
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Create a time-only filter with 1 hour max age
	lf, err := filters.NewLimitFilter("logs/*", filters.LimitFilterConfig{
		MaxAge: time.Hour,
	}, server.Storage)
	require.NoError(t, err)
	require.True(t, lf.HasMaxAge())
	require.False(t, lf.HasLimit())

	// Base time in nanoseconds
	baseTime := int64(1_000_000_000_000)

	// Add items with controlled timestamps via direct storage
	for i := range 5 {
		data, _ := json.Marshal(map[string]int{"value": i})
		_, err := server.Storage.Set(key.Build("logs/*"), data)
		require.NoError(t, err)
		time.Sleep(time.Millisecond) // Ensure different timestamps
	}

	allItems, err := server.Storage.GetList("logs/*")
	require.NoError(t, err)
	require.Len(t, allItems, 5)

	// Inject a clock that is 30 minutes after the items were created
	// All items should be within the 1-hour window
	lf.SetNow(func() int64 {
		return allItems[0].Created + (30 * time.Minute).Nanoseconds()
	})

	filtered, err := lf.ReadListFilter("logs/*", allItems)
	require.NoError(t, err)
	require.Len(t, filtered, 5, "all items should be within 1-hour window")

	// Now advance clock to 2 hours after the items — all should be expired
	lf.SetNow(func() int64 {
		return allItems[len(allItems)-1].Created + (2 * time.Hour).Nanoseconds()
	})

	filtered, err = lf.ReadListFilter("logs/*", allItems)
	require.NoError(t, err)
	require.Len(t, filtered, 0, "all items should be expired after 2 hours")

	// Advance clock to just past the oldest item's age but within newest
	// Set clock so oldest item is 61 minutes old, newest is ~61 minutes old too (they're ms apart)
	// Instead: set items to have spread-out timestamps
	_ = baseTime // used for spread test below
}

func TestLimitFilter_MaxAgeOnly_SpreadTimestamps(t *testing.T) {
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	lf, err := filters.NewLimitFilter("spread/*", filters.LimitFilterConfig{
		MaxAge: time.Hour,
	}, server.Storage)
	require.NoError(t, err)

	// Add 5 items
	for i := range 5 {
		data, _ := json.Marshal(map[string]int{"value": i})
		_, err := server.Storage.Set(key.Build("spread/*"), data)
		require.NoError(t, err)
		time.Sleep(time.Millisecond)
	}

	allItems, err := server.Storage.GetList("spread/*")
	require.NoError(t, err)
	require.Len(t, allItems, 5)

	// Manually set timestamps to create a spread:
	// item 0: baseTime (oldest)
	// item 1: baseTime + 20min
	// item 2: baseTime + 40min
	// item 3: baseTime + 60min
	// item 4: baseTime + 80min
	baseTime := int64(1_000_000_000_000)
	for i := range allItems {
		allItems[i].Created = baseTime + int64(i)*20*int64(time.Minute)
	}

	// Clock at baseTime + 90min: items older than 1 hour are expired
	// Cutoff: baseTime + 90min - 60min = baseTime + 30min
	// Items 0 (baseTime) and 1 (baseTime+20min) are expired
	// Items 2 (baseTime+40min), 3 (baseTime+60min), 4 (baseTime+80min) survive
	lf.SetNow(func() int64 {
		return baseTime + 90*int64(time.Minute)
	})

	filtered, err := lf.ReadListFilter("spread/*", allItems)
	require.NoError(t, err)
	require.Len(t, filtered, 3)
}

func TestLimitFilter_MaxAgeOnly_Check(t *testing.T) {
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	lf, err := filters.NewLimitFilter("aging/*", filters.LimitFilterConfig{
		MaxAge: time.Hour,
	}, server.Storage)
	require.NoError(t, err)

	// Add 5 items
	for i := range 5 {
		data, _ := json.Marshal(map[string]int{"value": i})
		_, err := server.Storage.Set(key.Build("aging/*"), data)
		require.NoError(t, err)
		time.Sleep(time.Millisecond)
	}

	allItems, err := server.Storage.GetList("aging/*")
	require.NoError(t, err)
	require.Len(t, allItems, 5)

	// Set clock to 2 hours after the newest item — all should be expired
	lf.SetNow(func() int64 {
		return allItems[len(allItems)-1].Created + (2 * time.Hour).Nanoseconds()
	})

	lf.Check()

	items, err := server.Storage.GetList("aging/*")
	require.NoError(t, err)
	require.Len(t, items, 0, "all items should be deleted after expiration")
}

func TestLimitFilter_CountAndMaxAge_Composition(t *testing.T) {
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Both: max 3 items AND max 1 hour
	lf, err := filters.NewLimitFilter("combo/*", filters.LimitFilterConfig{
		Limit:  3,
		MaxAge: time.Hour,
	}, server.Storage)
	require.NoError(t, err)
	require.True(t, lf.HasLimit())
	require.True(t, lf.HasMaxAge())

	// Add 5 items
	for i := range 5 {
		data, _ := json.Marshal(map[string]int{"value": i})
		_, err := server.Storage.Set(key.Build("combo/*"), data)
		require.NoError(t, err)
		time.Sleep(time.Millisecond)
	}

	allItems, err := server.Storage.GetList("combo/*")
	require.NoError(t, err)
	require.Len(t, allItems, 5)

	// Set timestamps: spread over 2 hours
	baseTime := int64(1_000_000_000_000)
	for i := range allItems {
		allItems[i].Created = baseTime + int64(i)*30*int64(time.Minute)
	}
	// item 0: baseTime
	// item 1: baseTime + 30min
	// item 2: baseTime + 60min
	// item 3: baseTime + 90min
	// item 4: baseTime + 120min

	// Clock at baseTime + 100min
	// Cutoff for 1hr: baseTime + 100min - 60min = baseTime + 40min
	// Time filter keeps: item 2 (60min), item 3 (90min), item 4 (120min) → 3 items
	// Count filter: 3 <= 3, no further reduction
	lf.SetNow(func() int64 {
		return baseTime + 100*int64(time.Minute)
	})

	// Fresh copy since ReadListFilter modifies in-place
	copy1 := make([]meta.Object, len(allItems))
	copy(copy1, allItems)

	filtered, err := lf.ReadListFilter("combo/*", copy1)
	require.NoError(t, err)
	require.Len(t, filtered, 3, "time filter keeps 3, count allows 3")

	// Now test where count is the tighter constraint
	// Clock at baseTime + 50min
	// Cutoff: baseTime + 50min - 60min = baseTime - 10min (all items survive time filter)
	// Time filter keeps: all 5 items
	// Count filter: 5 > 3, keeps newest 3 → items 2,3,4
	lf.SetNow(func() int64 {
		return baseTime + 50*int64(time.Minute)
	})

	copy2 := make([]meta.Object, len(allItems))
	copy(copy2, allItems)

	filtered, err = lf.ReadListFilter("combo/*", copy2)
	require.NoError(t, err)
	require.Len(t, filtered, 3, "all survive time, count caps to 3 of 5")
}

func TestLimitFilter_CountAndMaxAge_Check(t *testing.T) {
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	lf, err := filters.NewLimitFilter("combo2/*", filters.LimitFilterConfig{
		Limit:  3,
		MaxAge: time.Hour,
	}, server.Storage)
	require.NoError(t, err)

	// Add 5 items with time.Sleep for unique timestamps
	for i := range 5 {
		data, _ := json.Marshal(map[string]int{"value": i})
		_, err := server.Storage.Set(key.Build("combo2/*"), data)
		require.NoError(t, err)
		time.Sleep(time.Millisecond)
	}

	// Set clock to just after creation — all items are fresh
	items, _ := server.Storage.GetList("combo2/*")
	lf.SetNow(func() int64 {
		return items[len(items)-1].Created + int64(time.Second)
	})

	// Check should delete 2 (5 items, limit 3, all within time window)
	lf.Check()

	items, err = server.Storage.GetList("combo2/*")
	require.NoError(t, err)
	require.Len(t, items, 3, "count limit should cap at 3")

	// Now expire all remaining by advancing clock 2 hours
	lf.SetNow(func() int64 {
		return items[len(items)-1].Created + (2 * time.Hour).Nanoseconds()
	})

	lf.Check()

	items, err = server.Storage.GetList("combo2/*")
	require.NoError(t, err)
	require.Len(t, items, 0, "all items expired by time")
}

func TestLimitFilter_DynamicMaxAgeFunc(t *testing.T) {
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	dynamicMaxAge := time.Hour

	lf, err := filters.NewLimitFilter("dynamicage/*", filters.LimitFilterConfig{
		MaxAgeFunc: func() time.Duration { return dynamicMaxAge },
	}, server.Storage)
	require.NoError(t, err)
	require.True(t, lf.IsDynamic())
	require.True(t, lf.HasMaxAge())
	require.Equal(t, time.Hour, lf.MaxAge())

	// Add 3 items
	for i := range 3 {
		data, _ := json.Marshal(map[string]int{"value": i})
		_, err := server.Storage.Set(key.Build("dynamicage/*"), data)
		require.NoError(t, err)
		time.Sleep(time.Millisecond)
	}

	allItems, err := server.Storage.GetList("dynamicage/*")
	require.NoError(t, err)
	require.Len(t, allItems, 3)

	// Clock 30min after creation — all within 1hr window
	lf.SetNow(func() int64 {
		return allItems[len(allItems)-1].Created + (30 * time.Minute).Nanoseconds()
	})

	filtered, err := lf.ReadListFilter("dynamicage/*", allItems)
	require.NoError(t, err)
	require.Len(t, filtered, 3)

	// Change dynamic max age to 10 minutes — items at 30min old are now expired
	dynamicMaxAge = 10 * time.Minute

	allItems2 := make([]meta.Object, len(allItems))
	copy(allItems2, allItems)
	filtered, err = lf.ReadListFilter("dynamicage/*", allItems2)
	require.NoError(t, err)
	require.Len(t, filtered, 0, "all items expired with 10min max age")

	// Change back to 2 hours — all items should be visible again
	dynamicMaxAge = 2 * time.Hour
	allItems3 := make([]meta.Object, len(allItems))
	copy(allItems3, allItems)
	filtered, err = lf.ReadListFilter("dynamicage/*", allItems3)
	require.NoError(t, err)
	require.Len(t, filtered, 3, "all items back within 2hr window")
}

func TestLimitFilter_MaxAge_Errors(t *testing.T) {
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Negative MaxAge should fail
	_, err := filters.NewLimitFilter("bad/*", filters.LimitFilterConfig{
		MaxAge: -time.Hour,
	}, server.Storage)
	require.ErrorIs(t, err, filters.ErrMaxAgeMustBePositive)

	// No constraint at all should fail with backwards-compat error
	_, err = filters.NewLimitFilter("bad/*", filters.LimitFilterConfig{}, server.Storage)
	require.ErrorIs(t, err, filters.ErrLimitMustBePositive)
}

func TestLimitFilter_CleanupConfig_Errors(t *testing.T) {
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Cleanup interval too low (30 seconds < 1 minute minimum)
	_, err := filters.NewLimitFilter("cleanup/*", filters.LimitFilterConfig{
		MaxAge: time.Hour,
		Cleanup: filters.CleanupConfig{
			Enabled:  true,
			Interval: 30 * time.Second,
		},
	}, server.Storage)
	require.ErrorIs(t, err, filters.ErrCleanupIntervalTooLow)

	// Cleanup with zero interval should default to 10 minutes (no error)
	lf, err := filters.NewLimitFilter("cleanup2/*", filters.LimitFilterConfig{
		MaxAge: time.Hour,
		Cleanup: filters.CleanupConfig{
			Enabled: true,
			// Interval: 0 → defaults to 10min
		},
	}, server.Storage)
	require.NoError(t, err)
	require.NotNil(t, lf)
}

func TestLimitFilter_PeriodicCleanup(t *testing.T) {
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Use server.LimitFilter with cleanup enabled and 1-minute interval
	server.LimitFilter("periodic/*", filters.LimitFilterConfig{
		MaxAge: time.Hour,
		Cleanup: filters.CleanupConfig{
			Enabled:  true,
			Interval: time.Minute,
		},
	})

	// Add items
	for i := range 3 {
		data, _ := json.Marshal(map[string]int{"value": i})
		_, err := server.Storage.Set(key.Build("periodic/*"), data)
		require.NoError(t, err)
		time.Sleep(time.Millisecond)
	}

	items, err := server.Storage.GetList("periodic/*")
	require.NoError(t, err)
	require.Len(t, items, 3, "items should exist before cleanup runs")

	// Server close should stop the cleanup goroutine (verified by not hanging)
}

func TestLimitFilter_MaxAgeOnly_ViaServer(t *testing.T) {
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Register via server convenience method
	server.LimitFilter("retention/*", filters.LimitFilterConfig{
		MaxAge: time.Hour,
	})

	// Add items via HTTP
	for i := range 3 {
		data, _ := json.Marshal(map[string]string{"log": "entry " + strconv.Itoa(i)})
		resp, err := server.Client.Post("http://"+server.Address+"/retention/*", "application/json", bytes.NewBuffer(data))
		require.NoError(t, err)
		require.Equal(t, 200, resp.StatusCode)
		resp.Body.Close()
	}

	// All items should exist (they're fresh, well within 1hr)
	items, err := server.Storage.GetList("retention/*")
	require.NoError(t, err)
	require.Len(t, items, 3)
}

func TestLimitFilter_Accessors(t *testing.T) {
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)

	// Count-only
	lf1, err := filters.NewLimitFilter("a/*", filters.LimitFilterConfig{Limit: 10}, server.Storage)
	require.NoError(t, err)
	require.True(t, lf1.HasLimit())
	require.False(t, lf1.HasMaxAge())
	require.False(t, lf1.IsDynamic())
	require.Equal(t, 10, lf1.Limit())
	require.Equal(t, time.Duration(0), lf1.MaxAge())

	// Time-only
	lf2, err := filters.NewLimitFilter("b/*", filters.LimitFilterConfig{MaxAge: 24 * time.Hour}, server.Storage)
	require.NoError(t, err)
	require.False(t, lf2.HasLimit())
	require.True(t, lf2.HasMaxAge())
	require.False(t, lf2.IsDynamic())
	require.Equal(t, 24*time.Hour, lf2.MaxAge())

	// Both with dynamic
	lf3, err := filters.NewLimitFilter("c/*", filters.LimitFilterConfig{
		Limit:      5,
		MaxAgeFunc: func() time.Duration { return time.Hour },
	}, server.Storage)
	require.NoError(t, err)
	require.True(t, lf3.HasLimit())
	require.True(t, lf3.HasMaxAge())
	require.True(t, lf3.IsDynamic())
	require.Equal(t, time.Hour, lf3.MaxAge())
}
