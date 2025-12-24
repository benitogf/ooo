package meta

import (
	"testing"

	"github.com/goccy/go-json"
)

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
