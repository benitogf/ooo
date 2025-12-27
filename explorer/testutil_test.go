package explorer_test

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/benitogf/ooo"
	"github.com/chromedp/chromedp"
)

// TestServer holds a running ooo server for UI tests
type TestServer struct {
	Server    *ooo.Server
	URL       string
	ScreenDir string
}

// SetupTestServer creates and starts a test server with initial data
func SetupTestServer(t *testing.T) *TestServer {
	t.Helper()

	server := &ooo.Server{Silence: true, ForcePatch: true}
	server.OpenFilter("test/*")
	server.OpenFilter("config")
	server.Start("localhost:0")

	serverURL := "http://" + server.Address

	// Create screenshots directory
	screenshotDir := filepath.Join(os.TempDir(), "explorer_screenshots")
	if err := os.MkdirAll(screenshotDir, 0755); err != nil {
		t.Fatalf("failed to create screenshot dir: %v", err)
	}

	ts := &TestServer{
		Server:    server,
		URL:       serverURL,
		ScreenDir: screenshotDir,
	}

	t.Logf("Server running at: %s", serverURL)
	t.Logf("Screenshots: %s", screenshotDir)

	return ts
}

// Close shuts down the test server
func (ts *TestServer) Close() {
	ts.Server.Close(os.Interrupt)
}

// CreateData creates a key with the given data
func (ts *TestServer) CreateData(t *testing.T, key, data string) {
	t.Helper()
	req, _ := http.NewRequest("POST", ts.URL+"/"+key, bytes.NewBufferString(data))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Logf("Failed to create %s: %v", key, err)
		return
	}
	resp.Body.Close()
	t.Logf("Created %s: status %d", key, resp.StatusCode)
}

// CreateTestData creates standard test data
func (ts *TestServer) CreateTestData(t *testing.T) {
	t.Helper()
	ts.CreateData(t, "config", `{"name":"test config","value":123}`)
	ts.CreateData(t, "test/item1", `{"title":"Item 1","active":true}`)
	ts.CreateData(t, "test/item2", `{"title":"Item 2","active":false}`)
}

// SaveScreenshot saves a screenshot to the screenshots directory
func (ts *TestServer) SaveScreenshot(t *testing.T, name string, buf []byte) {
	t.Helper()
	filename := filepath.Join(ts.ScreenDir, name+".png")
	if err := os.WriteFile(filename, buf, 0644); err != nil {
		t.Logf("failed to write screenshot: %v", err)
	}
	t.Logf("Screenshot saved: %s", filename)
}

// ChromeContext creates a chromedp context for browser testing
func ChromeContext(t *testing.T, timeout time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.WindowSize(1280, 800),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)

	ctx, ctxCancel := chromedp.NewContext(allocCtx)

	ctx, timeoutCancel := context.WithTimeout(ctx, timeout)

	cancel := func() {
		timeoutCancel()
		ctxCancel()
		allocCancel()
	}

	return ctx, cancel
}
