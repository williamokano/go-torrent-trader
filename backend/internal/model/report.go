package model

import "time"

// Report represents a user-submitted report about a torrent.
type Report struct {
	ID         int64
	ReporterID int64
	TorrentID  *int64
	Reason     string
	Resolved   bool
	ResolvedBy *int64
	ResolvedAt *time.Time
	CreatedAt  time.Time

	// Enrichment fields (populated by JOINs, not persisted)
	ReporterUsername string
	TorrentName     string
}
