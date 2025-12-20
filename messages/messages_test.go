package messages

import (
	"strings"
	"testing"

	"github.com/goccy/go-json"
	"github.com/stretchr/testify/require"
)

func TestDecodeBuffer(t *testing.T) {
	data := []byte(`{"data":{"key":"value"},"version":"abc123","snapshot":true}`)

	msg, err := DecodeBuffer(data)
	require.NoError(t, err)
	require.Equal(t, `{"key":"value"}`, string(msg.Data))
	require.Equal(t, "abc123", msg.Version)
	require.True(t, msg.Snapshot)
}

func TestDecodeBufferEmptyData(t *testing.T) {
	// Empty object {} has length > 0, so we test with truly empty data field
	data := []byte(`{"version":"abc123","snapshot":true}`)

	_, err := DecodeBuffer(data)
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty data")
}

func TestDecodeBufferInvalidJSON(t *testing.T) {
	data := []byte(`invalid json`)

	_, err := DecodeBuffer(data)
	require.Error(t, err)
}

func TestDecodeReader(t *testing.T) {
	data := `{"key":"value"}`
	reader := strings.NewReader(data)

	result, err := DecodeReader(reader)
	require.NoError(t, err)
	require.Equal(t, `{"key":"value"}`, string(result))
}

func TestDecodeReaderEmpty(t *testing.T) {
	reader := strings.NewReader(``)

	_, err := DecodeReader(reader)
	require.Error(t, err)
}

func TestPatchCacheSnapshot(t *testing.T) {
	// When snapshot is true, cache should be replaced with data
	msg := Message{
		Data:     json.RawMessage(`{"new":"data"}`),
		Version:  "v1",
		Snapshot: true,
	}
	msgBytes, _ := json.Marshal(msg)

	cache := json.RawMessage(`{"old":"cache"}`)
	result, err := PatchCache(msgBytes, cache)
	require.NoError(t, err)
	require.Equal(t, `{"new":"data"}`, string(result))
}

func TestPatchCacheEmptyPatch(t *testing.T) {
	// When patch is empty array, cache should remain unchanged
	msg := Message{
		Data:     json.RawMessage(`[]`),
		Version:  "v1",
		Snapshot: false,
	}
	msgBytes, _ := json.Marshal(msg)

	cache := json.RawMessage(`{"existing":"data"}`)
	result, err := PatchCache(msgBytes, cache)
	require.NoError(t, err)
	require.Equal(t, `{"existing":"data"}`, string(result))
}

func TestPatch(t *testing.T) {
	// Test with snapshot message
	msg := Message{
		Data:     json.RawMessage(`{"created":1000,"updated":2000,"index":"test","data":{"key":"value"}}`),
		Version:  "v1",
		Snapshot: true,
	}
	msgBytes, _ := json.Marshal(msg)

	cache := json.RawMessage(`{}`)
	newCache, obj, err := Patch(msgBytes, cache)
	require.NoError(t, err)
	require.NotEmpty(t, newCache)
	require.Equal(t, int64(1000), obj.Created)
	require.Equal(t, "test", obj.Index)
}

func TestPatchList(t *testing.T) {
	// Test with snapshot message containing a list
	msg := Message{
		Data:     json.RawMessage(`[{"created":1000,"updated":2000,"index":"1","data":{}},{"created":2000,"updated":3000,"index":"2","data":{}}]`),
		Version:  "v1",
		Snapshot: true,
	}
	msgBytes, _ := json.Marshal(msg)

	cache := json.RawMessage(`[]`)
	newCache, objs, err := PatchList(msgBytes, cache)
	require.NoError(t, err)
	require.NotEmpty(t, newCache)
	require.Len(t, objs, 2)
	require.Equal(t, int64(1000), objs[0].Created)
	require.Equal(t, int64(2000), objs[1].Created)
}

func TestPatchInvalidMessage(t *testing.T) {
	_, _, err := Patch([]byte(`invalid`), json.RawMessage(`{}`))
	require.Error(t, err)
}

func TestPatchListInvalidMessage(t *testing.T) {
	_, _, err := PatchList([]byte(`invalid`), json.RawMessage(`[]`))
	require.Error(t, err)
}
