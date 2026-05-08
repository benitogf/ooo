package ooo_test

import (
	"os"
	"runtime"
	"testing"

	"github.com/benitogf/ooo"
	"github.com/benitogf/ooo/monotonic"
	"github.com/benitogf/ooo/oootest"
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
	app := &ooo.Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)
	oootest.StorageListTest(app, t)
	oootest.StorageObjectTest(app, t)
}

func TestStreamBroadcast(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.ForcePatch = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	oootest.StreamBroadcastTest(t, &server)
}

func TestStreamGlobBroadcast(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.ForcePatch = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	oootest.StreamGlobBroadcastTest(t, &server, 3)
}

func TestStreamGlobBroadcastConcurrent(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.ForcePatch = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	oootest.StreamGlobBroadcastConcurrentTest(t, &server, 3)
}

func TestStreamBroadcastFilter(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.ForcePatch = true
	defer server.Close(os.Interrupt)
	oootest.StreamBroadcastFilterTest(t, &server)
}

func TestStreamForcePatch(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	defer server.Close(os.Interrupt)
	oootest.StreamBroadcastForcePatchTest(t, &server)
}

func TestStreamNoPatch(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	defer server.Close(os.Interrupt)
	oootest.StreamBroadcastNoPatchTest(t, &server)
}

func TestGetN(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := &ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	oootest.StorageGetNTest(server, t, 10)
}

func TestGetNRange(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := &ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	oootest.StorageGetNRangeTest(server, t, 10)
}

func TestKeysRange(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := &ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	oootest.StorageKeysRangeTest(server, t, 10)
}

func TestStreamItemGlobBroadcast(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := ooo.Server{}
	server.Silence = true
	server.ForcePatch = true
	server.Start("localhost:0")
	server.Storage.Clear()
	defer server.Close(os.Interrupt)
	oootest.StreamItemGlobBroadcastTest(t, &server)
}

func TestBatchSet(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := &ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	oootest.StorageBatchSetTest(server, t, 10)
}

func TestStreamPatch(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := &ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	oootest.StreamBroadcastPatchTest(t, server)
}

func TestStreamLimitFilter(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := &ooo.Server{}
	server.Silence = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	oootest.StreamLimitFilterTest(t, server)
}

func TestClientCompatibility(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Parallel()
	}
	server := &ooo.Server{}
	server.Silence = true
	server.ForcePatch = true
	server.Start("localhost:0")
	defer server.Close(os.Interrupt)
	oootest.ClientCompatibilityTest(t, server)
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
	oootest.StorageBeforeReadTest(db, t)
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
	oootest.StorageAfterWriteTest(db, t)
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
	oootest.WatchStorageNoopTest(db, t)
}
