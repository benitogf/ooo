package ooo

import (
	"bytes"
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/benitogf/ooo/client"
	"github.com/benitogf/ooo/key"
	"github.com/benitogf/ooo/meta"
	"github.com/goccy/go-json"
)

func GetList[T any](server *Server, path string) ([]client.Meta[T], error) {
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

func Get[T any](server *Server, path string) (client.Meta[T], error) {
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

func Set[T any](server *Server, path string, item T) error {
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

func Push[T any](server *Server, path string, item T) error {
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

func RemoteSet[T any](_client *http.Client, ssl bool, host string, path string, item T) error {
	lastPath := key.LastIndex(path)
	isList := lastPath == "*"

	if isList {
		return errors.New("RemoteSet[" + path + "]: path is a list")
	}

	data, err := json.Marshal(item)
	if err != nil {
		log.Println("RemoteSet["+path+"]: failed to marshal data", err)
		return err
	}
	if ssl {
		_, err = _client.Post("https://"+host+"/"+path, "application/json", bytes.NewReader(data))
	} else {
		_, err = _client.Post("http://"+host+"/"+path, "application/json", bytes.NewReader(data))
	}
	return err
}

func RemotePush[T any](_client *http.Client, ssl bool, host string, path string, item T) error {
	lastPath := key.LastIndex(path)
	isList := lastPath == "*"

	if !isList {
		return errors.New("RemotePush[" + path + "]: path is not a list")
	}

	_path := key.Build(path)

	data, err := json.Marshal(item)
	if err != nil {
		log.Println("RemotePush["+path+"]: failed to marshal data", err)
		return err
	}
	if ssl {
		_, err = _client.Post("https://"+host+"/"+_path, "application/json", bytes.NewReader(data))
	} else {
		_, err = _client.Post("http://"+host+"/"+_path, "application/json", bytes.NewReader(data))
	}
	return err
}

func RemoteGet[T any](_client *http.Client, ssl bool, host string, path string) (client.Meta[T], error) {
	lastPath := key.LastIndex(path)
	isList := lastPath == "*"

	if isList {
		return client.Meta[T]{}, errors.New("GetFrom[" + path + "]: path is a list")
	}

	var resp *http.Response
	var err error
	if ssl {
		resp, err = _client.Get("https://" + host + "/" + path)
	} else {
		resp, err = _client.Get("http://" + host + "/" + path)
	}
	if err != nil {
		log.Println("GetFrom["+path+"]: failed to get from remote", err)
		return client.Meta[T]{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("GetFrom["+path+"]: failed to read response", err)
		return client.Meta[T]{}, err
	}
	obj, err := meta.Decode(body)
	if err != nil {
		log.Println("GetFrom["+path+"]: failed to decode data", err)
		return client.Meta[T]{}, err
	}
	var item T
	err = json.Unmarshal([]byte(obj.Data), &item)
	if err != nil {
		log.Println("GetFrom["+path+"]: failed to unmarshal data", err)
		return client.Meta[T]{}, err
	}
	return client.Meta[T]{
		Created: obj.Created,
		Updated: obj.Updated,
		Index:   obj.Index,
		Data:    item,
	}, nil
}

func RemoteGetList[T any](_client *http.Client, ssl bool, host string, path string) ([]client.Meta[T], error) {
	lastPath := key.LastIndex(path)
	isList := lastPath == "*"

	if !isList {
		return []client.Meta[T]{}, errors.New("GetListFrom[" + path + "]: path is not a list")
	}

	var resp *http.Response
	var err error
	if ssl {
		resp, err = _client.Get("https://" + host + "/" + path)
	} else {
		resp, err = _client.Get("http://" + host + "/" + path)
	}
	if err != nil {
		log.Println("GetListFrom["+path+"]: failed to get from remote", err)
		return []client.Meta[T]{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("GetListFrom["+path+"]: failed to read response", err)
		return []client.Meta[T]{}, err
	}
	objs, err := meta.DecodeList(body)
	if err != nil {
		log.Println("GetListFrom["+path+"]: failed to decode data", err)
		return []client.Meta[T]{}, err
	}
	result := []client.Meta[T]{}
	for _, obj := range objs {
		var item T
		err = json.Unmarshal([]byte(obj.Data), &item)
		if err != nil {
			log.Println("GetListFrom["+path+"]: failed to unmarshal data", err)
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
