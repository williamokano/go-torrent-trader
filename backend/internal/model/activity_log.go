package model

import "time"

// ActivityLog represents a single append-only activity log entry for site transparency.
type ActivityLog struct {
	ID        int64
	EventType string
	ActorID   *int64
	Message   string
	Metadata  *string // JSON, optional extra data
	// StaffOnly hides this entry from members whatever its event type: a
	// torrent still awaiting moderation, rejected or hidden is not public yet,
	// and an anonymous uploader is not to be named.
	StaffOnly bool
	CreatedAt time.Time
}
