// Package main demonstrates custom HTTP endpoints with server.Endpoint().
// This example shows how to:
// - Register custom endpoints with typed request/response schemas
// - Endpoints appear in the storage explorer UI with documentation
// - Use Vars for route variables like {id} (mandatory)
// - Use Params for query parameters like ?name=x (optional)
// - Support multiple HTTP methods on the same endpoint
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/benitogf/ooo"
	"github.com/gorilla/mux"
)

// Policy represents an access control policy
type Policy struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

// PolicyResponse is returned when getting a policy
type PolicyResponse struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

// policies stores our in-memory data
var (
	policies   = make(map[string]Policy)
	policiesMu sync.RWMutex
)

func main() {
	server := ooo.Server{
		Static: true,
		Name:   "Custom Endpoints Demo",
	}

	// Register a custom endpoint for policies
	// The endpoint metadata is visible in the storage explorer UI
	server.Endpoint(ooo.EndpointConfig{
		Path:        "/policies/{id}",
		Description: "Manage access control policies",
		Methods: ooo.Methods{
			"GET": ooo.MethodSpec{
				Response: PolicyResponse{},
			},
			"PUT": ooo.MethodSpec{
				Request:  Policy{},
				Response: PolicyResponse{},
			},
			"DELETE": ooo.MethodSpec{},
		},
		Handler: func(w http.ResponseWriter, r *http.Request) {
			id := mux.Vars(r)["id"]

			switch r.Method {
			case "GET":
				policiesMu.RLock()
				p, ok := policies[id]
				policiesMu.RUnlock()
				if !ok {
					http.Error(w, "policy not found", http.StatusNotFound)
					return
				}
				json.NewEncoder(w).Encode(PolicyResponse{
					ID:          id,
					Name:        p.Name,
					Description: p.Description,
					Permissions: p.Permissions,
				})

			case "PUT":
				var p Policy
				if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				policiesMu.Lock()
				policies[id] = p
				policiesMu.Unlock()
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(PolicyResponse{
					ID:          id,
					Name:        p.Name,
					Description: p.Description,
					Permissions: p.Permissions,
				})

			case "DELETE":
				policiesMu.Lock()
				delete(policies, id)
				policiesMu.Unlock()
				w.WriteHeader(http.StatusNoContent)
			}
		},
	})

	// Register a list endpoint with optional query parameter filtering
	// Params are per-method query parameters shown in the UI
	server.Endpoint(ooo.EndpointConfig{
		Path:        "/policies",
		Description: "List and search policies",
		Methods: ooo.Methods{
			"GET": ooo.MethodSpec{
				Response: []PolicyResponse{},
				Params: ooo.Params{
					"name": "Filter policies by name (partial match)",
				},
			},
		},
		Handler: func(w http.ResponseWriter, r *http.Request) {
			nameFilter := r.URL.Query().Get("name")

			policiesMu.RLock()
			result := make([]PolicyResponse, 0, len(policies))
			for id, p := range policies {
				// Apply optional name filter
				if nameFilter != "" && !strings.Contains(strings.ToLower(p.Name), strings.ToLower(nameFilter)) {
					continue
				}
				result = append(result, PolicyResponse{
					ID:          id,
					Name:        p.Name,
					Description: p.Description,
					Permissions: p.Permissions,
				})
			}
			policiesMu.RUnlock()
			json.NewEncoder(w).Encode(result)
		},
	})

	server.Start("0.0.0.0:8800")
	log.Println("Server running with custom endpoints")
	log.Println("")
	log.Println("Try these commands:")
	log.Println("  curl -X PUT http://localhost:8800/policies/admin -d '{\"name\":\"Admin\",\"description\":\"Full access\",\"permissions\":[\"read\",\"write\",\"delete\"]}'")
	log.Println("  curl http://localhost:8800/policies/admin")
	log.Println("  curl http://localhost:8800/policies")
	log.Println("  curl -X DELETE http://localhost:8800/policies/admin")
	log.Println("")
	log.Println("Visit http://localhost:8800 to see endpoints in the storage explorer")
	server.WaitClose()
}
