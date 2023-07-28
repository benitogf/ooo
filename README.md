# ooo

[![Test](https://github.com/benitogf/ooo/actions/workflows/tests.yml/badge.svg?branch=master)](https://github.com/benitogf/ooo/actions/workflows/tests.yml)

Zero configuration data persistence and communication layer.

Dynamic websocket and restful http service to quickly prototype realtime applications.

## features

- dynamic routing
- glob pattern routes for lists
- [patch](http://jsonpatch.com) updates on lists subscriptions
- version check on subscriptions (no message on version match)
- restful CRUD service that reflects interactions to real-time subscriptions
- filtering and audit middleware
- auto managed timestamps (created, updated)

## quickstart

### client

There's a [js client](https://www.npmjs.com/package/ooo-client).

### server

with [go installed](https://golang.org/doc/install) get the library

```bash
go get github.com/benitogf/ooo
```

create a file `main.go`
```golang
package main

import "github.com/benitogf/ooo"

func main() {
  app := ooo.Server{}
  app.Start("0.0.0.0:8800")
  app.WaitClose()
}
```

run the service:
```bash
go run main.go
```

# routes

| method | description | url    |
| ------------- |:-------------:| -----:|
| GET | key list | http://{host}:{port} |
| websocket| clock | ws://{host}:{port} |
| POST | create/update | http://{host}:{port}/{key} |
| GET | read | http://{host}:{port}/{key} |
| DELETE | delete | http://{host}:{port}/{key} |
| websocket| subscribe | ws://{host}:{port}/{key} |


# control

### static routes

Activating this flag will limit the server to process requests defined in read and write filters

```golang
app := ooo.Server{}
app.Static = true
```


### filters

- Write filters will be called before processing a write operation
- Read filters will be called before sending the results of a read operation
- if the static flag is enabled only filtered routes will be available

```golang
app.WriteFilter("books/*", func(index string, data json.RawMessage) (json.RawMessage, error) {
  // returning an error will deny the write
  return data, nil
})
app.AfterWrite("books/*", func(index string) {
  // trigger after a write is done
  log.Println("wrote data on ", index)
})
app.ReadFilter("books/taup", func(index string, data json.RawMessage) (json.RawMessage, error) {
  // returning an error will deny the read
  return json.RawMessage(`{"intercepted":true}`), nil
})
app.DeleteFilter("books/taup", func(key string) (error) {
  // returning an error will prevent the delete
  return errors.New("can't delete")
})
```

### audit

```golang
app.Audit = func(r *http.Request) bool {
  return false // condition to allow access to the resource
}
```

### subscribe events capture

```golang
// new subscription event
server.OnSubscribe = func(key string) error {
  log.Println(key)
  // returning an error will deny the subscription
  return nil
}
// closing subscription event
server.OnUnsubscribe = func(key string) {
  log.Println(key)
}
```

### extra routes

```golang
// Define custom endpoints
app.Router = mux.NewRouter()
app.Router.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Content-Type", "application/json")
  fmt.Fprintf(w, "{}")
})
app.Start("0.0.0.0:8800")
```


### write/read storage api

```golang
package main

import (
	"encoding/json"
	"log"
	"strconv"
	"time"

	"github.com/benitogf/ooo"
	"github.com/benitogf/ooo/meta"
)

type Game struct {
	Started int64 `json:"started"`
}

// not good practice, just for illustration purposes only
// handle errors responsably :)
func panicHandle(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	// create a static server
	server := ooo.Server{
		Static: true,
	}
	// define the path so it's available throug http/ws
	server.OpenFilter("game")

	// start the server/storage (default to memory storage)
	server.Start("0.0.0.0:8800")

	// write
	timestamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	index, err := server.Storage.Set("game", json.RawMessage(`{"started": `+timestamp+`}`))
	panicHandle(err)
	log.Println("stored in", index)

	// read
	data, err := server.Storage.Get("game")
	panicHandle(err)
	dataObject, err := meta.Decode(data)
	panicHandle(err)
	log.Println("created", dataObject.Created)
	log.Println("updated", dataObject.Updated)
	log.Println("data", string(dataObject.Data))

	// parse json to struct
	game := Game{}
	err = json.Unmarshal(dataObject.Data, &game)
	panicHandle(err)
	log.Println("started", game.Started)

	// close server handler
	server.WaitClose()
}
```