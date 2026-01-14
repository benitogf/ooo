package main

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/benitogf/ooo"
	"github.com/benitogf/ooo/filters"
)

func main() {
	server := ooo.Server{}

	// Add filters similar to mono/router
	server.LimitFilter("logs/*", filters.LimitFilterConfig{Limit: 5})
	server.OpenFilter("items/*")
	server.OpenFilter("users/*")
	server.OpenFilter("config")
	server.OpenFilter("statistics/*/*/*")

	server.Start("0.0.0.0:8800")

	// Seed some demo data
	seedData(&server)

	server.WaitClose()
}

func seedData(server *ooo.Server) {
	// Wait for server to be ready
	time.Sleep(100 * time.Millisecond)

	// Add some items
	for i := 1; i <= 5; i++ {
		key := "items/" + strconv.Itoa(i)
		data := map[string]interface{}{
			"name":        "Item " + strconv.Itoa(i),
			"description": "Description for item " + strconv.Itoa(i),
			"price":       float64(i) * 10.5,
			"active":      i%2 == 0,
		}
		jsonData, _ := json.Marshal(data)
		server.Storage.Set(key, json.RawMessage(jsonData))
	}

	// Add some users
	users := []map[string]interface{}{
		{"username": "alice", "email": "alice@example.com", "role": "admin"},
		{"username": "bob", "email": "bob@example.com", "role": "user"},
		{"username": "charlie", "email": "charlie@example.com", "role": "user"},
	}
	for i, user := range users {
		key := "users/" + strconv.Itoa(i+1)
		jsonData, _ := json.Marshal(user)
		server.Storage.Set(key, json.RawMessage(jsonData))
	}

	// Add some logs
	for i := 1; i <= 3; i++ {
		key := "logs/" + strconv.Itoa(i)
		data := map[string]interface{}{
			"level":   "info",
			"message": "Log entry " + strconv.Itoa(i),
			"time":    time.Now().Format(time.RFC3339),
		}
		jsonData, _ := json.Marshal(data)
		server.Storage.Set(key, json.RawMessage(jsonData))
	}

	// Add config
	config := map[string]interface{}{
		"theme":    "dark",
		"language": "en",
		"version":  "1.0.0",
	}
	configData, _ := json.Marshal(config)
	server.Storage.Set("config", json.RawMessage(configData))

	// Add statistics with multiglob path
	stats := map[string]interface{}{
		"views":    1234,
		"clicks":   567,
		"sessions": 89,
	}
	statsData, _ := json.Marshal(stats)
	server.Storage.Set("statistics/2024/12/daily", json.RawMessage(statsData))
	server.Storage.Set("statistics/2024/12/weekly", json.RawMessage(statsData))
	server.Storage.Set("statistics/2024/11/daily", json.RawMessage(statsData))
}
