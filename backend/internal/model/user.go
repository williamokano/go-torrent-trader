package model

import "time"

// User represents a registered member of the tracker.
type User struct {
	ID             int64
	Username       string
	Email          string
	PasswordHash   string
	PasswordScheme string
	Passkey        *string
	GroupID        int64
	Uploaded       int64
	Downloaded     int64
	Avatar         *string
	Title          *string
	Info           *string
	Enabled        bool
	Parked         bool
	IP             *string
	LastLogin      *time.Time
	LastAccess     *time.Time
	Invites        int
	Warned         bool
	WarnUntil      *time.Time
	Donor          bool
	InvitedBy      *int64
	CanDownload    bool
	CanUpload      bool
	CanChat        bool
	CanForum       bool
	DisabledUntil  *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
