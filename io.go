package ooo

import (
	"errors"
	"log"

	"github.com/benitogf/go-json"
	"github.com/benitogf/ooo/client"
	"github.com/benitogf/ooo/key"
	"github.com/benitogf/ooo/merge"
)

var (
	ErrPathGlobRequired   = errors.New("io: path glob required")
	ErrPathGlobNotAllowed = errors.New("io: path glob not allowed")
)

// GetList retrieves all items at the supplied glob path. The configured
// ReadListFilter (if any) is consulted with the same static-mode flag the
// REST handler uses; a filter rejection is propagated to the caller.
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

	objs, err = server.filters.ReadList.Check(path, objs, server.Static)
	if err != nil {
		log.Println("GetList["+path+"]: read list filter rejected", err)
		return result, err
	}

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

// Get retrieves a single item at the specified path. The configured
// ReadObjectFilter (with ReadListFilter fallback) is consulted before the
// value is returned, matching the REST read handler.
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
	obj, err = server.filters.ReadObject.CheckWithListFallback(path, obj, server.Static, server.filters.ReadList)
	if err != nil {
		log.Println("Get["+path+"]: read filter rejected", err)
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

// Set stores a value at the specified path. The path must not contain glob
// patterns. The configured WriteFilter (if any) is consulted before the write
// and may reject or transform the value; the AfterWriteFilter fires on success.
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
	data, err = server.filters.Write.Check(path, data, server.Static)
	if err != nil {
		log.Println("Set["+path+"]: write filter rejected", err)
		return err
	}
	if _, err = server.Storage.Set(path, data); err != nil {
		return err
	}
	server.filters.AfterWrite.Check(path)
	return nil
}

// Push stores a value at a fresh key under the supplied glob path. The
// configured WriteFilter (if any) is consulted on the resolved path before
// the write; the AfterWriteFilter fires on success.
func Push[T any](server *Server, path string, item T) (string, error) {
	data, err := json.Marshal(item)
	if err != nil {
		log.Println("Push["+path+"]: failed to marshal data", err)
		return "", err
	}
	data, err = server.filters.Write.Check(path, data, server.Static)
	if err != nil {
		log.Println("Push["+path+"]: write filter rejected", err)
		return "", err
	}
	index, err := server.Storage.Push(path, data)
	if err != nil {
		return "", err
	}
	server.filters.AfterWrite.Check(path)
	return index, nil
}

// Delete removes an item at the specified path from storage.
// The path must not contain glob patterns. The configured DeleteFilter (if
// any) is consulted before the delete and may reject it.
func Delete(server *Server, path string) error {
	if key.IsGlob(path) {
		log.Println("Delete["+path+"]: ", ErrPathGlobNotAllowed)
		return ErrPathGlobNotAllowed
	}
	if err := server.filters.Delete.Check(path, server.Static); err != nil {
		log.Println("Delete["+path+"]: delete filter rejected", err)
		return err
	}
	return server.Storage.Del(path)
}

// Patch applies a partial update to an existing item at the specified path.
// The path must not contain glob patterns and the item must already exist.
// The patch is merged with the existing data using JSON merge semantics; the
// configured WriteFilter (if any) sees the merged result and may reject or
// transform it. The AfterWriteFilter fires on success.
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

	data, err := server.filters.Write.Check(path, json.RawMessage(mergedBytes), server.Static)
	if err != nil {
		log.Println("Patch["+path+"]: write filter rejected", err)
		return err
	}
	if _, err = server.Storage.Set(path, data); err != nil {
		return err
	}
	server.filters.AfterWrite.Check(path)
	return nil
}
