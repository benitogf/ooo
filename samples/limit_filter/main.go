// Package main demonstrates the LimitFilter for capped collections.
// This example shows how to:
// - Use LimitFilter to maintain a max number of entries (count constraint)
// - Use LimitFilter with time-based retention (MaxAge constraint)
// - Combine count and time constraints (stricter one wins)
// - Use dynamic LimitFunc and MaxAgeFunc for runtime-configurable limits
// - Enable periodic background cleanup for automatic garbage collection
package main

import (
	"log"
	"time"

	"github.com/benitogf/ooo"
)

func main() {
	server := ooo.Server{Static: true}
	server.Start("0.0.0.0:8800")

	// ── Count-only ──────────────────────────────────────────────
	// Keep only the 100 most recent log entries.
	// When a new entry is added and count > 100, the oldest is deleted.
	server.LimitFilter("logs/*", ooo.LimitFilterConfig{
		Limit: 100,
	})

	// ── Time-only (retention policy) ────────────────────────────
	// Keep entries younger than 24 hours. Expired entries are filtered
	// out on read and deleted on each new write.
	server.LimitFilter("events/*", ooo.LimitFilterConfig{
		MaxAge: 24 * time.Hour,
	})

	// ── Combined count + time ───────────────────────────────────
	// Both constraints apply — the stricter one wins.
	// Keeps at most 1000 entries AND only those younger than 7 days.
	server.LimitFilter("metrics/*", ooo.LimitFilterConfig{
		Limit:  1000,
		MaxAge: 7 * 24 * time.Hour,
	})

	// ── Ascending order ─────────────────────────────────────────
	// Results sorted oldest-first instead of the default newest-first.
	server.LimitFilter("timeline/*", ooo.LimitFilterConfig{
		Limit: 50,
		Order: ooo.OrderAsc,
	})

	// ── Dynamic limit via LimitFunc ─────────────────────────────
	// The limit is evaluated at runtime on each read/write.
	// Useful when the cap comes from external configuration or state.
	maxNotifications := 50
	server.LimitFilter("notifications/*", ooo.LimitFilterConfig{
		LimitFunc: func() int { return maxNotifications },
	})

	// ── Dynamic retention via MaxAgeFunc ─────────────────────────
	// The max age is evaluated at runtime. Useful when retention
	// policies are stored in a database or remote config.
	retentionWindow := 30 * 24 * time.Hour
	server.LimitFilter("audit/*", ooo.LimitFilterConfig{
		MaxAgeFunc: func() time.Duration { return retentionWindow },
	})

	// ── Periodic background cleanup ─────────────────────────────
	// By default, expired entries are only cleaned up on new writes.
	// Enable Cleanup to run a background goroutine that periodically
	// scans and deletes expired entries even without new writes.
	// Interval defaults to 10 minutes; minimum allowed is 1 minute.
	server.LimitFilter("telemetry/*", ooo.LimitFilterConfig{
		MaxAge: 24 * time.Hour,
		Limit:  5000,
		Cleanup: ooo.CleanupConfig{
			Enabled:  true,
			Interval: 5 * time.Minute,
		},
	})

	log.Println("Server running with limit filters on :8800")
	log.Println("")
	log.Println("  POST /logs/*          - count-only (max 100)")
	log.Println("  POST /events/*        - time-only (max 24h)")
	log.Println("  POST /metrics/*       - combined (max 1000 / 7d)")
	log.Println("  POST /timeline/*      - ascending order (max 50)")
	log.Println("  POST /notifications/* - dynamic count (LimitFunc)")
	log.Println("  POST /audit/*         - dynamic retention (MaxAgeFunc)")
	log.Println("  POST /telemetry/*     - cleanup every 5min (max 5000 / 24h)")
	server.WaitClose()
}
