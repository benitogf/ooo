package explorer_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/benitogf/ooo"
)

func TestFileServing(t *testing.T) {
	server := &ooo.Server{Silence: true}
	server.Start("localhost:0")
	defer server.Close(nil)

	baseURL := "http://" + server.Address

	tests := []struct {
		path     string
		contains string
	}{
		{"/", "react.min.js"},
		{"/react.min.js", "React"},
		{"/react-dom.min.js", "ReactDOM"},
		{"/babel.min.js", "Babel"},
		{"/styles.css", "body"},
		{"/components/Icons.jsx", "IconBox"},
		{"/components/ExplorerApp.jsx", "ExplorerApp"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			resp, err := http.Get(baseURL + tt.path)
			if err != nil {
				t.Fatalf("Failed to fetch %s: %v", tt.path, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != 200 {
				t.Errorf("Expected 200 for %s, got %d", tt.path, resp.StatusCode)
				return
			}

			body, _ := io.ReadAll(resp.Body)
			if !strings.Contains(string(body), tt.contains) {
				t.Errorf("Expected %s to contain %q, got first 100 chars: %s", tt.path, tt.contains, string(body)[:min(100, len(body))])
			}
		})
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
