package meta

import (
	"bytes"
	"io"
	"sync"

	"github.com/goccy/go-json"
)

// bufferPool is a pool of bytes.Buffer for reducing allocations in encode operations.
var bufferPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

// getBuffer gets a buffer from the pool.
func getBuffer() *bytes.Buffer {
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

// putBuffer returns a buffer to the pool.
func putBuffer(buf *bytes.Buffer) {
	bufferPool.Put(buf)
}

// objectPool is a pool of Object structs for reducing allocations in decode operations.
var objectPool = sync.Pool{
	New: func() any {
		return new(Object)
	},
}

// readerPool is a pool of bytes.Reader for reducing allocations in decode operations.
var readerPool = sync.Pool{
	New: func() any {
		return new(bytes.Reader)
	},
}

// getReader gets a reader from the pool and resets it with the given data.
func getReader(data []byte) *bytes.Reader {
	r := readerPool.Get().(*bytes.Reader)
	r.Reset(data)
	return r
}

// putReader returns a reader to the pool.
func putReader(r *bytes.Reader) {
	readerPool.Put(r)
}

// GetObject gets an Object from the pool. Call PutObject when done.
func GetObject() *Object {
	return objectPool.Get().(*Object)
}

// PutObject returns an Object to the pool. Resets all fields.
func PutObject(obj *Object) {
	obj.Created = 0
	obj.Updated = 0
	obj.Index = ""
	obj.Path = ""
	obj.Data = nil
	objectPool.Put(obj)
}

// Meta data structure of elements
type Object struct {
	Created int64           `json:"created"`
	Updated int64           `json:"updated"`
	Index   string          `json:"index"`
	Path    string          `json:"path"`
	Data    json.RawMessage `json:"data"`
}

// Empty meta object byte array value
var EmptyObject = []byte(`{ "created": 0, "updated": 0, "index": "", "data": {} }`)

// SortDesc by created
func SortDesc(obj []Object) func(i, j int) bool {
	return func(i, j int) bool {
		return obj[i].Created > obj[j].Created
	}
}

// SortAsc returns a sort function that sorts objects by created timestamp ascending.
func SortAsc(obj []Object) func(i, j int) bool {
	return func(i, j int) bool {
		return obj[i].Created < obj[j].Created
	}
}

// Encode meta objects in json
func Encode(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// EncodeToBuffer encodes to a pooled buffer and returns a copy of the bytes.
// This reduces allocations when encoding many objects in sequence.
func EncodeToBuffer(v interface{}) ([]byte, error) {
	buf := getBuffer()
	defer putBuffer(buf)

	encoder := json.NewEncoder(buf)
	if err := encoder.Encode(v); err != nil {
		return nil, err
	}

	// Remove trailing newline added by Encoder
	data := buf.Bytes()
	if len(data) > 0 && data[len(data)-1] == '\n' {
		data = data[:len(data)-1]
	}

	// Return a copy since buffer will be reused
	result := make([]byte, len(data))
	copy(result, data)
	return result, nil
}

// Decode json meta object
func Decode(data []byte) (Object, error) {
	var obj Object
	err := json.Unmarshal(data, &obj)
	if err != nil {
		return obj, err
	}

	return obj, err
}

// DecodePooled decodes into a pooled Object. Call PutObject when done with the result.
// This reduces allocations in hot paths where many objects are decoded.
func DecodePooled(data []byte) (*Object, error) {
	obj := GetObject()
	err := json.Unmarshal(data, obj)
	if err != nil {
		PutObject(obj)
		return nil, err
	}
	return obj, nil
}

// DecodeFromReader meta object from io reader
func DecodeFromReader(r io.Reader) (Object, error) {
	var obj Object
	decoder := json.NewDecoder(r)
	err := decoder.Decode(&obj)

	return obj, err
}

// DecodeList json
func DecodeList(data []byte) ([]Object, error) {
	var objects []Object
	err := json.Unmarshal(data, &objects)
	if err != nil {
		return objects, err
	}

	return objects, nil
}

// DecodeListWithCap decodes a JSON array with pre-allocated capacity.
// Use this when you have an estimate of the number of objects to reduce allocations.
func DecodeListWithCap(data []byte, capacity int) ([]Object, error) {
	objects := make([]Object, 0, capacity)
	err := json.Unmarshal(data, &objects)
	if err != nil {
		return nil, err
	}
	return objects, nil
}

// DecodeListFromReader meta objects from io reader
func DecodeListFromReader(r io.Reader) ([]Object, error) {
	var objs []Object
	decoder := json.NewDecoder(r)
	err := decoder.Decode(&objs)

	return objs, err
}

// New encodes a meta object to JSON bytes.
// Returns nil if encoding fails.
func New(obj *Object) []byte {
	data, _ := json.Marshal(obj)
	return data
}
