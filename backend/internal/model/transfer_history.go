package model

import "time"

// TransferHistory records a user's completed download of a torrent.
// TorrentID is nullable because deleted torrents use ON DELETE SET NULL
// to preserve transfer history.
type TransferHistory struct {
	ID           int64
	UserID       int64
	TorrentID    *int64 // nullable — set to NULL when the torrent is deleted
	Uploaded     int64
	Downloaded   int64
	Seeder       bool
	CompletedAt  time.Time
	LastAnnounce time.Time
}
