package model

import "time"

// CheatFlag flag types.
const (
	CheatFlagImpossibleUploadSpeed = "impossible_upload_speed"
	CheatFlagUploadNoDownloaders   = "upload_no_downloaders"
	CheatFlagLeftMismatch          = "left_mismatch"
)

// CheatFlag represents a suspected cheating event flagged by the tracker.
type CheatFlag struct {
	ID           int64      `json:"id"`
	UserID       int64      `json:"user_id"`
	TorrentID    *int64     `json:"torrent_id"`
	FlagType     string     `json:"flag_type"`
	Details      string     `json:"details"`
	Dismissed    bool       `json:"dismissed"`
	DismissedBy  *int64     `json:"dismissed_by"`
	DismissedAt  *time.Time `json:"dismissed_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	Username     string     `json:"username,omitempty"`
	TorrentName  string     `json:"torrent_name,omitempty"`
	DismisserName string   `json:"dismisser_name,omitempty"`
}
