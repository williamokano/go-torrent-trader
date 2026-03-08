package model

import "time"

// ActivityLog represents a single append-only activity log entry for site transparency.
type ActivityLog struct {
	ID        int64
	EventType string
	ActorID   int64
	Message   string
	Metadata  *string // JSON, optional extra data
	CreatedAt time.Time
}
