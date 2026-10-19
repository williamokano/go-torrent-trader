package model

import "time"

// Set is one field of a UserPatch: written when Valid, left alone otherwise.
// Not sql.Null: there, not valid means "write NULL"; here it means "leave
// the column as it is", and a Set[*T] can still write NULL with To(nil).
type Set[T any] struct {
	Value T
	Valid bool
}

// To marks a patch field to be written with v.
func To[T any](v T) Set[T] {
	return Set[T]{Value: v, Valid: true}
}

// UserPatch names the user columns one write changes. Fields left unset are
// not written, so a write touches only what its caller decided: a stale copy
// of the row can never put back a ban, a demotion or a password that changed
// after it was read.
//
// Some columns are deliberately absent: the privilege flags, invites, stats
// and bonus points each have their own atomic writer, and
// awaiting_confirmation is only ever set by Create.
type UserPatch struct {
	Username       Set[string]
	Email          Set[string]
	PasswordHash   Set[string]
	PasswordScheme Set[string]
	Passkey        Set[*string]
	GroupID        Set[int64]
	Avatar         Set[*string]
	Title          Set[*string]
	Info           Set[*string]
	// enabled is only reachable through SetEnabled, which keeps the pending
	// confirmation and the temporary-ban expiry in step with it.
	enabled       Set[bool]
	Parked        Set[bool]
	Warned        Set[bool]
	WarnUntil     Set[*time.Time]
	Donor         Set[bool]
	IP            Set[*string]
	LastLogin     Set[*time.Time]
	DisabledUntil Set[*time.Time]
	// EndConfirmation clears awaiting_confirmation.
	EndConfirmation bool
	// ActivateAt stamps activated_at, only if the account has none yet.
	ActivateAt Set[time.Time]
}

// SetEnabled turns the account on or off. Either way it ends any pending
// email confirmation: off, so confirming can never undo a ban; on, because
// enabling an unconfirmed account stands in for the confirmation — which is
// also why turning on counts as the account's activation. It also clears
// any temporary-ban expiry, so an old expiry cannot later lift a ban that is
// meant to be permanent; a timed ban sets DisabledUntil after this.
func (p *UserPatch) SetEnabled(on bool, now time.Time) {
	p.enabled = To(on)
	p.EndConfirmation = true
	p.DisabledUntil = To[*time.Time](nil)
	if on {
		p.ActivateAt = To(now)
	}
}

// Enabled reports the change SetEnabled recorded, if any.
func (p UserPatch) Enabled() Set[bool] {
	return p.enabled
}

// DisablePatch turns an account off; see SetEnabled.
func DisablePatch() UserPatch {
	var p UserPatch
	p.SetEnabled(false, time.Time{})
	return p
}

// EnablePatch turns an account on; see SetEnabled.
func EnablePatch(now time.Time) UserPatch {
	var p UserPatch
	p.SetEnabled(true, now)
	return p
}

// Apply makes the same change to an in-memory user that the repository makes
// to the row, so a caller's copy stays in step with what it wrote.
func (u *User) Apply(p UserPatch) {
	set(&u.Username, p.Username)
	set(&u.Email, p.Email)
	set(&u.PasswordHash, p.PasswordHash)
	set(&u.PasswordScheme, p.PasswordScheme)
	set(&u.Passkey, p.Passkey)
	set(&u.GroupID, p.GroupID)
	set(&u.Avatar, p.Avatar)
	set(&u.Title, p.Title)
	set(&u.Info, p.Info)
	set(&u.Enabled, p.enabled)
	set(&u.Parked, p.Parked)
	set(&u.Warned, p.Warned)
	set(&u.WarnUntil, p.WarnUntil)
	set(&u.Donor, p.Donor)
	set(&u.IP, p.IP)
	set(&u.LastLogin, p.LastLogin)
	set(&u.DisabledUntil, p.DisabledUntil)
	if p.EndConfirmation {
		u.AwaitingConfirmation = false
	}
	if p.ActivateAt.Valid && u.ActivatedAt == nil {
		at := p.ActivateAt.Value
		u.ActivatedAt = &at
	}
}

func set[T any](dst *T, f Set[T]) {
	if f.Valid {
		*dst = f.Value
	}
}
