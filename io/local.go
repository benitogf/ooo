package io

import (
	"errors"
	"log"

	"github.com/benitogf/ooo"
	"github.com/benitogf/ooo/client"
	"github.com/benitogf/ooo/key"
	"github.com/benitogf/ooo/meta"
	"github.com/goccy/go-json"
)

var (
	ErrPathGlobRequired   = errors.New("io: path glob required")
	ErrPathGlobNotAllowed = errors.New("io: path glob not allowed")
)

func GetList[T any](server *ooo.Server, path string) ([]client.Meta[T], error) {
	lastPath := key.LastIndex(path)
	isList := lastPath == "*"

	var result []client.Meta[T]
	if !isList {
		log.Println("GetList["+path+"]: ", ErrPathGlobRequired)
		return result, ErrPathGlobRequired
	}

	// Use GetNAscending with a large limit to get all items
	objs, err := server.Storage.GetNAscending(path, 0)
	if err != nil {
		log.Println("GetList["+path+"]: failed to get from storage", err)
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

func Get[T any](server *ooo.Server, path string) (client.Meta[T], error) {
	lastPath := key.LastIndex(path)
	isList := lastPath == "*"

	var result client.Meta[T]
	if isList {
		log.Println("Get["+path+"]: ", ErrPathGlobNotAllowed)
		return result, ErrPathGlobNotAllowed
	}

	raw, err := server.Storage.Get(path)
	if err != nil {
		log.Println("Get["+path+"]: failed to get from storage", err)
		return result, err
	}
	obj, err := meta.DecodePooled(raw)
	if err != nil {
		log.Println("Get["+path+"]: failed to decode data", err)
		return result, err
	}
	var item T
	err = json.Unmarshal([]byte(obj.Data), &item)
	if err != nil {
		meta.PutObject(obj)
		log.Println("Get["+path+"]: failed to unmarshal data", err)
		return result, err
	}
	result = client.Meta[T]{
		Created: obj.Created,
		Updated: obj.Updated,
		Index:   obj.Index,
		Data:    item,
	}
	meta.PutObject(obj)
	return result, nil
}

func Set[T any](server *ooo.Server, path string, item T) error {
	lastPath := key.LastIndex(path)
	isList := lastPath == "*"

	if isList {
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

func Push[T any](server *ooo.Server, path string, item T) (string, error) {
	data, err := json.Marshal(item)
	if err != nil {
		log.Println("Push["+path+"]: failed to marshal data", err)
		return "", err
	}
	return server.Storage.Push(path, data)
}
