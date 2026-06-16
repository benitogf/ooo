package storage

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestAfterWriteOpReportsOperation verifies the op-aware hook fires with the
// correct operation for each write kind: Set / SetWithMeta / Push report
// "set", Del reports "del".
func TestAfterWriteOpReportsOperation(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	ops := map[string]string{} // key -> last op

	st := New(LayeredConfig{Memory: NewMemoryLayer()})
	require.NoError(t, st.Start(Options{
		AfterWriteOp: func(key string, op string) {
			mu.Lock()
			ops[key] = op
			mu.Unlock()
		},
	}))
	defer st.Close()

	data := json.RawMessage(`{"v":1}`)

	_, err := st.Set("things/a", data)
	require.NoError(t, err)
	_, err = st.SetWithMeta("things/b", data, 1, 1)
	require.NoError(t, err)
	pushed, err := st.Push("things/*", data)
	require.NoError(t, err)
	require.NoError(t, st.Del("things/a"))

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, "del", ops["things/a"], "Set then Del should leave the del op")
	require.Equal(t, "set", ops["things/b"], "SetWithMeta should report set")
	// Push returns the leaf index; the hook (like the storage event) keys on
	// the full path "things/<index>".
	require.Equal(t, "set", ops["things/"+pushed], "Push should report set")
}

// TestAfterWriteOpReportsGlobDelete covers the glob-delete path: Del on a glob
// routes through delGlob, which fires a single "del" keyed on the glob path
// itself — not on the expanded keys it removes. This mirrors the broadcast
// event delGlob emits (Key = glob path, Object = nil), so a consumer reacting
// off AfterWriteOp sees exactly what a broadcast subscriber sees.
func TestAfterWriteOpReportsGlobDelete(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	ops := map[string]string{} // key -> last op

	st := New(LayeredConfig{Memory: NewMemoryLayer()})
	require.NoError(t, st.Start(Options{
		AfterWriteOp: func(key string, op string) {
			mu.Lock()
			ops[key] = op
			mu.Unlock()
		},
	}))
	defer st.Close()

	data := json.RawMessage(`{"v":1}`)
	_, err := st.Set("things/a", data)
	require.NoError(t, err)
	_, err = st.Set("things/b", data)
	require.NoError(t, err)

	require.NoError(t, st.Del("things/*"))

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, "del", ops["things/*"], "glob Del fires one del keyed on the glob path")
	// The expanded keys keep their prior "set" op — a per-expanded-key "del"
	// hook would have overwritten them. delGlob fires once, on the glob path.
	require.Equal(t, "set", ops["things/a"], "glob Del must not fire a per-key del for things/a")
	require.Equal(t, "set", ops["things/b"], "glob Del must not fire a per-key del for things/b")
}

// TestAfterWriteOpFiresBeforeEvent locks the ordering the version-vector bump
// depends on: AfterWriteOp runs BEFORE the storage event is broadcast, so a
// consumer that updates derived state in it has that state in place before any
// event-driven reactor (broadcast subscriber, async sync push) observes the
// write.
//
// The test parks the write goroutine inside the hook and asserts that no
// storage event escapes to a watcher while it is parked. With the hook firing
// before sendEvent, the broadcast cannot have run yet, so the window stays
// silent; if the hook were moved after the broadcast, the already-enqueued
// event would be delivered during the parked window and fail the test. (The
// previous version only stored a flag and checked it from the watch callback —
// it could never fail, because the synchronous hook always won the race against
// the async watcher regardless of which side of sendEvent it sat on.)
func TestAfterWriteOpFiresBeforeEvent(t *testing.T) {
	t.Parallel()

	inHook := make(chan struct{})
	release := make(chan struct{})
	st := New(LayeredConfig{Memory: NewMemoryLayer()})
	require.NoError(t, st.Start(Options{
		AfterWriteOp: func(key string, op string) {
			close(inHook)
			<-release
		},
	}))
	defer st.Close()

	events := make(chan Event, 1)
	WatchWithCallback(context.Background(), st, func(ev Event) {
		select {
		case events <- ev:
		default:
		}
	})

	go func() { _, _ = st.Set("test/order", json.RawMessage(`{"v":1}`)) }()

	<-inHook // the write goroutine is now parked inside AfterWriteOp

	// AfterWriteOp fires before sendEvent, so while the hook is parked the
	// broadcast has not run. Hold it open long enough that an event enqueued
	// before the hook (the inverted ordering) would surely have been delivered.
	select {
	case <-events:
		t.Fatal("storage event broadcast before AfterWriteOp returned — ordering inverted")
	case <-time.After(200 * time.Millisecond):
	}

	close(release) // let the write finish; the broadcast happens now

	select {
	case <-events:
	case <-time.After(2 * time.Second):
		t.Fatal("storage event never delivered after AfterWriteOp returned")
	}
}
