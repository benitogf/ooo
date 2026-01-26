//go:build ignore

// Remote server that holds the actual data.
// This is the source of truth that the proxy will forward requests to.
package main

import (
	"log"

	"github.com/benitogf/ooo"
)

func main() {
	server := &ooo.Server{}

	// Seed some initial data
	server.Start("0.0.0.0:8800")

	// Add initial settings
	ooo.Set(server, "settings", map[string]any{
		"theme":    "dark",
		"language": "en",
	})

	// Add some items
	ooo.Push(server, "items/*", map[string]any{"name": "Item 1", "value": 100})
	ooo.Push(server, "items/*", map[string]any{"name": "Item 2", "value": 200})

	log.Println("Remote server running on :8800")
	log.Println("This server holds the actual data")
	server.WaitClose()
}
