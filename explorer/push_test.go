package explorer_test

import (
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func TestPushEditor(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping browser test in short mode")
	}

	ts := SetupTestServer(t)
	defer ts.Close()
	ts.CreateTestData(t)

	ctx, cancel := ChromeContext(t, 30*time.Second)
	defer cancel()

	var buf []byte
	err := chromedp.Run(ctx,
		chromedp.Navigate(ts.URL+"/#/storage/push/test%2F*"),
		chromedp.Sleep(3*time.Second),
		chromedp.FullScreenshot(&buf, 90),
	)
	if err != nil {
		t.Fatalf("failed to load push editor: %v", err)
	}
	ts.SaveScreenshot(t, "push_editor", buf)

	// Verify editor is displayed (React uses regular DOM, no shadow DOM)
	var hasEditor bool
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`
			(function() {
				const root = document.getElementById('root');
				if (!root) return false;
				const editor = root.querySelector('.editor-container');
				return !!editor;
			})()
		`, &hasEditor),
	)
	if err != nil {
		t.Fatalf("failed to check editor: %v", err)
	}
	if !hasEditor {
		t.Error("JSON editor not found in storage-push component")
	}
}

func TestPushOperation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping browser test in short mode")
	}

	ts := SetupTestServer(t)
	defer ts.Close()
	ts.CreateTestData(t)

	ctx, cancel := ChromeContext(t, 60*time.Second)
	defer cancel()

	// Navigate to push page
	var buf []byte
	err := chromedp.Run(ctx,
		chromedp.Navigate(ts.URL+"/#/storage/push/test%2F*"),
		chromedp.Sleep(2*time.Second),
		chromedp.FullScreenshot(&buf, 90),
	)
	if err != nil {
		t.Fatalf("failed to load push page: %v", err)
	}
	ts.SaveScreenshot(t, "push_before", buf)

	// Click push button (React uses regular DOM, no shadow DOM)
	var result string
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`
			(function() {
				const root = document.getElementById('root');
				if (!root) return 'no root';
				const btn = root.querySelector('.edit-page-actions .btn:not(.secondary)');
				if (!btn) return 'no button';
				btn.click();
				return 'clicked';
			})()
		`, &result),
		chromedp.Sleep(3*time.Second),
		chromedp.FullScreenshot(&buf, 90),
	)
	if err != nil {
		t.Fatalf("failed to click push: %v", err)
	}
	t.Logf("Push button result: %s", result)
	ts.SaveScreenshot(t, "push_after", buf)

	// Verify push worked by checking keys via API
	resp, err := http.Get(ts.URL + "/?api=keys&filter=test/*")
	if err != nil {
		t.Fatalf("Failed to get keys: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	t.Logf("Keys after push: %s", string(body))

	// Navigate to glob keys list and verify new key appears
	err = chromedp.Run(ctx,
		chromedp.Navigate(ts.URL+"/#/storage/key/glob/test%2F*"),
		chromedp.Sleep(3*time.Second),
		chromedp.FullScreenshot(&buf, 90),
	)
	if err != nil {
		t.Fatalf("failed to load glob keys: %v", err)
	}
	ts.SaveScreenshot(t, "glob_keys_after_push", buf)

	// Verify we have 3 keys now (2 original + 1 pushed) (React uses regular DOM)
	var keyCount int
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`
			(function() {
				const root = document.getElementById('root');
				if (!root) return 0;
				const rows = root.querySelectorAll('.data-table tbody tr');
				return rows.length;
			})()
		`, &keyCount),
	)
	if err != nil {
		t.Fatalf("failed to count keys: %v", err)
	}
	t.Logf("Key rows after push: %d", keyCount)
	if keyCount < 3 {
		t.Errorf("expected at least 3 keys after push, got %d", keyCount)
	}
}
