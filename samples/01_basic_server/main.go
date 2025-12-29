// Package main demonstrates a basic ooo server setup.
// This is the simplest way to start an ooo server.
package main

import "github.com/benitogf/ooo"

func main() {
	server := ooo.Server{}
	server.Start("0.0.0.0:8800")
	server.WaitClose()
}
