package ooo

import (
	"os"
	"testing"
)

// go test -bench=.

func BenchmarkMemoryStorageSetGetDel(b *testing.B) {
	b.ReportAllocs()
	server := Server{}
	server.Silence = true
	server.Start("localhost:9889")
	defer server.Close(os.Interrupt)
	StorageSetGetDelTestBenchmark(server.Storage, b)
}
