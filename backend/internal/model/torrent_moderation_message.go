package model

import "time"

// TorrentModerationMessage is one entry in a torrent's moderation discussion
// thread (BE-8.22b), exchanged between staff and the uploader during review.
type TorrentModerationMessage struct {
	ID             int64
	TorrentID      int64
	AuthorID       int64
	Body           string
	CreatedAt      time.Time
	AuthorUsername string // resolved via JOIN
}
