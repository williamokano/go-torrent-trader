package model

import "time"

// Invite represents a user invitation to join the tracker.
type Invite struct {
	ID         int64
	InviterID  int64
	InviteeID  *int64     // set when redeemed (maps to used_by_id in DB)
	Email      string     // invited email
	Token      string     // unique invite token
	Redeemed   bool       // derived: true when InviteeID is set
	RedeemedAt *time.Time // maps to used_at in DB
	ExpiresAt  time.Time
	CreatedAt  time.Time
}
