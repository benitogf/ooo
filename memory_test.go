package ooo

import (
	"os"
	"runtime"
	"testing"

	"github.com/benitogf/ooo/storage"
	"github.com/stretchr/testify/require"
)

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
