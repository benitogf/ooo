# ooo

[![Test](https://github.com/benitogf/ooo/actions/workflows/tests.yml/badge.svg?branch=master)](https://github.com/benitogf/ooo/actions/workflows/tests.yml)

![ooo](ooo.jpg)

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
  app.Start("localhost:8800")
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
app.Start("localhost:8800")
```