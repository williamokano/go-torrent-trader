package model

import "time"

// ModNote represents a private staff note on a user.
type ModNote struct {
	ID             int64
	UserID         int64
	AuthorID       int64
	Note           string
	CreatedAt      time.Time
	AuthorUsername string // populated via JOIN
}
