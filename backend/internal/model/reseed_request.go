package model

import "time"

// ReseedRequest represents a user's request to reseed a torrent.
type ReseedRequest struct {
	ID          int64
	TorrentID   int64
	RequesterID int64
	CreatedAt   time.Time
}
