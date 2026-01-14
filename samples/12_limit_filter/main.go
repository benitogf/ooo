// Package main demonstrates the LimitFilter for capped collections.
// This example shows how to:
// - Use LimitFilter to maintain a max number of entries
// - Oldest entries are automatically deleted when limit is exceeded
// - Useful for logs, activity feeds, or any rolling window data
package main

import (
	"log"

	"github.com/benitogf/ooo"
	"github.com/benitogf/ooo/filters"
)

func main() {
	server := ooo.Server{Static: true}
	server.Start("0.0.0.0:8800")

	// Keep only the 100 most recent log entries
	// When a new entry is added and count > 100, oldest is deleted
	server.LimitFilter("logs/*", filters.LimitFilterConfig{Limit: 100})

	// Keep only 50 most recent notifications per user
	server.LimitFilter("notifications/*", filters.LimitFilterConfig{Limit: 50})

	log.Println("Server running with limit filters")
	log.Println("POST /logs/* - capped at 100 entries")
	log.Println("POST /notifications/* - capped at 50 entries")
	server.WaitClose()
}
