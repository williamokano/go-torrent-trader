package model

import "time"

// ChatMute represents a temporary mute applied to a user in the chat.
type ChatMute struct {
	ID        int64
	UserID    int64
	MutedBy   *int64
	Reason    string
	ExpiresAt time.Time
	CreatedAt time.Time
}
