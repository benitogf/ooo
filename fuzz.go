//go:build gofuzz
// +build gofuzz

package ooo

import (
	"fmt"
	"github.com/benitogf/jsondiff"
	"github.com/benitogf/ooo/meta"
	"github.com/goccy/go-json"
)

type TestObject struct {
	Data string `json:"data"`
}

// https://medium.com/@dgryski/go-fuzz-github-com-arolek-ase-3c74d5a3150c
// go get -u github.com/dvyukov/go-fuzz/go-fuzz-build
// go get -u github.com/dvyukov/go-fuzz/go-fuzz
// go-fuzz-build github.com/benitogf/ooo
// go-fuzz -bin='ooo-fuzz.zip' -workdir=fuzz
func Fuzz(fdata []byte) int {
	data := fmt.Sprintf("%#v", string(fdata))
	memory := &MemoryStorage{}
	fuzzStorage(memory, data)
	return 1
}

func fuzzStorage(storage Database, data string) {
	storage.Start(StorageOpt{})

	testData := TestObject{
		Data: data,
	}
	encodedData, err := json.Marshal(testData)
	if err != nil {
		panic(err)
	}
	_, err = storage.Set("fuzz", json.RawMessage(encodedData))
	if err != nil {
		storage.Close()
		panic(err)
	}
	raw, err := storage.Get("fuzz")
	if err != nil {
		storage.Close()
		panic(err)
	}
	var obj meta.Object
	err = json.Unmarshal(raw, &obj)
	if err != nil {
		storage.Close()
		panic(err)
	}

	same, _ := jsondiff.Compare(obj.Data, json.RawMessage(data), &jsondiff.Options{})
	if same != jsondiff.FullMatch {
		panic("data != obj.Data: " + string(obj.Data) + " : " + string(data))
	}
	err = storage.Del("fuzz")
	if err != nil {
		storage.Close()
		panic(err)
	}
	post, err := storage.Get("fuzz")
	if err == nil {
		storage.Close()
		panic("expected empty but got: " + string(post))
	}
	storage.Close()
}
