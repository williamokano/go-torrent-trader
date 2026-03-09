package model

import (
	"encoding/json"
	"time"
)

// TorrentFile represents a single file inside a torrent.
type TorrentFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// Torrent represents a torrent file registered in the tracker.
type Torrent struct {
	ID               int64
	Name             string
	InfoHash         []byte
	Size             int64
	Description      *string
	Nfo              *string
	CategoryID       int64
	CategoryName     string
	CategoryImageURL *string
	UploaderID       int64
	Anonymous        bool
	Seeders          int
	Leechers         int
	TimesCompleted   int
	CommentsCount    int
	Visible          bool
	Banned           bool
	Free             bool
	Silver           bool
	FileCount        int
	Files            *json.RawMessage // JSONB array of TorrentFile, nullable
	UploaderName     string           // Resolved via JOIN; "Anonymous" when anonymous=true
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
