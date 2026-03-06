package model

import "time"

// Comment represents a user comment on a torrent.
type Comment struct {
	ID        int64
	TorrentID int64
	UserID    int64
	Username  string
	Body      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Rating represents a user's rating of a torrent.
type Rating struct {
	ID        int64
	TorrentID int64
	UserID    int64
	Rating    int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// RatingStats holds aggregated rating information for a torrent.
type RatingStats struct {
	Average    float64
	Count      int
	UserRating *int
}
