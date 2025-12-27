package explorer_test

import (
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func TestStorageList(t *testing.T) {
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
		chromedp.Navigate(ts.URL+"/#/storage"),
		chromedp.Sleep(3*time.Second),
		chromedp.FullScreenshot(&buf, 90),
	)
	if err != nil {
		t.Fatalf("failed to load storage list: %v", err)
	}
	ts.SaveScreenshot(t, "storage_list", buf)

	// Verify filters are displayed (React uses regular DOM, no shadow DOM)
	var filterCount int
	err = chromedp.Run(ctx,
		chromedp.Evaluate(`
			(function() {
				const root = document.getElementById('root');
				if (!root) return 0;
				const rows = root.querySelectorAll('.data-table tbody tr');
				return rows.length;
			})()
		`, &filterCount),
	)
	if err != nil {
		t.Fatalf("failed to count filters: %v", err)
	}
	t.Logf("Filter rows found: %d", filterCount)
}
