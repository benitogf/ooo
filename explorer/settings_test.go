package explorer_test

import (
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func TestSettingsPage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping browser test in short mode")
	}

	ts := SetupTestServer(t)
	defer ts.Close()

	ctx, cancel := ChromeContext(t, 30*time.Second)
	defer cancel()

	var buf []byte
	err := chromedp.Run(ctx,
		chromedp.Navigate(ts.URL+"/#/settings"),
		chromedp.Sleep(2*time.Second),
		chromedp.FullScreenshot(&buf, 90),
	)
	if err != nil {
		t.Fatalf("failed to load settings page: %v", err)
	}
	ts.SaveScreenshot(t, "settings_page", buf)

	// Verify settings page is displayed (React uses regular DOM, no shadow DOM)
	var hasSettings bool
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`
			(function() {
				const root = document.getElementById('root');
				if (!root) return false;
				const infoGrid = root.querySelector('.info-grid');
				return !!infoGrid;
			})()
		`, &hasSettings),
	)
	if err != nil {
		t.Fatalf("failed to check settings: %v", err)
	}
	if !hasSettings {
		t.Error("Settings page not found")
	}
}
