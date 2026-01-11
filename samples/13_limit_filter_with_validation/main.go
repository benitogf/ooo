// Package main demonstrates LimitFilter with custom write validation.
// This example shows how to:
// - Create a limit filter that also validates incoming data
// - Reject writes that don't match the expected struct fields exactly
// - Combine LimitFilter behavior with strict schema validation
package main

import (
	"encoding/json"
	"errors"
	"log"
	"strings"

	"github.com/benitogf/ooo"
)

// LogEntry defines the exact schema for log entries
type LogEntry struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

func main() {
	server := ooo.Server{Static: true}
	server.Start("0.0.0.0:8800")

	// Add write validation that enforces strict schema
	server.WriteFilter("logs/*", func(index string, data json.RawMessage) (json.RawMessage, error) {
		// Use DisallowUnknownFields to reject extra fields
		decoder := json.NewDecoder(strings.NewReader(string(data)))
		decoder.DisallowUnknownFields()

		var entry LogEntry
		if err := decoder.Decode(&entry); err != nil {
			return nil, errors.New("invalid schema: " + err.Error())
		}

		// Validate required fields
		if entry.Level == "" {
			return nil, errors.New("level is required")
		}
		if entry.Message == "" {
			return nil, errors.New("message is required")
		}

		// Validate level values
		validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
		if !validLevels[entry.Level] {
			return nil, errors.New("level must be one of: debug, info, warn, error")
		}

		return data, nil
	})

	// Add limit filter to cap entries
	server.LimitFilter("logs/*", 100)

	log.Println("Server running with validated limit filter")
	log.Println("POST /logs/* - validates schema and caps at 100 entries")
	log.Println("")
	log.Println("Valid request:")
	log.Println(`  curl -X POST http://localhost:8800/logs/1 -d '{"level":"info","message":"test"}'`)
	log.Println("")
	log.Println("Invalid requests (will be rejected):")
	log.Println(`  curl -X POST http://localhost:8800/logs/1 -d '{"level":"info"}'  # missing message`)
	log.Println(`  curl -X POST http://localhost:8800/logs/1 -d '{"level":"info","message":"test","extra":"field"}'  # extra field`)
	server.WaitClose()
}
