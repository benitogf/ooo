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
