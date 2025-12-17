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

func RemoteSet[T any](_client *http.Client, ssl bool, host string, path string, item T, header http.Header) error {
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
	var url string
	if ssl {
		url = "https://" + host + "/" + path
	} else {
		url = "http://" + host + "/" + path
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		log.Println("RemoteSet["+path+"]: failed to create request", err)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := _client.Do(req)
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

func RemotePush[T any](_client *http.Client, ssl bool, host string, path string, item T, header http.Header) error {
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
	var url string
	if ssl {
		url = "https://" + host + "/" + _path
	} else {
		url = "http://" + host + "/" + _path
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		log.Println("RemotePush["+path+"]: failed to create request", err)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := _client.Do(req)
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

func RemoteGet[T any](_client *http.Client, ssl bool, host string, path string, header http.Header) (client.Meta[T], error) {
	lastPath := key.LastIndex(path)
	isList := lastPath == "*"

	if isList {
		return client.Meta[T]{}, errors.New("RemoteGet[" + path + "]: path is a list")
	}

	var url string
	if ssl {
		url = "https://" + host + "/" + path
	} else {
		url = "http://" + host + "/" + path
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Println("RemoteGet["+path+"]: failed to create request", err)
		return client.Meta[T]{}, err
	}
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := _client.Do(req)
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

func RemoteGetList[T any](_client *http.Client, ssl bool, host string, path string, header http.Header) ([]client.Meta[T], error) {
	lastPath := key.LastIndex(path)
	isList := lastPath == "*"

	if !isList {
		return []client.Meta[T]{}, errors.New("RemoteGetList[" + path + "]: path is not a list")
	}

	var url string
	if ssl {
		url = "https://" + host + "/" + path
	} else {
		url = "http://" + host + "/" + path
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Println("RemoteGetList["+path+"]: failed to create request", err)
		return []client.Meta[T]{}, err
	}
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := _client.Do(req)
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
