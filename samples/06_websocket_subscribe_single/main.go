//go:build ignore
// +build ignore

// Code generated for documentation purposes. DO NOT EDIT.
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/benitogf/ooo"
	"github.com/benitogf/ooo/client"
)

type Todo struct {
	Task      string    `json:"task"`
	Completed bool      `json:"completed"`
	Due       time.Time `json:"due"`
}

func main() {
	// Start server (for demo)
	server := &ooo.Server{Silence: true}
	server.Start("localhost:0")
	defer server.Close(nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := client.SubscribeConfig{
		Ctx:    ctx,
		Server: client.Server{Protocol: "ws", Host: server.Address},
	}

	// Ensure path exists
	ooo.Set(server, "todo", Todo{Task: "one", Due: time.Now().Add(24 * time.Hour)})

	go client.Subscribe[Todo](cfg, "todo", client.SubscribeEvents[Todo]{
		OnMessage: func(item client.Meta[Todo]) {
			fmt.Println("current todo:", item.Data.Task)
		},
		OnError: func(err error) {
			fmt.Println("connection error:", err)
		},
	})

	// Update the item to trigger a message
	time.Sleep(50 * time.Millisecond)
	ooo.Set(server, "todo", Todo{Task: "updated", Due: time.Now().Add(48 * time.Hour)})

	time.Sleep(300 * time.Millisecond)
}
