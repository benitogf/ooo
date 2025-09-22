package messages

import (
	"errors"
	"io"
	"log"

	"github.com/benitogf/jsonpatch"
	"github.com/benitogf/ooo/meta"
	"github.com/goccy/go-json"
)

// Message sent through websocket connections
type Message struct {
	Data     json.RawMessage `json:"data"`
	Version  string          `json:"version"`
	Snapshot bool            `json:"snapshot"`
}

// DecodeTest data (testing function)
func DecodeBuffer(data []byte) (Message, error) {
	var wsEvent Message
	err := json.Unmarshal(data, &wsEvent)
	if len(wsEvent.Data) == 0 {
		return wsEvent, errors.New("ooo: decode error, empty data")
	}
	if err != nil {
		return wsEvent, err
	}

	return wsEvent, nil
}

// Decode message
func DecodeReader(r io.Reader) (json.RawMessage, error) {
	var httpEvent json.RawMessage
	decoder := json.NewDecoder(r)
	err := decoder.Decode(&httpEvent)
	if err != nil {
		return httpEvent, err
	}
	if len(httpEvent) == 0 {
		return httpEvent, errors.New("ooo: decode error, empty data")
	}

	return httpEvent, nil
}

func PatchCache(data []byte, cache json.RawMessage) (json.RawMessage, error) {
	message, err := DecodeBuffer(data)
	if err != nil {
		return cache, err
	}

	if message.Snapshot {
		cache = message.Data
		return cache, nil
	}
	if string(message.Data) == "[]" {
		return cache, nil
	}

	patch, err := jsonpatch.DecodePatch([]byte(message.Data))
	if err != nil || patch == nil {
		log.Println("PatchCache: failed to decode patch", err)
		return cache, err
	}
	modifiedBytes, err := patch.Apply([]byte(cache))
	if err != nil || modifiedBytes == nil {
		log.Println("PatchCache: failed to apply patch", err)
		return cache, err
	}

	return modifiedBytes, nil
}

func Patch(data []byte, cache json.RawMessage) (json.RawMessage, meta.Object, error) {
	cache, err := PatchCache(data, cache)
	if err != nil {
		return cache, meta.Object{}, err
	}

	result, err := meta.Decode([]byte(cache))
	if err != nil {
		return cache, result, err
	}

	return cache, result, nil
}

func PatchList(data []byte, cache json.RawMessage) (json.RawMessage, []meta.Object, error) {
	cache, err := PatchCache(data, cache)
	if err != nil {
		return cache, []meta.Object{}, err
	}

	result, err := meta.DecodeList([]byte(cache))
	if err != nil {
		return cache, result, err
	}

	return cache, result, nil
}
