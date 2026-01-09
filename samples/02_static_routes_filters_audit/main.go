// Package main demonstrates static routes, filters, and audit middleware.
// This example shows how to:
// - Enable static mode (only filtered routes available)
// - Add API key authentication via Audit
// - Use write filters for validation
// - Use read filters to hide sensitive data
// - Use delete filters to protect resources
package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/benitogf/ooo"
	"github.com/benitogf/ooo/meta"
)

type Book struct {
	Title  string `json:"title"`
	Author string `json:"author"`
	Secret string `json:"secret,omitempty"`
}

func main() {
	server := ooo.Server{Static: true}

	// Only allow requests that carry a valid API key
	server.Audit = func(r *http.Request) bool {
		return r.Header.Get("X-API-Key") == "secret"
	}

	// Make the list route available while Static mode is enabled
	server.OpenFilter("books/*")
	server.OpenFilter("books/locked") // single resource example

	// Sanitize/validate before writes to the list
	server.WriteFilter("books/*", func(index string, data json.RawMessage) (json.RawMessage, error) {
		var b Book
		err := json.Unmarshal(data, &b)
		if err != nil {
			return nil, err
		}
		if b.Title == "" {
			return nil, errors.New("title is required")
		}
		if b.Author == "" {
			b.Author = "unknown"
		}
		// Persist possibly modified payload
		out, _ := json.Marshal(b)
		return out, nil
	})

	// Log after any write
	server.AfterWriteFilter("books/*", func(index string) {
		log.Println("wrote book at", index)
	})

	// Hide secrets on reads (for lists)
	server.ReadListFilter("books/*", func(index string, objs []meta.Object) ([]meta.Object, error) {
		for i := range objs {
			var b Book
			json.Unmarshal(objs[i].Data, &b)
			b.Secret = ""
			objs[i].Data, _ = json.Marshal(b)
		}
		return objs, nil
	})

	// Prevent deleting a specific resource
	server.DeleteFilter("books/locked", func(key string) error {
		return errors.New("this book cannot be deleted")
	})

	server.Start("0.0.0.0:8800")
	server.WaitClose()
}
