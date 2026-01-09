package messages

import (
	"errors"
	"io"
	"sync"

	"github.com/benitogf/jsonpatch"
	"github.com/benitogf/ooo/meta"
	"github.com/goccy/go-json"
)

var (
	ErrDecodeEmptyData = errors.New("messages: decode error, empty data")
)

// Message sent through websocket connections
type Message struct {
	Data     json.RawMessage `json:"data"`
	Version  string          `json:"version"`
	Snapshot bool            `json:"snapshot"`
}

// messagePool reduces allocations for Message structs in hot paths.
var messagePool = sync.Pool{
	New: func() any {
		return new(Message)
	},
}

// getMessage gets a Message from the pool.
func getMessage() *Message {
	return messagePool.Get().(*Message)
}

// putMessage returns a Message to the pool after resetting it.
func putMessage(m *Message) {
	m.Data = nil
	m.Version = ""
	m.Snapshot = false
	messagePool.Put(m)
}

// DecodeBuffer decodes a WebSocket message from a byte buffer.
// Returns an error if the JSON is invalid or the data field is empty.
func DecodeBuffer(data []byte) (Message, error) {
	var wsEvent Message
	err := json.Unmarshal(data, &wsEvent)
	if err != nil {
		return wsEvent, err
	}
	if len(wsEvent.Data) == 0 {
		return wsEvent, ErrDecodeEmptyData
	}

	return wsEvent, nil
}

// DecodeBufferPooled decodes a WebSocket message using a pooled Message struct.
// The caller must call ReleaseMessage when done with the result.
// Returns nil on error.
func DecodeBufferPooled(data []byte) (*Message, error) {
	msg := getMessage()
	err := json.Unmarshal(data, msg)
	if err != nil {
		putMessage(msg)
		return nil, err
	}
	if len(msg.Data) == 0 {
		putMessage(msg)
		return nil, ErrDecodeEmptyData
	}
	return msg, nil
}

// ReleaseMessage returns a Message to the pool. Call this when done with a pooled message.
func ReleaseMessage(m *Message) {
	if m != nil {
		putMessage(m)
	}
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
		return httpEvent, ErrDecodeEmptyData
	}

	return httpEvent, nil
}

// PatchCache applies a message to the cache. If the message is a snapshot,
// it replaces the cache entirely. Otherwise, it applies the JSON patch.
// Returns the updated cache.
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
		return cache, err
	}
	modifiedBytes, err := patch.Apply([]byte(cache))
	if err != nil || modifiedBytes == nil {
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
