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

	// Seed one item
	ooo.Push(server, "todos/*", Todo{Task: "seed", Due: time.Now().Add(1 * time.Hour)})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := client.SubscribeConfig{
		Ctx:     ctx,
		Server:  client.Server{Protocol: "ws", Host: server.Address},
		Silence: true,
	}

	go client.SubscribeList[Todo](cfg, "todos/*", client.SubscribeListEvents[Todo]{
		OnMessage: func(items []client.Meta[Todo]) {
			fmt.Println("list size:", len(items))
			for i, it := range items {
				fmt.Printf("%d. %s (due: %v)\n", i+1, it.Data.Task, it.Data.Due)
			}
		},
		OnError: func(err error) {
			fmt.Println("connection error:", err)
		},
	})

	// Produce updates
	time.Sleep(50 * time.Millisecond)
	ooo.Push(server, "todos/*", Todo{Task: "another", Due: time.Now().Add(2 * time.Hour)})

	time.Sleep(300 * time.Millisecond)
}
