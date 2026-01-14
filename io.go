package ooo

import (
	"errors"
	"log"

	"github.com/benitogf/ooo/client"
	"github.com/benitogf/ooo/key"
	"github.com/benitogf/ooo/merge"
	"github.com/goccy/go-json"
)

var (
	ErrPathGlobRequired   = errors.New("io: path glob required")
	ErrPathGlobNotAllowed = errors.New("io: path glob not allowed")
)

func GetList[T any](server *Server, path string) ([]client.Meta[T], error) {
	var result []client.Meta[T]
	if !key.IsGlob(path) {
		log.Println("GetList["+path+"]: ", ErrPathGlobRequired)
		return result, ErrPathGlobRequired
	}

	// Use GetNAscending with a large limit to get all items
	objs, err := server.Storage.GetNAscending(path, 0)
	if err != nil {
		log.Println("GetList["+path+"]: failed to get from storage", err)
		return result, err
	}

	// Apply ReadListFilter if registered for this path
	objs, _ = server.filters.ReadList.Check(path, objs, false)

	for _, obj := range objs {
		var item T
		err = json.Unmarshal([]byte(obj.Data), &item)
		if err != nil {
			log.Println("GetList["+path+"]: failed to unmarshal data", err)
			continue
		}
		result = append(result, client.Meta[T]{
			Created: obj.Created,
			Updated: obj.Updated,
			Index:   obj.Index,
			Data:    item,
		})
	}
	return result, nil
}

func Get[T any](server *Server, path string) (client.Meta[T], error) {
	var result client.Meta[T]
	if key.IsGlob(path) {
		log.Println("Get["+path+"]: ", ErrPathGlobNotAllowed)
		return result, ErrPathGlobNotAllowed
	}

	obj, err := server.Storage.Get(path)
	if err != nil {
		log.Println("Get["+path+"]: failed to get from storage", err)
		return result, err
	}
	var item T
	err = json.Unmarshal([]byte(obj.Data), &item)
	if err != nil {
		log.Println("Get["+path+"]: failed to unmarshal data", err)
		return result, err
	}
	return client.Meta[T]{
		Created: obj.Created,
		Updated: obj.Updated,
		Index:   obj.Index,
		Data:    item,
	}, nil
}

func Set[T any](server *Server, path string, item T) error {
	if key.IsGlob(path) {
		log.Println("Set["+path+"]: ", ErrPathGlobNotAllowed)
		return ErrPathGlobNotAllowed
	}

	data, err := json.Marshal(item)
	if err != nil {
		log.Println("Set["+path+"]: failed to marshal data", err)
		return err
	}
	_, err = server.Storage.Set(path, data)
	return err
}

func Push[T any](server *Server, path string, item T) (string, error) {
	data, err := json.Marshal(item)
	if err != nil {
		log.Println("Push["+path+"]: failed to marshal data", err)
		return "", err
	}
	return server.Storage.Push(path, data)
}

// Delete removes an item at the specified path from storage.
// The path must not contain glob patterns.
func Delete(server *Server, path string) error {
	if key.IsGlob(path) {
		log.Println("Delete["+path+"]: ", ErrPathGlobNotAllowed)
		return ErrPathGlobNotAllowed
	}
	return server.Storage.Del(path)
}

// Patch applies a partial update to an existing item at the specified path.
// The path must not contain glob patterns and the item must already exist.
// The patch is merged with the existing data using JSON merge semantics.
func Patch[T any](server *Server, path string, item T) error {
	if key.IsGlob(path) {
		log.Println("Patch["+path+"]: ", ErrPathGlobNotAllowed)
		return ErrPathGlobNotAllowed
	}

	currentObj, err := server.Storage.Get(path)
	if err != nil {
		log.Println("Patch["+path+"]: failed to get current data", err)
		return err
	}

	patchData, err := json.Marshal(item)
	if err != nil {
		log.Println("Patch["+path+"]: failed to marshal patch data", err)
		return err
	}

	mergedBytes, _, err := merge.MergeBytes(currentObj.Data, patchData)
	if err != nil {
		log.Println("Patch["+path+"]: failed to merge data", err)
		return err
	}

	_, err = server.Storage.Set(path, mergedBytes)
	return err
}
