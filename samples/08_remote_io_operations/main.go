// Package main demonstrates remote I/O operations.
// This example shows how to:
// - Use RemoteConfig to connect to a remote ooo server
// - Use RemoteSet to create/update items
// - Use RemoteGet to retrieve items
// - Use RemotePush to add items to lists
// - Use RemoteGetList to retrieve lists
// - Use RemoteDelete to delete items
package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/benitogf/ooo"
	"github.com/benitogf/ooo/io"
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

	// Create a remote config
	cfg := io.RemoteConfig{
		Client: &http.Client{Timeout: 10 * time.Second},
		Host:   server.Address,
		SSL:    false, // set to true for HTTPS
	}

	// RemoteSet - create/update a single item
	err := io.RemoteSet(cfg, "config", map[string]string{"theme": "dark"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Set config successfully")

	// RemoteGet - retrieve a single item
	config, err := io.RemoteGet[map[string]string](cfg, "config")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Got config: %+v\n", config.Data)

	// RemotePush - add items to a list
	err = io.RemotePush(cfg, "todos/*", Todo{
		Task:      "First task",
		Completed: false,
		Due:       time.Now().Add(24 * time.Hour),
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Pushed first todo")

	err = io.RemotePush(cfg, "todos/*", Todo{
		Task:      "Second task",
		Completed: false,
		Due:       time.Now().Add(48 * time.Hour),
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Pushed second todo")

	// RemoteGetList - retrieve all items from a list
	todos, err := io.RemoteGetList[Todo](cfg, "todos/*")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Got %d todos:\n", len(todos))
	for i, todo := range todos {
		fmt.Printf("  %d. %s (due: %v)\n", i+1, todo.Data.Task, todo.Data.Due)
	}

	// RemoteDelete - delete an item
	err = io.RemoteDelete(cfg, "config")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Deleted config")
}
