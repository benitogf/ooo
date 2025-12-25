package ooo

import (
	"os"
	"testing"
)

// go test -bench=.

func BenchmarkMemoryStorageSetGetDel(b *testing.B) {
	b.ReportAllocs()
	app := Server{}
	app.Silence = true
	app.Start("localhost:9889")
	defer app.Close(os.Interrupt)
	StorageSetGetDelTestBenchmark(app.Storage, b)
}
