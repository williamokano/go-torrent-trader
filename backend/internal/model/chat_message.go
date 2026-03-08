package model

import "time"

// ChatMessage represents a message in the site-wide shoutbox/chat.
type ChatMessage struct {
	ID        int64
	UserID    int64
	Username  string // populated via JOIN
	Message   string
	CreatedAt time.Time
}
