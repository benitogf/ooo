package meta

import (
	"bytes"
	"io"

	"github.com/goccy/go-json"
)

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

func SortAsc(obj []Object) func(i, j int) bool {
	return func(i, j int) bool {
		return obj[i].Created < obj[j].Created
	}
}

// Encode meta objects in json
func Encode(v interface{}) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return []byte(""), err
	}

	return data, nil
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

// DecodeListFromReader meta objects from io reader
func DecodeListFromReader(r io.Reader) ([]Object, error) {
	var objs []Object
	decoder := json.NewDecoder(r)
	err := decoder.Decode(&objs)

	return objs, err
}

// New meta object as json
func New(obj *Object) []byte {
	dataBytes := new(bytes.Buffer)
	json.NewEncoder(dataBytes).Encode(obj)

	return dataBytes.Bytes()
}
