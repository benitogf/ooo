// Package main demonstrates I/O operations with typed helpers.
// This example shows how to:
// - Use ooo.Push to add items to a list
// - Use ooo.GetList to retrieve all items
// - Use ooo.Set to create/update single items
// - Use ooo.Get to retrieve single items
package main

import (
	"fmt"
	"log"
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

	// Add some todos using Push (for lists)
	_, err := ooo.Push(server, "todos/*", Todo{
		Task:      "todo 1",
		Completed: false,
		Due:       time.Now().Add(24 * time.Hour),
	})
	if err != nil {
		log.Fatal(err)
	}

	_, err = ooo.Push(server, "todos/*", Todo{
		Task:      "todo 2",
		Completed: false,
		Due:       time.Now().Add(48 * time.Hour),
	})
	if err != nil {
		log.Fatal(err)
	}

	// Get all todos using GetList
	todos, err := ooo.GetList[Todo](server, "todos/*")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("All todos:")
	for i, todo := range todos {
		fmt.Printf("%d. %s (Due: %v)\n", i+1, todo.Data.Task, todo.Data.Due)
	}

	// Set a single item
	err = ooo.Set(server, "config", map[string]string{"theme": "dark"})
	if err != nil {
		log.Fatal(err)
	}

	// Get a single item
	config, err := ooo.Get[map[string]string](server, "config")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Config: %+v\n", config.Data)
}
