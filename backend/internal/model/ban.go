package model

import "time"

// BannedEmail represents a banned email pattern (e.g. "%@mailinator.com" or "spammer@example.com").
type BannedEmail struct {
	ID        int64     `json:"id"`
	Pattern   string    `json:"pattern"`
	Reason    *string   `json:"reason"`
	CreatedBy *int64    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// BannedIP represents a banned IP address or CIDR range.
type BannedIP struct {
	ID        int64     `json:"id"`
	IPRange   string    `json:"ip_range"`
	Reason    *string   `json:"reason"`
	CreatedBy *int64    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}
