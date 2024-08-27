package merge

// copy from https://github.com/RaveNoX/go-jsonmerge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// Info describes result of merge operation
type Info struct {
	// Errors is slice of non-critical errors of merge operations
	Errors []error
	// Replaced is describe replacements
	// Key is path in document like
	//   "prop1.prop2.prop3" for object properties or
	//   "arr1.1.prop" for arrays
	// Value is value of replacemet
	Replaced map[string]interface{}
}

func contains(value string, list []string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}

	return false
}

func (info *Info) mergeValue(path []string, patch map[string]interface{}, key string, value interface{}, newKey bool) interface{} {
	// log.Println("merv", path, patch, value, key)
	patchValue, patchHasValue := patch[key]

	if !patchHasValue {
		return value
	}

	_, patchValueIsObject := patchValue.(map[string]interface{})

	path = append(path, key)
	pathStr := strings.Join(path, ".")

	_, ok := value.(map[string]interface{})
	if ok {
		if !patchValueIsObject {
			err := fmt.Errorf("patch value must be object for key \"%v\"", pathStr)
			info.Errors = append(info.Errors, err)
			return value
		}

		return info.mergeObjects(value, patchValue, path)
	}

	_, ok = value.([]interface{})
	if ok && patchValueIsObject {
		return info.mergeObjects(value, patchValue, path)
	}

	if !reflect.DeepEqual(value, patchValue) || newKey {
		info.Replaced[pathStr] = patchValue
	}

	return patchValue
}

func (info *Info) mergeObjects(data, patch interface{}, path []string) interface{} {
	patchObject, ok := patch.(map[string]interface{})
	if ok {
		dataArray, ok := data.([]interface{})
		if ok {
			ret := make([]interface{}, len(dataArray))

			for i, val := range dataArray {
				ret[i] = info.mergeValue(path, patchObject, strconv.Itoa(i), val, false)
			}

			return ret
		}

		dataObject, ok := data.(map[string]interface{})
		if ok {
			ret := make(map[string]interface{})

			founds := []string{}
			for k, v := range dataObject {
				ret[k] = info.mergeValue(path, patchObject, k, v, false)
				founds = append(founds, k)
			}

			for k, v := range patchObject {
				if !contains(k, founds) {
					// ret[k] = v
					ret[k] = info.mergeValue(path, patchObject, k, v, true)
				}
			}

			return ret
		}
	}

	return data
}

// Merge merges patch document to data document
//
// Returning merged document and merge info
func Merge(data, patch interface{}) (interface{}, *Info) {
	info := &Info{
		Replaced: make(map[string]interface{}),
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
	var data, patch, merged interface{}

	err = unmarshalJSON(dataBuff, &data)
	if err != nil {
		err = fmt.Errorf("error in data JSON: %v", err)
		return
	}

	err = unmarshalJSON(patchBuff, &patch)
	if err != nil {
		err = fmt.Errorf("error in patch JSON: %v", err)
		return
	}

	merged, info = Merge(data, patch)

	mergedBuff, err = json.MarshalIndent(merged, prefix, indent)
	if err != nil {
		err = fmt.Errorf("error writing merged JSON: %v", err)
	}

	return
}

// MergeBytes merges patch document buffer to data document buffer
//
// Returning merged document buffer, merge info and
// error if any
func MergeBytes(dataBuff, patchBuff []byte) (mergedBuff []byte, info *Info, err error) {
	var data, patch, merged interface{}

	err = unmarshalJSON(dataBuff, &data)
	if err != nil {
		err = fmt.Errorf("error in data JSON: %v", err)
		return
	}

	err = unmarshalJSON(patchBuff, &patch)
	if err != nil {
		err = fmt.Errorf("error in patch JSON: %v", err)
		return
	}

	merged, info = Merge(data, patch)

	mergedBuff, err = json.Marshal(merged)
	if err != nil {
		err = fmt.Errorf("error writing merged JSON: %v", err)
	}

	return
}

func unmarshalJSON(buff []byte, data interface{}) error {
	decoder := json.NewDecoder(bytes.NewReader(buff))
	decoder.UseNumber()

	return decoder.Decode(data)
}
