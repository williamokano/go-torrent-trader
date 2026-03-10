package model

import "time"

// RestrictionType represents the kind of privilege restriction.
const (
	RestrictionTypeDownload = "download"
	RestrictionTypeUpload   = "upload"
	RestrictionTypeChat     = "chat"
)

// Restriction represents a per-user privilege restriction record.
type Restriction struct {
	ID              int64
	UserID          int64
	RestrictionType string
	Reason          string
	IssuedBy        *int64
	ExpiresAt       *time.Time
	LiftedAt        *time.Time
	LiftedBy        *int64
	CreatedAt       time.Time

	// Joined fields for display.
	IssuedByUsername string
	LiftedByUsername string
}
