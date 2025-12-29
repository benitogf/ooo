//go:build ignore
// +build ignore

// Code generated for documentation purposes. DO NOT EDIT.
package main

import (
	"encoding/json"
	"log"
	"strconv"

	"github.com/benitogf/ooo"
	"github.com/benitogf/ooo/monotonic"
)

type Game struct {
	Started int64 `json:"started"`
}

func main() {
	// Initialize the monotonic clock for consistent timestamps
	// This clock is resilient to NTP/system clock adjustments
	monotonic.Init()
	defer monotonic.Stop()

	// Create a static server - only filtered routes are accessible
	server := ooo.Server{Static: true}

	// Define the path so it's available through HTTP/WebSocket
	server.OpenFilter("game")

	// Start the server with default memory storage
	server.Start("0.0.0.0:8800")

	// Write data using monotonic timestamp for ordering consistency
	// monotonic.Now() returns nanoseconds that always increase
	timestamp := strconv.FormatInt(monotonic.Now(), 10)
	index, err := server.Storage.Set("game", json.RawMessage(`{"started": `+timestamp+`}`))
	if err != nil {
		log.Fatal(err)
	}
	log.Println("stored in", index)

	// Read data back from storage
	dataObject, err := server.Storage.Get("game")
	if err != nil {
		log.Fatal(err)
	}
	log.Println("created:", dataObject.Created)
	log.Println("updated:", dataObject.Updated)
	log.Println("data:", string(dataObject.Data))

	// Parse JSON to struct
	game := Game{}
	err = json.Unmarshal(dataObject.Data, &game)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("started:", game.Started)

	server.WaitClose()
}
