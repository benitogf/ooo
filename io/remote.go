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

// RemoteConfig holds connection configuration for remote operations.
// Required fields: Client, Host.
// Optional fields: SSL, Header.
type RemoteConfig struct {
	Client *http.Client
	SSL    bool
	Host   string
	Header http.Header
}

// URL returns the full URL for a given path based on the config.
func (c *RemoteConfig) URL(path string) string {
	if c.SSL {
		return "https://" + c.Host + "/" + path
	}
	return "http://" + c.Host + "/" + path
}

// Validate checks that required fields are set.
func (c *RemoteConfig) Validate() error {
	if c.Client == nil {
		return errors.New("io: Client is required")
	}
	if c.Host == "" {
		return errors.New("io: Host is required")
	}
	return nil
}

func RemoteSet[T any](cfg RemoteConfig, path string, item T) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
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
	req, err := http.NewRequest("POST", cfg.URL(path), bytes.NewReader(data))
	if err != nil {
		log.Println("RemoteSet["+path+"]: failed to create request", err)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range cfg.Header {
		req.Header[k] = v
	}
	resp, err := cfg.Client.Do(req)
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

func RemotePush[T any](cfg RemoteConfig, path string, item T) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
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
	req, err := http.NewRequest("POST", cfg.URL(_path), bytes.NewReader(data))
	if err != nil {
		log.Println("RemotePush["+path+"]: failed to create request", err)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range cfg.Header {
		req.Header[k] = v
	}
	resp, err := cfg.Client.Do(req)
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

func RemoteGet[T any](cfg RemoteConfig, path string) (client.Meta[T], error) {
	if err := cfg.Validate(); err != nil {
		return client.Meta[T]{}, err
	}
	lastPath := key.LastIndex(path)
	isList := lastPath == "*"

	if isList {
		return client.Meta[T]{}, errors.New("RemoteGet[" + path + "]: path is a list")
	}

	req, err := http.NewRequest("GET", cfg.URL(path), nil)
	if err != nil {
		log.Println("RemoteGet["+path+"]: failed to create request", err)
		return client.Meta[T]{}, err
	}
	for k, v := range cfg.Header {
		req.Header[k] = v
	}
	resp, err := cfg.Client.Do(req)
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

func RemoteGetList[T any](cfg RemoteConfig, path string) ([]client.Meta[T], error) {
	if err := cfg.Validate(); err != nil {
		return []client.Meta[T]{}, err
	}
	lastPath := key.LastIndex(path)
	isList := lastPath == "*"

	if !isList {
		return []client.Meta[T]{}, errors.New("RemoteGetList[" + path + "]: path is not a list")
	}

	req, err := http.NewRequest("GET", cfg.URL(path), nil)
	if err != nil {
		log.Println("RemoteGetList["+path+"]: failed to create request", err)
		return []client.Meta[T]{}, err
	}
	for k, v := range cfg.Header {
		req.Header[k] = v
	}
	resp, err := cfg.Client.Do(req)
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
