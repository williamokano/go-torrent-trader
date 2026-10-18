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
	ActivatedAt *time.Time
	// AwaitingConfirmation is true from a registration that needs email
	// confirmation until the email is confirmed. It is what makes a disabled
	// account confirmable: a ban also sets Enabled=false, so Enabled alone
	// cannot tell the two apart. Disable clears it.
	AwaitingConfirmation bool
	Invites              int
	BonusPoints          int64
	Warned               bool
	WarnUntil            *time.Time
	Donor                bool
	InvitedBy            *int64
	CanDownload          bool
	CanUpload            bool
	CanChat              bool
	// CanFeed is the per-user half of live feed access: the class grants it,
	// and a restriction can take it away from one member without moving them
	// out of their class.
	CanFeed       bool
	CanForum      bool
	CanInvite     bool
	DisabledUntil *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Disable turns the account off. It also ends any pending email confirmation,
// so confirming can never undo a ban.
func (u *User) Disable() {
	u.Enabled = false
	u.AwaitingConfirmation = false
}
