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
	for b.Loop() {
		Encode(obj)
	}
}

func BenchmarkDecode(b *testing.B) {
	data := []byte(`{"created":12345,"updated":67890,"index":"test-index","path":"test/path","data":{"key":"value"}}`)
	for b.Loop() {
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
	for b.Loop() {
		New(obj)
	}
}

func BenchmarkDecodeList(b *testing.B) {
	data := []byte(`[{"created":1,"updated":2,"index":"1","data":{}},{"created":2,"updated":3,"index":"2","data":{}},{"created":3,"updated":4,"index":"3","data":{}}]`)
	for b.Loop() {
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
	for b.Loop() {
		EncodeToBuffer(obj)
	}
}

func BenchmarkDecodePooled(b *testing.B) {
	data := []byte(`{"created":12345,"updated":67890,"index":"test-index","path":"test/path","data":{"key":"value"}}`)
	for b.Loop() {
		obj, _ := DecodePooled(data)
		if obj != nil {
			PutObject(obj)
		}
	}
}

func BenchmarkDecodeListWithCap(b *testing.B) {
	data := []byte(`[{"created":1,"updated":2,"index":"1","data":{}},{"created":2,"updated":3,"index":"2","data":{}},{"created":3,"updated":4,"index":"3","data":{}}]`)
	for b.Loop() {
		DecodeListWithCap(data, 3)
	}
}
