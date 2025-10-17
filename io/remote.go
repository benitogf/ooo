package io

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
	var resp *http.Response
	if ssl {
		resp, err = _client.Post("https://"+host+"/"+path, "application/json", bytes.NewReader(data))
	} else {
		resp, err = _client.Post("http://"+host+"/"+path, "application/json", bytes.NewReader(data))
	}
	if err != nil {
		log.Println("RemoteSet["+path+"]: failed to post to remote", err)
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("RemoteSet["+path+"]: failed to read response", err)
		return err
	}
	if resp.StatusCode != http.StatusOK {
		log.Println("RemoteSet["+path+"]: not OK status: ", resp.Status, string(body))
		return errors.New("RemoteSet[" + path + "]: not OK status: " + resp.Status)
	}
	return nil
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
	var resp *http.Response
	if ssl {
		resp, err = _client.Post("https://"+host+"/"+_path, "application/json", bytes.NewReader(data))
	} else {
		resp, err = _client.Post("http://"+host+"/"+_path, "application/json", bytes.NewReader(data))
	}
	if err != nil {
		log.Println("RemotePush["+path+"]: failed to post to remote", err)
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("RemotePush["+path+"]: failed to read response", err)
		return err
	}
	if resp.StatusCode != http.StatusOK {
		log.Println("RemotePush["+path+"]: not OK status: ", resp.Status, string(body))
		return errors.New("RemotePush[" + path + "]: not OK status: " + resp.Status)
	}
	return nil
}

func RemoteGet[T any](_client *http.Client, ssl bool, host string, path string) (client.Meta[T], error) {
	lastPath := key.LastIndex(path)
	isList := lastPath == "*"

	if isList {
		return client.Meta[T]{}, errors.New("RemoteGet[" + path + "]: path is a list")
	}

	var resp *http.Response
	var err error
	if ssl {
		resp, err = _client.Get("https://" + host + "/" + path)
	} else {
		resp, err = _client.Get("http://" + host + "/" + path)
	}
	if err != nil {
		log.Println("RemoteGet["+path+"]: failed to get from remote", err)
		return client.Meta[T]{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("RemoteGet["+path+"]: failed to read response", err)
		return client.Meta[T]{}, err
	}
	if resp.StatusCode != http.StatusOK {
		log.Println("RemoteGet["+path+"]: not OK status: ", resp.Status, string(body))
		return client.Meta[T]{}, errors.New("RemoteGet[" + path + "]: not OK status: " + resp.Status)
	}
	obj, err := meta.Decode(body)
	if err != nil {
		log.Println("RemoteGet["+path+"]: failed to decode data", err)
		return client.Meta[T]{}, err
	}
	var item T
	err = json.Unmarshal([]byte(obj.Data), &item)
	if err != nil {
		log.Println("RemoteGet["+path+"]: failed to unmarshal data", err)
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
		return []client.Meta[T]{}, errors.New("RemoteGetList[" + path + "]: path is not a list")
	}

	var resp *http.Response
	var err error
	if ssl {
		resp, err = _client.Get("https://" + host + "/" + path)
	} else {
		resp, err = _client.Get("http://" + host + "/" + path)
	}
	if err != nil {
		log.Println("RemoteGetList["+path+"]: failed to get from remote", err)
		return []client.Meta[T]{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("RemoteGetList["+path+"]: failed to read response", err)
		return []client.Meta[T]{}, err
	}
	if resp.StatusCode != http.StatusOK {
		log.Println("RemoteGetList["+path+"]: not OK status: ", resp.Status, string(body))
		return []client.Meta[T]{}, errors.New("RemoteGetList[" + path + "]: not OK status: " + resp.Status)
	}
	objs, err := meta.DecodeList(body)
	if err != nil {
		log.Println("RemoteGetList["+path+"]: failed to decode data", err)
		return []client.Meta[T]{}, err
	}
	result := []client.Meta[T]{}
	for _, obj := range objs {
		var item T
		err = json.Unmarshal([]byte(obj.Data), &item)
		if err != nil {
			log.Println("RemoteGetList["+path+"]: failed to unmarshal data", err)
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
