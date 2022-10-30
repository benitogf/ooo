package ooo_test

import "github.com/benitogf/ooo"

func ExampleServer() {
	app := ooo.Server{}
	app.Start("localhost:8800")
	app.WaitClose()
}
