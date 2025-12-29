//go:build ignore
// +build ignore

// Code generated for documentation purposes. DO NOT EDIT.
package main

import (
	"context"
	"fmt"
	"sync"

	"github.com/benitogf/ooo"
	"github.com/benitogf/ooo/client"
)

type Product struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Price int    `json:"price"`
}

type Order struct {
	ID        int `json:"id"`
	ProductID int `json:"product_id"`
	Quantity  int `json:"quantity"`
}

func main() {
	// Start server for demo
	server := &ooo.Server{Silence: true}
	server.Start("localhost:0")
	defer server.Close(nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := client.SubscribeConfig{
		Ctx:     ctx,
		Server:  client.Server{Protocol: "ws", Host: server.Address},
		Silence: true,
	}

	// Use WaitGroup to synchronize async operations
	var wg sync.WaitGroup
	wg.Add(3) // Expect 3 updates

	go client.SubscribeMultipleList2(
		cfg,
		[2]string{"products/*", "orders/*"},
		client.SubscribeMultipleList2Events[Product, Order]{
			OnMessage: func(products client.MultiState[Product], orders client.MultiState[Order]) {
				// Called when either subscription updates
				// Use .Updated to check which one changed
				if products.Updated {
					fmt.Println("products updated:", len(products.Data))
					wg.Done()
				}
				if orders.Updated {
					fmt.Println("orders updated:", len(orders.Data))
					wg.Done()
				}
			},
			OnError: func(productsErr, ordersErr error) {
				if productsErr != nil {
					fmt.Println("products error:", productsErr)
				}
				if ordersErr != nil {
					fmt.Println("orders error:", ordersErr)
				}
			},
		},
	)

	// Create some data - updates trigger OnMessage callbacks
	ooo.Push(server, "products/*", Product{ID: 1, Name: "Widget", Price: 100})
	ooo.Push(server, "orders/*", Order{ID: 1, ProductID: 1, Quantity: 5})
	ooo.Push(server, "products/*", Product{ID: 2, Name: "Gadget", Price: 200})

	wg.Wait()
}
