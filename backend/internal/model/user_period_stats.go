package model

import "time"

// UserPeriodStats is one member's transfer totals for one calendar month (UTC),
// rolled up from announce_events.
//
// It exists because the raw log is pruned: announce_log_retention_days bounds how
// long per-announce rows are kept, and once they are gone the only way to answer
// "how much did this member upload in June" is to have counted it beforehand.
// These rows are kept indefinitely and are additive — the rollup adds each closed
// day's deltas exactly once, tracked by announce_rollup_state.
type UserPeriodStats struct {
	UserID int64
	// YearMonth is 'YYYY-MM' in UTC. Fixed width, so it sorts chronologically.
	YearMonth string

	// Uploaded and Downloaded are sums of the per-announce deltas, not the
	// client-reported cumulative totals.
	Uploaded   int64
	Downloaded int64
	// CountedDownloaded is what counted toward ratio, after any freeleech
	// discount. It diverges from Downloaded whenever a torrent was free.
	CountedDownloaded int64

	Announces     int64
	SeedAnnounces int64
	UpdatedAt     time.Time
}
