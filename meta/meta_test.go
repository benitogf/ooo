package meta

import (
	"bytes"
	"sort"
	"strings"
	"testing"

	"github.com/goccy/go-json"
	"github.com/stretchr/testify/require"
)

func TestEncode(t *testing.T) {
	obj := Object{
		Created: 1000,
		Updated: 2000,
		Index:   "test",
		Path:    "test/path",
		Data:    json.RawMessage(`{"key":"value"}`),
	}

	encoded, err := Encode(obj)
	require.NoError(t, err)
	require.NotEmpty(t, encoded)
	require.Contains(t, string(encoded), `"created":1000`)
	require.Contains(t, string(encoded), `"index":"test"`)
}

func TestDecode(t *testing.T) {
	data := []byte(`{"created":1000,"updated":2000,"index":"test","path":"test/path","data":{"key":"value"}}`)

	obj, err := Decode(data)
	require.NoError(t, err)
	require.Equal(t, int64(1000), obj.Created)
	require.Equal(t, int64(2000), obj.Updated)
	require.Equal(t, "test", obj.Index)
	require.Equal(t, "test/path", obj.Path)
	require.Equal(t, `{"key":"value"}`, string(obj.Data))
}

func TestDecodeInvalid(t *testing.T) {
	_, err := Decode([]byte(`invalid json`))
	require.Error(t, err)
}

func TestDecodeFromReader(t *testing.T) {
	data := `{"created":1000,"updated":2000,"index":"test","path":"test/path","data":{"key":"value"}}`
	reader := strings.NewReader(data)

	obj, err := DecodeFromReader(reader)
	require.NoError(t, err)
	require.Equal(t, int64(1000), obj.Created)
	require.Equal(t, "test", obj.Index)
}

func TestDecodeList(t *testing.T) {
	data := []byte(`[{"created":1000,"updated":2000,"index":"1","data":{}},{"created":2000,"updated":3000,"index":"2","data":{}}]`)

	objs, err := DecodeList(data)
	require.NoError(t, err)
	require.Len(t, objs, 2)
	require.Equal(t, int64(1000), objs[0].Created)
	require.Equal(t, int64(2000), objs[1].Created)
}

func TestDecodeListFromReader(t *testing.T) {
	data := `[{"created":1000,"updated":2000,"index":"1","data":{}},{"created":2000,"updated":3000,"index":"2","data":{}}]`
	reader := strings.NewReader(data)

	objs, err := DecodeListFromReader(reader)
	require.NoError(t, err)
	require.Len(t, objs, 2)
}

func TestNew(t *testing.T) {
	obj := &Object{
		Created: 1000,
		Updated: 2000,
		Index:   "test",
		Path:    "test/path",
		Data:    json.RawMessage(`{"key":"value"}`),
	}

	result := New(obj)
	require.NotEmpty(t, result)

	// Verify it can be decoded back
	decoded, err := Decode(result)
	require.NoError(t, err)
	require.Equal(t, obj.Created, decoded.Created)
	require.Equal(t, obj.Index, decoded.Index)
}

func TestSortDesc(t *testing.T) {
	objs := []Object{
		{Created: 1000},
		{Created: 3000},
		{Created: 2000},
	}

	sort.Slice(objs, SortDesc(objs))
	require.Equal(t, int64(3000), objs[0].Created)
	require.Equal(t, int64(2000), objs[1].Created)
	require.Equal(t, int64(1000), objs[2].Created)
}

func TestSortAsc(t *testing.T) {
	objs := []Object{
		{Created: 3000},
		{Created: 1000},
		{Created: 2000},
	}

	sort.Slice(objs, SortAsc(objs))
	require.Equal(t, int64(1000), objs[0].Created)
	require.Equal(t, int64(2000), objs[1].Created)
	require.Equal(t, int64(3000), objs[2].Created)
}

func TestEmptyObject(t *testing.T) {
	obj, err := Decode(EmptyObject)
	require.NoError(t, err)
	require.Equal(t, int64(0), obj.Created)
	require.Equal(t, int64(0), obj.Updated)
	require.Equal(t, "", obj.Index)
}

func TestEncodeRoundTrip(t *testing.T) {
	original := Object{
		Created: 12345,
		Updated: 67890,
		Index:   "idx",
		Path:    "some/path",
		Data:    json.RawMessage(`{"nested":{"value":123}}`),
	}

	encoded, err := Encode(original)
	require.NoError(t, err)

	decoded, err := Decode(encoded)
	require.NoError(t, err)

	require.Equal(t, original.Created, decoded.Created)
	require.Equal(t, original.Updated, decoded.Updated)
	require.Equal(t, original.Index, decoded.Index)
	require.Equal(t, original.Path, decoded.Path)
	require.True(t, bytes.Equal(original.Data, decoded.Data))
}

func BenchmarkEncode(b *testing.B) {
	obj := Object{
		Created: 12345,
		Updated: 67890,
		Index:   "test-index",
		Path:    "test/path",
		Data:    json.RawMessage(`{"key":"value","nested":{"a":1,"b":2}}`),
	}
	for i := 0; i < b.N; i++ {
		Encode(obj)
	}
}

func BenchmarkDecode(b *testing.B) {
	data := []byte(`{"created":12345,"updated":67890,"index":"test-index","path":"test/path","data":{"key":"value"}}`)
	for i := 0; i < b.N; i++ {
		Decode(data)
	}
}

func BenchmarkNew(b *testing.B) {
	obj := &Object{
		Created: 12345,
		Updated: 67890,
		Index:   "test-index",
		Path:    "test/path",
		Data:    json.RawMessage(`{"key":"value"}`),
	}
	for i := 0; i < b.N; i++ {
		New(obj)
	}
}

func BenchmarkDecodeList(b *testing.B) {
	data := []byte(`[{"created":1,"updated":2,"index":"1","data":{}},{"created":2,"updated":3,"index":"2","data":{}},{"created":3,"updated":4,"index":"3","data":{}}]`)
	for i := 0; i < b.N; i++ {
		DecodeList(data)
	}
}

func BenchmarkEncodeToBuffer(b *testing.B) {
	obj := Object{
		Created: 12345,
		Updated: 67890,
		Index:   "test-index",
		Path:    "test/path",
		Data:    json.RawMessage(`{"key":"value","nested":{"a":1,"b":2}}`),
	}
	for i := 0; i < b.N; i++ {
		EncodeToBuffer(obj)
	}
}

func BenchmarkDecodePooled(b *testing.B) {
	data := []byte(`{"created":12345,"updated":67890,"index":"test-index","path":"test/path","data":{"key":"value"}}`)
	for i := 0; i < b.N; i++ {
		obj, _ := DecodePooled(data)
		if obj != nil {
			PutObject(obj)
		}
	}
}

func BenchmarkDecodeListWithCap(b *testing.B) {
	data := []byte(`[{"created":1,"updated":2,"index":"1","data":{}},{"created":2,"updated":3,"index":"2","data":{}},{"created":3,"updated":4,"index":"3","data":{}}]`)
	for i := 0; i < b.N; i++ {
		DecodeListWithCap(data, 3)
	}
}
