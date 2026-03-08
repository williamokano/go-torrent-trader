package model

import "time"

// Message represents a private message between two users.
type Message struct {
	ID               int64
	SenderID         int64
	SenderUsername   string // from JOIN
	ReceiverID       int64
	ReceiverUsername string // from JOIN
	Subject          string
	Body             string
	IsRead           bool
	SenderDeleted    bool
	ReceiverDeleted  bool
	ParentID         *int64
	CreatedAt        time.Time
}
