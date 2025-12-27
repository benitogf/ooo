package explorer_test

import (
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func TestKeyEditor(t *testing.T) {
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
		chromedp.Navigate(ts.URL+"/#/storage/key/static/config"),
		chromedp.Sleep(3*time.Second),
		chromedp.FullScreenshot(&buf, 90),
	)
	if err != nil {
		t.Fatalf("failed to load key editor: %v", err)
	}
	ts.SaveScreenshot(t, "key_editor", buf)

	// Verify editor is displayed with data (React uses regular DOM, no shadow DOM)
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
		t.Error("JSON editor not found in key-editor component")
	}
}

func TestGlobKeysList(t *testing.T) {
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
		chromedp.Navigate(ts.URL+"/#/storage/key/glob/test%2F*"),
		chromedp.Sleep(3*time.Second),
		chromedp.FullScreenshot(&buf, 90),
	)
	if err != nil {
		t.Fatalf("failed to load glob keys list: %v", err)
	}
	ts.SaveScreenshot(t, "glob_keys_list", buf)

	// Verify keys are displayed (React uses regular DOM, no shadow DOM)
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
	t.Logf("Key rows found: %d", keyCount)
	if keyCount < 2 {
		t.Errorf("expected at least 2 keys, got %d", keyCount)
	}
}
