package ooo

import (
	"os"
	"runtime"
	"testing"

	"github.com/benitogf/ooo/monotonic"
	"github.com/benitogf/ooo/storage"
	"github.com/stretchr/testify/require"
)

func init() {
	monotonic.Init()
}

func TestStorage(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	app := &Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)
	StorageListTest(app, t)
	StorageObjectTest(app, t)
}

// TestStorageExtendedKeyChars asserts the storage layer accepts keys with
// hyphens, dots, and underscores so that UUIDs, ISO dates, filenames, and
// snake_case identifiers work end-to-end through Set/Get/GetList/Del.
func TestStorageExtendedKeyChars(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	app := &Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)

	keys := []string{
		"users/john-doe",
		"logs/2026-05-08",
		"data/report.json",
		"users/jane_doe",
		"550e8400-e29b-41d4-a716-446655440000",
	}

	for _, k := range keys {
		_, err := app.Storage.Set(k, TEST_DATA)
		require.NoError(t, err, "Set %q", k)

		obj, err := app.Storage.Get(k)
		require.NoError(t, err, "Get %q", k)
		require.NotEmpty(t, obj.Data)
	}

	users, err := app.Storage.GetList("users/*")
	require.NoError(t, err)
	require.Len(t, users, 2)

	logs, err := app.Storage.GetList("logs/*")
	require.NoError(t, err)
	require.Len(t, logs, 1)

	for _, k := range keys {
		require.NoError(t, app.Storage.Del(k))
	}
}

func TestStreamBroadcast(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true
	server.ForcePatch = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	StreamBroadcastTest(t, &server)
}

func TestStreamGlobBroadcast(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true
	server.ForcePatch = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	StreamGlobBroadcastTest(t, &server, 3)
}

func TestStreamGlobBroadcastConcurrent(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true
	server.ForcePatch = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	StreamGlobBroadcastConcurrentTest(t, &server, 3)
}

func TestStreamBroadcastFilter(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true
	server.ForcePatch = true
	defer server.Close(os.Interrupt)
	StreamBroadcastFilterTest(t, &server)
}

func TestStreamForcePatch(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true
	defer server.Close(os.Interrupt)
	StreamBroadcastForcePatchTest(t, &server)
}

func TestStreamNoPatch(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true
	defer server.Close(os.Interrupt)
	StreamBroadcastNoPatchTest(t, &server)
}

func TestGetN(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := &Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	StorageGetNTest(server, t, 10)
}

func TestGetNRange(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := &Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	StorageGetNRangeTest(server, t, 10)
}

func TestKeysRange(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := &Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	StorageKeysRangeTest(server, t, 10)
}

func TestStreamItemGlobBroadcast(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := Server{}
	server.Silence = true
	server.ForcePatch = true
	server.Start("localhost:0")
	server.Storage.Clear()
	defer server.Close(os.Interrupt)
	StreamItemGlobBroadcastTest(t, &server)
}

func TestBatchSet(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := &Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	StorageBatchSetTest(server, t, 10)
}

func TestStreamPatch(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := &Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	StreamBroadcastPatchTest(t, server)
}

func TestStreamLimitFilter(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := &Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	StreamLimitFilterTest(t, server)
}

func TestClientCompatibility(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := &Server{}
	server.Silence = true
	server.ForcePatch = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	ClientCompatibilityTest(t, server)
}

func TestBeforeRead(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	db := storage.New(storage.LayeredConfig{
		Memory: storage.NewMemoryLayer(),
	})
	err := db.Start(storage.Options{})
	require.NoError(t, err)
	defer db.Close()
	StorageBeforeReadTest(db, t)
}

func TestStorageAfterWrite(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	db := storage.New(storage.LayeredConfig{
		Memory: storage.NewMemoryLayer(),
	})
	err := db.Start(storage.Options{})
	require.NoError(t, err)
	defer db.Close()
	StorageAfterWriteTest(db, t)
}

func TestWatchStorageNoop(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	db := storage.New(storage.LayeredConfig{
		Memory: storage.NewMemoryLayer(),
	})
	err := db.Start(storage.Options{})
	require.NoError(t, err)
	defer db.Close()
	WatchStorageNoopTest(db, t)
}
