package ooo

import (
	"os"
	"testing"
)

func TestStorage(t *testing.T) {
	// t.Parallel()
	app := &Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)
	StorageListTest(app, t)
	StorageObjectTest(app, t)
}

func TestStreamBroadcast(t *testing.T) {
	// t.Parallel()
	app := Server{}
	app.Silence = true
	app.ForcePatch = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)
	StreamBroadcastTest(t, &app)
}

func TestStreamGlobBroadcast(t *testing.T) {
	// t.Parallel()
	app := Server{}
	app.Silence = true
	app.ForcePatch = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)
	StreamGlobBroadcastTest(t, &app, 3)
}

func TestStreamGlobBroadcastConcurrent(t *testing.T) {
	t.Skip()
	// t.Parallel()
	app := Server{}
	app.Silence = false
	app.ForcePatch = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)
	StreamGlobBroadcastConcurrentTest(t, &app, 3)
}

func TestStreamBroadcastFilter(t *testing.T) {
	// t.Parallel()
	app := Server{}
	app.Silence = true
	app.ForcePatch = true
	defer app.Close(os.Interrupt)
	StreamBroadcastFilterTest(t, &app)
}

func TestStreamForcePatch(t *testing.T) {
	// t.Parallel()
	app := Server{}
	app.Silence = true
	defer app.Close(os.Interrupt)
	StreamBroadcastForcePatchTest(t, &app)
}

func TestStreamNoPatch(t *testing.T) {
	// t.Parallel()
	app := Server{}
	app.Silence = true
	defer app.Close(os.Interrupt)
	StreamBroadcastNoPatchTest(t, &app)
}

func TestGetN(t *testing.T) {
	// t.Parallel()
	app := &Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)
	StorageGetNTest(app, t, 10)
}

func TestKeysRange(t *testing.T) {
	// t.Parallel()
	app := &Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)
	StorageKeysRangeTest(app, t, 10)
}

func TestStreamItemGlobBroadcastLevel(t *testing.T) {
	// t.Parallel()
	app := Server{}
	app.Silence = true
	app.ForcePatch = true
	app.Start("localhost:0")
	app.Storage.Clear()
	defer app.Close(os.Interrupt)
	StreamItemGlobBroadcastTest(t, &app)
}

func TestBatchSet(t *testing.T) {
	app := &Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)
	StorageBatchSetTest(app, t, 10)
}

func TestStreamPatch(t *testing.T) {
	app := &Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)
	StreamBroadcastPatchTest(t, app)
}

func TestStreamLimitFilter(t *testing.T) {
	app := &Server{}
	app.Silence = true
	app.Start("localhost:0")
	defer app.Close(os.Interrupt)
	StreamLimitFilterTest(t, app)
}

func TestBeforeRead(t *testing.T) {
	db := &MemoryStorage{}
	err := db.Start(StorageOpt{})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	StorageBeforeReadTest(db, t)
}

func TestWatchStorageNoop(t *testing.T) {
	db := &MemoryStorage{}
	err := db.Start(StorageOpt{})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	WatchStorageNoopTest(db, t)
}
