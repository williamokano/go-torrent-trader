package model

import "time"

// Invite represents a user invitation to join the tracker.
type Invite struct {
	ID         int64
	InviterID  int64
	InviteeID  *int64     // set when redeemed (maps to used_by_id in DB)
	Token      string     // unique invite token
	Redeemed   bool       // derived: true when InviteeID is set
	RedeemedAt *time.Time // maps to used_at in DB
	ExpiresAt  time.Time
	CreatedAt  time.Time

	// Enrichment fields (populated by service, not persisted)
	InviteeName string       // username of the invitee (if redeemed)
	Invitee     *InviteeView // full invitee stats (if redeemed)
}

// InviteeView is the data shown about an invitee in the invite list.
type InviteeView struct {
	ID         int64   `json:"id"`
	Username   string  `json:"username"`
	Uploaded   int64   `json:"uploaded"`
	Downloaded int64   `json:"downloaded"`
	Ratio      float64 `json:"ratio"`
	Enabled    bool    `json:"enabled"`
	Warned     bool    `json:"warned"`
	CreatedAt  string  `json:"created_at"`
}
