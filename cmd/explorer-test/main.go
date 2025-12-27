package main

import (
	"log"

	"github.com/benitogf/ooo"
)

func main() {
	app := ooo.Server{}
	app.ForcePatch = true

	// Register some test filters using OpenFilter
	app.OpenFilter("test/*")
	app.OpenFilter("users/*")
	app.OpenFilter("config")

	log.Println("Starting explorer test server on :8888")
	app.Start("localhost:8888")
	app.WaitClose()
}
