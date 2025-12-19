package client

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/benitogf/ooo/key"
	"github.com/benitogf/ooo/messages"
	"github.com/benitogf/ooo/meta"
	"github.com/gorilla/websocket"
)

var DefaultHandshakeTimeout = time.Second * 2

type Meta[T any] struct {
	Created int64  `json:"created"`
	Updated int64  `json:"updated"`
	Index   string `json:"index"`
	Data    T      `json:"data"`
}

type OnMessageCallback[T any] func([]Meta[T])
type OnErrorCallback func(error)
type OnOpenCallback func()
type OnCloseCallback func()

// SubscribeConfig holds configuration for the Subscribe function.
// Required fields: Ctx, Protocol, Host, Path, OnMessage.
// Optional fields: Header, OnError, OnOpen, OnClose, HandshakeTimeout.
type SubscribeConfig[T any] struct {
	Ctx              context.Context
	Protocol         string
	Host             string
	Path             string
	Header           http.Header
	OnMessage        OnMessageCallback[T]
	OnError          OnErrorCallback
	OnOpen           OnOpenCallback
	OnClose          OnCloseCallback
	HandshakeTimeout time.Duration
}

// Validate checks that required fields are set and applies defaults.
func (c *SubscribeConfig[T]) Validate() error {
	if c.Ctx == nil {
		return errors.New("client: Ctx is required")
	}
	if c.Protocol == "" {
		return errors.New("client: Protocol is required")
	}
	if c.Host == "" {
		return errors.New("client: Host is required")
	}
	if c.Path == "" {
		return errors.New("client: Path is required")
	}
	if c.OnMessage == nil {
		return errors.New("client: OnMessage callback is required")
	}
	if c.HandshakeTimeout == 0 {
		c.HandshakeTimeout = DefaultHandshakeTimeout
	}
	return nil
}

func Subscribe[T any](cfg SubscribeConfig[T]) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	retryCount := 0
	var cache json.RawMessage
	lastPath := key.LastIndex(cfg.Path)
	isList := lastPath == "*"
	closingTime := atomic.Bool{}
	wsURL := url.URL{Scheme: cfg.Protocol, Host: cfg.Host, Path: cfg.Path}
	muWsClient := sync.Mutex{}
	var wsClient *websocket.Conn

	go func(ct *atomic.Bool) {
		<-cfg.Ctx.Done()
		ct.Swap(true)
		muWsClient.Lock()
		defer muWsClient.Unlock()
		if wsClient == nil {
			log.Println("subscribe["+cfg.Host+"/"+cfg.Path+"]: client closing but no connection to close", cfg.Host, cfg.Path, cfg.Ctx.Err())
			return
		}
		log.Println("subscribe["+cfg.Host+"/"+cfg.Path+"]: client closing", cfg.Host, cfg.Path, cfg.Ctx.Err())
		wsClient.Close()
		if cfg.OnClose != nil {
			cfg.OnClose()
		}
	}(&closingTime)

	for {
		var err error
		quickDial := &websocket.Dialer{
			Proxy:            http.ProxyFromEnvironment,
			HandshakeTimeout: cfg.HandshakeTimeout,
		}

		muWsClient.Lock()
		wsClient, _, err = quickDial.Dial(wsURL.String(), cfg.Header)
		if wsClient == nil || err != nil {
			muWsClient.Unlock()
			log.Println("subscribe["+cfg.Host+"/"+cfg.Path+"]: failed websocket dial ", err)
			if cfg.OnError != nil {
				cfg.OnError(err)
			}
			time.Sleep(2 * time.Second)
			continue
		}
		muWsClient.Unlock()
		log.Println("subscribe["+cfg.Host+"/"+cfg.Path+"]: client connection stablished", cfg.Host, cfg.Path)
		if cfg.OnOpen != nil {
			cfg.OnOpen()
		}

		for {
			_, message, err := wsClient.ReadMessage()
			if err != nil || message == nil {
				log.Println("subscribe["+cfg.Host+"/"+cfg.Path+"]: failed websocket read connection ", err)
				if cfg.OnError != nil {
					cfg.OnError(err)
				}
				wsClient.Close()
				break
			}

			result := []Meta[T]{}
			if isList {
				var objs []meta.Object
				// log.Println("subscribe[", cfg.Host, cfg.Path, "]: message", string(message))
				cache, objs, err = messages.PatchList(message, cache)
				if err != nil {
					log.Println("subscribe["+cfg.Host+"/"+cfg.Path+"]: failed to parse message from websocket", err)
					if cfg.OnError != nil {
						cfg.OnError(err)
					}
					break
				}
				for _, obj := range objs {
					var item T
					err = json.Unmarshal([]byte(obj.Data), &item)
					if err != nil {
						log.Println("subscribe["+cfg.Host+"/"+cfg.Path+"]: failed to unmarshal data from websocket", err)
						if cfg.OnError != nil {
							cfg.OnError(err)
						}
						continue
					}
					result = append(result, Meta[T]{
						Created: obj.Created,
						Updated: obj.Updated,
						Index:   obj.Index,
						Data:    item,
					})
				}
				retryCount = 0
				cfg.OnMessage(result)
				continue
			}

			var obj meta.Object
			cache, obj, err = messages.Patch(message, cache)
			if err != nil {
				log.Println("subscribe["+cfg.Host+"/"+cfg.Path+"]: failed to parse message from websocket", err)
				if cfg.OnError != nil {
					cfg.OnError(err)
				}
				break
			}

			var item T
			err = json.Unmarshal([]byte(obj.Data), &item)
			if err != nil {
				log.Println("subscribe["+cfg.Host+"/"+cfg.Path+"]: failed to unmarshal data from websocket", err)
				if cfg.OnError != nil {
					cfg.OnError(err)
				}
				break
			}
			result = append(result, Meta[T]{
				Created: obj.Created,
				Updated: obj.Updated,
				Index:   obj.Index,
				Data:    item,
			})
			retryCount = 0
			cfg.OnMessage(result)
		}

		bye := closingTime.Load()
		if bye {
			log.Println("subscribe["+cfg.Host+"/"+cfg.Path+"]: skip reconnection, client closing...", cfg.Host, cfg.Path)
			break
		}

		if cfg.OnClose != nil {
			cfg.OnClose()
		}

		retryCount++
		if retryCount < 30 {
			log.Println("subscribe["+cfg.Host+"/"+cfg.Path+"]: reconnecting...", cfg.Host, cfg.Path, err)
			time.Sleep(300 * time.Millisecond)
			continue
		}

		if retryCount < 100 {
			log.Println("subscribe["+cfg.Host+"/"+cfg.Path+"]: reconnecting in 2 seconds...", cfg.Host, cfg.Path, err)
			time.Sleep(2 * time.Second)
			continue
		}

		log.Println("subscribe["+cfg.Host+"/"+cfg.Path+"]: reconnecting in 10 seconds...", err)
		time.Sleep(10 * time.Second)
	}
	return nil
}
