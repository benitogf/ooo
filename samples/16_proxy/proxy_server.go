//go:build ignore

// Proxy server that forwards requests to the remote server.
// Demonstrates path remapping - expose different paths on proxy than on source.
//
// Example: Client requests /settings/deviceA on proxy → forwarded to /settings on remote
package main

import (
	"log"
	"strings"

	"github.com/benitogf/ooo"
	"github.com/benitogf/ooo/proxy"
)

func main() {
	server := &ooo.Server{
		Static: true, // Only allow explicitly defined routes
	}

	// Settings route: /settings/{deviceID} → /settings
	// Each resolver is specific to its route for clarity
	settingsConfig := proxy.Config{
		Resolve: func(localPath string) (address, remotePath string, err error) {
			// localPath = "settings/deviceA" → remotePath = "settings"
			return "localhost:8800", "settings", nil
		},
	}

	// Items route: /items/{deviceID}/* → /items/*
	itemsConfig := proxy.Config{
		Resolve: func(localPath string) (address, remotePath string, err error) {
			// localPath = "items/deviceA/abc123" → remotePath = "items/abc123"
			parts := strings.SplitN(localPath, "/", 3)
			if len(parts) == 3 {
				return "localhost:8800", "items/" + parts[2], nil
			}
			return "localhost:8800", "items/*", nil
		},
	}

	// Route: /settings/{deviceID} → /settings on remote
	err := proxy.Route(server, "settings/*", settingsConfig)
	if err != nil {
		log.Fatal("Failed to setup settings route:", err)
	}

	// Route: /items/{deviceID}/* → /items/* on remote
	err = proxy.RouteList(server, "items/*/*", itemsConfig)
	if err != nil {
		log.Fatal("Failed to setup items route:", err)
	}

	server.Start("0.0.0.0:8801")
	log.Println("Proxy server running on :8801")
	log.Println("Forwarding requests to remote at localhost:8800")
	log.Println("")
	log.Println("Path remapping examples:")
	log.Println("  /settings/deviceA  → /settings (remote)")
	log.Println("  /items/deviceA/*   → /items/*  (remote)")
	log.Println("")
	log.Println("Try these commands:")
	log.Println("  curl http://localhost:8801/settings/mydevice")
	log.Println("  curl http://localhost:8801/items/mydevice/*")
	log.Println("  curl -X POST http://localhost:8801/items/mydevice/* -d '{\"name\":\"New Item\"}'")
	server.WaitClose()
}
