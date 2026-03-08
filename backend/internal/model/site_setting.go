package model

import "time"

// SiteSetting represents a key-value site configuration entry.
type SiteSetting struct {
	Key       string
	Value     string
	UpdatedAt time.Time
}
