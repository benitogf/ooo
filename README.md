# ooo

[![Test](https://github.com/benitogf/ooo/actions/workflows/tests.yml/badge.svg)](https://github.com/benitogf/ooo/actions/workflows/tests.yml)

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


## I/O Operations

The `ooo` package provides functions for working with data through the OOO server. These functions handle JSON serialization/deserialization and provide a more convenient way to work with your data structures.

### Basic Operations

#### Get a Single Item

```go
// Get retrieves a single item from the specified path
item, err := ooo.Get[YourType](server, "path/to/item")
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Item: %+v\n", item.Data)
```

#### Get a List of Items

```go
// GetList retrieves all items from a list path (ends with "/*")
items, err := ooo.GetList[YourType](server, "path/to/items/*")
if err != nil {
    log.Fatal(err)
}
for _, item := range items {
    fmt.Printf("Item: %+v (created: %v)\n", item.Data, item.Created)
}
```

#### Set an Item

```go
// Set creates or updates an item at the specified path
err := ooo.Set(server, "path/to/item", YourType{
    Field1: "value1",
    Field2: "value2",
})
if err != nil {
    log.Fatal(err)
}
```

#### Add to a List

```go
// Push adds an item to a list (path must end with "/*")
err := ooo.Push(server, "path/to/items/*", YourType{
    Field1: "new item",
    Field2: "another value",
})
if err != nil {
    log.Fatal(err)
}
```

### Remote Operations

You can also perform operations on remote OOO servers using the client functions:

```go
// Create an HTTP client
client := &http.Client{Timeout: 10 * time.Second}

// RemoteGet fetches an item from a remote server
item, err := ooo.RemoteGet[YourType](
    client,
    false,  // useHTTPS
    "localhost:8800",  // host:port
    "path/to/item",
)

// RemoteSet updates or creates an item on a remote server
err = ooo.RemoteSet(
    client,
    false,  // useHTTPS
    "localhost:8800",
    "path/to/item",
    YourType{Field1: "value"},
)

// RemotePush adds an item to a list on a remote server
err = ooo.RemotePush(
    client,
    false,  // useHTTPS
    "localhost:8800",
    "path/to/items/*",
    YourType{Field1: "new item"},
)

// RemoteGetList fetches all items from a list on a remote server
items, err := ooo.RemoteGetList[YourType](
    client,
    false,  // useHTTPS
    "localhost:8800",
    "path/to/items/*",
)
```

### Complete Example

Here's a complete example demonstrating the usage of these functions:

```go
package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/benitogf/ooo"
)

type Todo struct {
	Task      string    `json:"task"`
	Completed bool      `json:"completed"`
	Due       time.Time `json:"due"`
}

func main() {
	// Start a local server for testing
	server := &ooo.Server{Silence: true}
	server.Start("localhost:0")
	defer server.Close(nil)

	// Add some todos
	err := ooo.Push(server, "todos/*", Todo{
		Task:      "todo 1",
		Completed: false,
		Due:       time.Now().Add(24 * time.Hour),
	})
	if err != nil {
		log.Fatal(err)
	}

	err = ooo.Push(server, "todos/*", Todo{
		Task:      "todo 2",
		Completed: false,
		Due:       time.Now().Add(48 * time.Hour),
	})
	if err != nil {
		log.Fatal(err)
	}

	// Get all todos
	todos, err := ooo.GetList[Todo](server, "todos/*")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("All todos:")
	for i, todo := range todos {
		fmt.Printf("%d. %s (Due: %v)\n", i+1, todo.Data.Task, todo.Data.Due)
	}
}
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
