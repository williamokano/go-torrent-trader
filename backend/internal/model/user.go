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
	// ActivatedAt is set the moment an account is first known to be under a
	// real person's control: a no-confirmation registration (set at create),
	// a completed email confirmation, or a first successful login — whichever
	// happens first. Unlike LastAccess (only touched by ActivityTracker, on a
	// *subsequent* authenticated request, debounced every 5 minutes),
	// ActivatedAt requires no follow-up request, so it stays nil precisely for
	// registrations that were never activated at all. This is what the
	// cleanup worker's expired-registration purge (BE-8.19) keys off of.
	ActivatedAt   *time.Time
	Invites       int
	BonusPoints   int64
	Warned        bool
	WarnUntil     *time.Time
	Donor         bool
	InvitedBy     *int64
	CanDownload   bool
	CanUpload     bool
	CanChat       bool
	CanForum      bool
	CanInvite     bool
	DisabledUntil *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
