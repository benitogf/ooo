package merge

// copy from https://github.com/RaveNoX/go-jsonmerge

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/goccy/go-json"
)

var (
	ErrDataJSON    = errors.New("merge: error in data JSON")
	ErrPatchJSON   = errors.New("merge: error in patch JSON")
	ErrMergedJSON  = errors.New("merge: error writing merged JSON")
	ErrPatchObject = errors.New("merge: patch value must be object")
)

// bufferPool is a pool of bytes.Buffer for reducing allocations in merge operations.
var bufferPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

// getBuffer gets a buffer from the pool and resets it.
func getBuffer() *bytes.Buffer {
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

// putBuffer returns a buffer to the pool.
func putBuffer(buf *bytes.Buffer) {
	bufferPool.Put(buf)
}

// readerPool is a pool of bytes.Reader for reducing allocations in unmarshal operations.
var readerPool = sync.Pool{
	New: func() any {
		return bytes.NewReader(nil)
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

// Info describes result of merge operation
type Info struct {
	// Errors is slice of non-critical errors of merge operations
	Errors []error
	// Replaced is describe replacements
	// Key is path in document like
	//   "prop1.prop2.prop3" for object properties or
	//   "arr1.1.prop" for arrays
	// Value is value of replacemet
	Replaced map[string]any
}

func (info *Info) mergeValue(path []string, patch map[string]any, key string, value any, newKey bool) any {
	// log.Println("merv", path, patch, value, key)
	patchValue, patchHasValue := patch[key]

	if !patchHasValue {
		return value
	}

	_, patchValueIsObject := patchValue.(map[string]any)

	path = append(path, key)
	pathStr := strings.Join(path, ".")

	_, ok := value.(map[string]any)
	if ok {
		if !patchValueIsObject {
			err := fmt.Errorf("%w for key \"%v\"", ErrPatchObject, pathStr)
			info.Errors = append(info.Errors, err)
			return value
		}

		return info.mergeObjects(value, patchValue, path)
	}

	_, ok = value.([]any)
	if ok && patchValueIsObject {
		return info.mergeObjects(value, patchValue, path)
	}

	if !jsonValuesEqual(value, patchValue) || newKey {
		info.Replaced[pathStr] = patchValue
	}

	return patchValue
}

func (info *Info) mergeObjects(data, patch any, path []string) any {
	patchObject, ok := patch.(map[string]any)
	if ok {
		dataArray, ok := data.([]any)
		if ok {
			ret := make([]any, len(dataArray))

			for i, val := range dataArray {
				ret[i] = info.mergeValue(path, patchObject, strconv.Itoa(i), val, false)
			}

			return ret
		}

		dataObject, ok := data.(map[string]any)
		if ok {
			ret := make(map[string]any)

			founds := []string{}
			for k, v := range dataObject {
				ret[k] = info.mergeValue(path, patchObject, k, v, false)
				founds = append(founds, k)
			}

			for k, v := range patchObject {
				if !slices.Contains(founds, k) {
					// ret[k] = v
					ret[k] = info.mergeValue(path, patchObject, k, v, true)
				}
			}

			return ret
		}
	}

	return data
}

// jsonValuesEqual compares two JSON values for equality without using reflect.DeepEqual.
// This is faster for primitive types commonly found in JSON (string, number, bool, nil).
func jsonValuesEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	switch va := a.(type) {
	case string:
		vb, ok := b.(string)
		return ok && va == vb
	case bool:
		vb, ok := b.(bool)
		return ok && va == vb
	case float64:
		vb, ok := b.(float64)
		return ok && va == vb
	case json.Number:
		vb, ok := b.(json.Number)
		return ok && va == vb
	default:
		// For complex types (arrays, objects), compare JSON representation
		// This is slower but handles edge cases correctly
		ja, errA := json.Marshal(a)
		jb, errB := json.Marshal(b)
		if errA != nil || errB != nil {
			return false
		}
		return bytes.Equal(ja, jb)
	}
}

// Merge merges patch document to data document
//
// Returning merged document and merge info
func Merge(data, patch any) (any, *Info) {
	info := &Info{
		Replaced: make(map[string]any),
	}
	ret := info.mergeObjects(data, patch, nil)
	return ret, info
}

// MergeBytesIndent merges patch document buffer to data document buffer
//
// # Use prefix and indent for set indentation like in json.MarshalIndent
//
// Returning merged document buffer, merge info and
// error if any
func MergeBytesIndent(dataBuff, patchBuff []byte, prefix, indent string) (mergedBuff []byte, info *Info, err error) {
	var data, patch, merged any

	err = unmarshalJSON(dataBuff, &data)
	if err != nil {
		err = fmt.Errorf("%w: %w", ErrDataJSON, err)
		return
	}

	err = unmarshalJSON(patchBuff, &patch)
	if err != nil {
		err = fmt.Errorf("%w: %w", ErrPatchJSON, err)
		return
	}

	merged, info = Merge(data, patch)

	mergedBuff, err = json.MarshalIndent(merged, prefix, indent)
	if err != nil {
		err = fmt.Errorf("%w: %w", ErrMergedJSON, err)
	}

	return
}

// MergeBytes merges patch document buffer to data document buffer
//
// Returning merged document buffer, merge info and
// error if any
func MergeBytes(dataBuff, patchBuff []byte) (mergedBuff []byte, info *Info, err error) {
	var data, patch, merged any

	err = unmarshalJSON(dataBuff, &data)
	if err != nil {
		err = fmt.Errorf("%w: %w", ErrDataJSON, err)
		return
	}

	err = unmarshalJSON(patchBuff, &patch)
	if err != nil {
		err = fmt.Errorf("%w: %w", ErrPatchJSON, err)
		return
	}

	merged, info = Merge(data, patch)

	// Use pooled buffer for encoding
	buf := getBuffer()
	encoder := json.NewEncoder(buf)
	err = encoder.Encode(merged)
	if err != nil {
		putBuffer(buf)
		err = fmt.Errorf("%w: %w", ErrMergedJSON, err)
		return
	}

	// Remove trailing newline added by Encoder and copy result
	bufData := buf.Bytes()
	if len(bufData) > 0 && bufData[len(bufData)-1] == '\n' {
		bufData = bufData[:len(bufData)-1]
	}
	mergedBuff = make([]byte, len(bufData))
	copy(mergedBuff, bufData)
	putBuffer(buf)

	return
}

func unmarshalJSON(buff []byte, data any) error {
	reader := getReader(buff)
	defer putReader(reader)
	decoder := json.NewDecoder(reader)
	decoder.UseNumber()

	return decoder.Decode(data)
}
