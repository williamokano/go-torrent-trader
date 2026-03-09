package model

import "time"

// Warning types
const (
	WarningTypeManual   = "manual"
	WarningTypeRatioSoft = "ratio_soft"
	WarningTypeRatioBan  = "ratio_ban"
)

// Warning statuses
const (
	WarningStatusActive    = "active"
	WarningStatusResolved  = "resolved"
	WarningStatusLifted    = "lifted"
	WarningStatusEscalated = "escalated"
)

// Warning represents a user warning record.
type Warning struct {
	ID           int64      `json:"id"`
	UserID       int64      `json:"user_id"`
	Type         string     `json:"type"`
	Reason       string     `json:"reason"`
	IssuedBy     *int64     `json:"issued_by"`
	Status       string     `json:"status"`
	LiftedAt     *time.Time `json:"lifted_at"`
	LiftedBy     *int64     `json:"lifted_by"`
	LiftedReason *string    `json:"lifted_reason"`
	ExpiresAt    *time.Time `json:"expires_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`

	// Joined fields (populated by queries with JOINs)
	Username       string  `json:"username,omitempty"`
	IssuedByName   *string `json:"issued_by_name,omitempty"`
	LiftedByName   *string `json:"lifted_by_name,omitempty"`
}
