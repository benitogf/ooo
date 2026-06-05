package storage

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"

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

// TestAfterWriteOpFiresBeforeEvent locks the ordering the version-vector bump
// depends on: AfterWriteOp runs BEFORE the storage event is broadcast, so a
// consumer that updates derived state in it has that state in place before any
// event-driven reactor (broadcast subscriber, async sync push) observes the
// write. The event is not enqueued until after AfterWriteOp returns, so the
// watch callback deterministically sees the hook's effect already applied.
func TestAfterWriteOpFiresBeforeEvent(t *testing.T) {
	t.Parallel()

	var opRan atomic.Bool
	st := New(LayeredConfig{Memory: NewMemoryLayer()})
	require.NoError(t, st.Start(Options{
		AfterWriteOp: func(key string, op string) { opRan.Store(true) },
	}))
	defer st.Close()

	observed := make(chan bool, 1)
	WatchWithCallback(st, func(ev Event) {
		select {
		case observed <- opRan.Load():
		default:
		}
	})

	_, err := st.Set("test/order", json.RawMessage(`{"v":1}`))
	require.NoError(t, err)

	require.True(t, <-observed, "AfterWriteOp must run before the storage event is delivered")
}
