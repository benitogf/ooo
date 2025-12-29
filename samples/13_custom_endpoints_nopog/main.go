//go:build ignore
// +build ignore

// Code generated for documentation purposes. DO NOT EDIT.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/benitogf/nopog"
	"github.com/benitogf/ooo"
	"github.com/benitogf/ooo/key"
	"github.com/gorilla/mux"
)

// Product represents an item in the catalog
type Product struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Price    int    `json:"price"`
}

func main() {
	// PostgreSQL storage for large-scale persistent data
	db := &nopog.Storage{
		Name:     "mydb",
		Host:     "localhost",
		User:     "postgres",
		Password: "postgres",
	}
	if err := db.Start(); err != nil {
		log.Fatal("Failed to connect:", err)
	}
	defer db.Close()

	// Create table (idempotent)
	if err := db.CreateTable("catalog"); err != nil {
		log.Fatal(err)
	}

	server := ooo.Server{Static: true}

	// GET /api/products - List all products
	// Uses single glob pattern (fast prefix matching)
	server.Router.HandleFunc("/api/products", func(w http.ResponseWriter, r *http.Request) {
		results, err := db.Get("catalog", "products/*")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		json.NewEncoder(w).Encode(results)
	}).Methods("GET")

	// GET /api/products/search?category=electronics
	// Uses JSONB field query for efficient filtering
	server.Router.HandleFunc("/api/products/search", func(w http.ResponseWriter, r *http.Request) {
		category := r.URL.Query().Get("category")
		results, err := db.GetByField("catalog", "products/*", "category", category)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		json.NewEncoder(w).Encode(results)
	}).Methods("GET")

	// GET /api/products/recent?from=1234567890&to=1234567899&limit=10
	// Range query by timestamp (microseconds)
	server.Router.HandleFunc("/api/products/recent", func(w http.ResponseWriter, r *http.Request) {
		from, _ := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
		to, _ := strconv.ParseInt(r.URL.Query().Get("to"), 10, 64)
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit == 0 {
			limit = 10
		}
		results, err := db.GetNRange("catalog", "products/*", from, to, limit)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		json.NewEncoder(w).Encode(results)
	}).Methods("GET")

	// POST /api/products - Create a product
	// Uses key.Build to generate a UUID for the glob pattern (safe key)
	server.Router.HandleFunc("/api/products", func(w http.ResponseWriter, r *http.Request) {
		var p Product
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		data, _ := json.Marshal(p)
		// key.Build generates a UUID - safe for glob patterns regardless of user input
		productKey := key.Build("products/*")
		ts, err := db.Set("catalog", productKey, string(data))
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"created": ts, "key": productKey})
	}).Methods("POST")

	// DELETE /api/products/{id} - Delete by UUID
	server.Router.HandleFunc("/api/products/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := mux.Vars(r)["id"]
		if err := db.Del("catalog", "products/"+id); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}).Methods("DELETE")

	log.Println("Server running on :8800")
	server.Start("0.0.0.0:8800")
	server.WaitClose()
}
