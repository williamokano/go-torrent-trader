package model

import "time"

// Torrent represents a torrent file registered in the tracker.
type Torrent struct {
	ID             int64
	Name           string
	InfoHash       []byte
	Size           int64
	Description    *string
	Nfo            *string
	CategoryID     int64
	CategoryName   string
	UploaderID     int64
	Anonymous      bool
	Seeders        int
	Leechers       int
	TimesCompleted int
	CommentsCount  int
	Visible        bool
	Banned         bool
	Free           bool
	Silver         bool
	FileCount      int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
