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

// SubscribeConfig holds connection configuration for Subscribe.
// Required fields: Ctx, Protocol, Host.
// Optional fields: Header, HandshakeTimeout.
type SubscribeConfig struct {
	Ctx              context.Context
	Protocol         string
	Host             string
	Header           http.Header
	HandshakeTimeout time.Duration
}

// Validate checks that required fields are set and applies defaults.
func (c *SubscribeConfig) Validate() error {
	if c.Ctx == nil {
		return errors.New("client: Ctx is required")
	}
	if c.Protocol == "" {
		return errors.New("client: Protocol is required")
	}
	if c.Host == "" {
		return errors.New("client: Host is required")
	}
	if c.HandshakeTimeout == 0 {
		c.HandshakeTimeout = DefaultHandshakeTimeout
	}
	return nil
}

// SubscribeEvents holds event callbacks for Subscribe.
// Required: OnMessage.
// Optional: OnError, OnOpen, OnClose.
type SubscribeEvents[T any] struct {
	OnMessage func([]Meta[T])
	OnError   func(error)
	OnOpen    func()
	OnClose   func()
}

// Validate checks that required callbacks are set.
func (e *SubscribeEvents[T]) Validate() error {
	if e.OnMessage == nil {
		return errors.New("client: OnMessage callback is required")
	}
	return nil
}

func Subscribe[T any](cfg SubscribeConfig, path string, events SubscribeEvents[T]) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if path == "" {
		return errors.New("client: path is required")
	}
	if err := events.Validate(); err != nil {
		return err
	}
	retryCount := 0
	var cache json.RawMessage
	lastPath := key.LastIndex(path)
	isList := lastPath == "*"
	closingTime := atomic.Bool{}
	wsURL := url.URL{Scheme: cfg.Protocol, Host: cfg.Host, Path: path}
	muWsClient := sync.Mutex{}
	var wsClient *websocket.Conn

	go func(ct *atomic.Bool) {
		<-cfg.Ctx.Done()
		ct.Swap(true)
		muWsClient.Lock()
		defer muWsClient.Unlock()
		if wsClient == nil {
			log.Println("subscribe["+cfg.Host+"/"+path+"]: client closing but no connection to close", cfg.Host, path, cfg.Ctx.Err())
			return
		}
		log.Println("subscribe["+cfg.Host+"/"+path+"]: client closing", cfg.Host, path, cfg.Ctx.Err())
		wsClient.Close()
		if events.OnClose != nil {
			events.OnClose()
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
			log.Println("subscribe["+cfg.Host+"/"+path+"]: failed websocket dial ", err)
			if events.OnError != nil {
				events.OnError(err)
			}
			time.Sleep(2 * time.Second)
			continue
		}
		muWsClient.Unlock()
		log.Println("subscribe["+cfg.Host+"/"+path+"]: client connection stablished", cfg.Host, path)
		if events.OnOpen != nil {
			events.OnOpen()
		}

		for {
			_, message, err := wsClient.ReadMessage()
			if err != nil || message == nil {
				log.Println("subscribe["+cfg.Host+"/"+path+"]: failed websocket read connection ", err)
				if events.OnError != nil {
					events.OnError(err)
				}
				wsClient.Close()
				break
			}

			result := []Meta[T]{}
			if isList {
				var objs []meta.Object
				// log.Println("subscribe[", cfg.Host, path, "]: message", string(message))
				cache, objs, err = messages.PatchList(message, cache)
				if err != nil {
					log.Println("subscribe["+cfg.Host+"/"+path+"]: failed to parse message from websocket", err)
					if events.OnError != nil {
						events.OnError(err)
					}
					break
				}
				for _, obj := range objs {
					var item T
					err = json.Unmarshal([]byte(obj.Data), &item)
					if err != nil {
						log.Println("subscribe["+cfg.Host+"/"+path+"]: failed to unmarshal data from websocket", err)
						if events.OnError != nil {
							events.OnError(err)
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
				events.OnMessage(result)
				continue
			}

			var obj meta.Object
			cache, obj, err = messages.Patch(message, cache)
			if err != nil {
				log.Println("subscribe["+cfg.Host+"/"+path+"]: failed to parse message from websocket", err)
				if events.OnError != nil {
					events.OnError(err)
				}
				break
			}

			var item T
			err = json.Unmarshal([]byte(obj.Data), &item)
			if err != nil {
				log.Println("subscribe["+cfg.Host+"/"+path+"]: failed to unmarshal data from websocket", err)
				if events.OnError != nil {
					events.OnError(err)
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
			events.OnMessage(result)
		}

		bye := closingTime.Load()
		if bye {
			log.Println("subscribe["+cfg.Host+"/"+path+"]: skip reconnection, client closing...", cfg.Host, path)
			break
		}

		if events.OnClose != nil {
			events.OnClose()
		}

		retryCount++
		if retryCount < 30 {
			log.Println("subscribe["+cfg.Host+"/"+path+"]: reconnecting...", cfg.Host, path, err)
			time.Sleep(300 * time.Millisecond)
			continue
		}

		if retryCount < 100 {
			log.Println("subscribe["+cfg.Host+"/"+path+"]: reconnecting in 2 seconds...", cfg.Host, path, err)
			time.Sleep(2 * time.Second)
			continue
		}

		log.Println("subscribe["+cfg.Host+"/"+path+"]: reconnecting in 10 seconds...", err)
		time.Sleep(10 * time.Second)
	}
	return nil
}
