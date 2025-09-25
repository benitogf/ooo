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

func GetList[T any](server *ooo.Server, path string) ([]client.Meta[T], error) {
	lastPath := key.LastIndex(path)
	isList := lastPath == "*"

	var result []client.Meta[T]
	if !isList {
		return result, errors.New("GetList[" + path + "]: path is not a list")
	}
	raw, err := server.Storage.Get(path)
	if err != nil {
		log.Println("GetList["+path+"]: failed to get from storage", err)
		return result, err
	}
	objs, err := meta.DecodeList(raw)
	if err != nil {
		log.Println("GetList["+path+"]: failed to decode data", err)
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
		return result, errors.New("Get[" + path + "]: path is a list")
	}

	raw, err := server.Storage.Get(path)
	if err != nil {
		log.Println("Get["+path+"]: failed to get from storage", err)
		return result, err
	}
	obj, err := meta.Decode(raw)
	if err != nil {
		log.Println("Get["+path+"]: failed to decode data", err)
		return result, err
	}
	var item T
	err = json.Unmarshal([]byte(obj.Data), &item)
	if err != nil {
		log.Println("Get["+path+"]: failed to unmarshal data", err)
		return result, err
	}
	result = client.Meta[T]{
		Created: obj.Created,
		Updated: obj.Updated,
		Index:   obj.Index,
		Data:    item,
	}
	return result, nil
}

func Set[T any](server *ooo.Server, path string, item T) error {
	lastPath := key.LastIndex(path)
	isList := lastPath == "*"

	if isList {
		return errors.New("Set[" + path + "]: path is a list")
	}

	data, err := json.Marshal(item)
	if err != nil {
		log.Println("Set["+path+"]: failed to marshal data", err)
		return err
	}
	_, err = server.Storage.Set(path, data)
	return err
}

func Push[T any](server *ooo.Server, path string, item T) error {
	lastPath := key.LastIndex(path)
	isList := lastPath == "*"

	if !isList {
		return errors.New("Push[" + path + "]: path is not a list")
	}

	_path := key.Build(path)

	data, err := json.Marshal(item)
	if err != nil {
		log.Println("Push["+path+"]: failed to marshal data", err)
		return err
	}
	_, err = server.Storage.Set(_path, data)
	return err
}
