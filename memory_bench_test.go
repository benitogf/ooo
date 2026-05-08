package ooo_test

import (
	"os"
	"testing"

	"github.com/benitogf/ooo"
	"github.com/benitogf/ooo/oootest"
)

// go test -bench=.

func BenchmarkMemoryStorageSetGetDel(b *testing.B) {
	b.ReportAllocs()
	server := ooo.Server{}
	server.Silence = true
	server.Start("localhost:9889")
	defer server.Close(os.Interrupt)
	oootest.StorageSetGetDelTestBenchmark(server.Storage, b)
}
