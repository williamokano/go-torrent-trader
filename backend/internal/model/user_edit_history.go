package model

import "time"

// UserEditHistory is one recorded change to a user profile field made through
// the admin edit endpoint: which field, the value before and after, and who
// made the change. Rows are append-only.
type UserEditHistory struct {
	ID        int64
	UserID    int64
	ChangedBy *int64 // nil when the acting admin was since deleted
	Field     string
	OldValue  string
	NewValue  string
	CreatedAt time.Time
	// ChangedByUsername is snapshotted at write time so the trail survives
	// the acting admin's deletion; reads prefer the live username via JOIN.
	ChangedByUsername string
}
