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

func TestGetObject_PutObject(t *testing.T) {
	obj := GetObject()
	require.NotNil(t, obj)
	require.Equal(t, int64(0), obj.Created)
	require.Equal(t, int64(0), obj.Updated)
	require.Equal(t, "", obj.Index)
	require.Equal(t, "", obj.Path)
	require.Nil(t, obj.Data)

	// Modify and return to pool
	obj.Created = 123
	obj.Index = "test"
	obj.Data = json.RawMessage(`{}`)
	PutObject(obj)

	// Get again - should be reset
	obj2 := GetObject()
	require.Equal(t, int64(0), obj2.Created)
	require.Equal(t, "", obj2.Index)
	require.Nil(t, obj2.Data)
	PutObject(obj2)
}

func TestDecodePooled_Success(t *testing.T) {
	data := []byte(`{"created":1000,"updated":2000,"index":"test","path":"test/path","data":{"key":"value"}}`)
	obj, err := DecodePooled(data)
	require.NoError(t, err)
	require.NotNil(t, obj)
	require.Equal(t, int64(1000), obj.Created)
	require.Equal(t, int64(2000), obj.Updated)
	require.Equal(t, "test", obj.Index)
	PutObject(obj)
}

func TestDecodePooled_Error(t *testing.T) {
	data := []byte(`invalid json`)
	obj, err := DecodePooled(data)
	require.Error(t, err)
	require.Nil(t, obj)
}

func TestEncodeToBuffer_Success(t *testing.T) {
	obj := Object{
		Created: 12345,
		Updated: 67890,
		Index:   "test-index",
		Path:    "test/path",
		Data:    json.RawMessage(`{"key":"value"}`),
	}
	result, err := EncodeToBuffer(obj)
	require.NoError(t, err)
	require.NotEmpty(t, result)

	// Verify it can be decoded back
	decoded, err := Decode(result)
	require.NoError(t, err)
	require.Equal(t, obj.Created, decoded.Created)
	require.Equal(t, obj.Index, decoded.Index)
}

func TestDecodeListWithCap_Success(t *testing.T) {
	data := []byte(`[{"created":1,"updated":2,"index":"1","data":{}},{"created":2,"updated":3,"index":"2","data":{}}]`)
	objs, err := DecodeListWithCap(data, 5)
	require.NoError(t, err)
	require.Len(t, objs, 2)
	require.Equal(t, int64(1), objs[0].Created)
	require.Equal(t, int64(2), objs[1].Created)
}

func TestDecodeListWithCap_Error(t *testing.T) {
	data := []byte(`invalid json`)
	objs, err := DecodeListWithCap(data, 5)
	require.Error(t, err)
	require.Nil(t, objs)
}

func TestDecodeList_Error(t *testing.T) {
	data := []byte(`invalid json`)
	_, err := DecodeList(data)
	require.Error(t, err)
}

func TestDecodeFromReader_Error(t *testing.T) {
	reader := strings.NewReader(`invalid json`)
	_, err := DecodeFromReader(reader)
	require.Error(t, err)
}

func TestDecodeListFromReader_Error(t *testing.T) {
	reader := strings.NewReader(`invalid json`)
	_, err := DecodeListFromReader(reader)
	require.Error(t, err)
}

func TestBufferPool_Reuse(t *testing.T) {
	// Encode multiple objects to exercise buffer pool
	for range 100 {
		obj := Object{
			Created: int64(0),
			Index:   "test",
			Data:    json.RawMessage(`{}`),
		}
		_, err := EncodeToBuffer(obj)
		require.NoError(t, err)
	}
}

func TestObjectPool_Reuse(t *testing.T) {
	// Get and put multiple objects to exercise pool
	for i := range 100 {
		obj := GetObject()
		obj.Created = int64(i)
		PutObject(obj)
	}
}
