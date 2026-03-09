package model

import "time"

// TransferHistory records a user's completed download of a torrent.
type TransferHistory struct {
	ID           int64
	UserID       int64
	TorrentID    int64
	Uploaded     int64
	Downloaded   int64
	Seeder       bool
	CompletedAt  time.Time
	LastAnnounce time.Time
}
