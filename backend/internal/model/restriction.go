package model

import "time"

// RestrictionType represents the kind of privilege restriction.
const (
	RestrictionTypeDownload = "download"
	RestrictionTypeUpload   = "upload"
	RestrictionTypeChat     = "chat"
	RestrictionTypeInvite   = "invite"
	// RestrictionTypeFeed suspends live feed access. Feeds are all-or-nothing:
	// there is no per-feed restriction, so this covers every one of them.
	RestrictionTypeFeed = "feed"
	// RestrictionTypeForum suspends forum posting. Unlike the others, no
	// group-level Permissions field can carry per-user forum restriction
	// state (see model/permissions.go's note on CanFeed) — a live per-user
	// gate reads this instead, in the style of FeedAccessService.
	RestrictionTypeForum = "forum"
)

// RestrictionSource identifies which system issued a restriction, so a lift
// can target exactly its own cause rather than inferring "the" active
// restriction of a type (see migration 082). HasActiveByType and the
// privilege-flag restore stay source-agnostic on purpose — the flag must
// reflect "no active restriction from anywhere" — only a lift is source-scoped.
const (
	RestrictionSourceManual       = "manual"
	RestrictionSourceRatioWarning = "ratio_warning"
	RestrictionSourceHnR          = "hnr"
)

// Restriction represents a per-user privilege restriction record.
type Restriction struct {
	ID              int64
	UserID          int64
	RestrictionType string
	Reason          string
	// Source is which system issued this restriction (RestrictionSource*).
	// Defaults to "manual" — see migration 082.
	Source    string
	IssuedBy  *int64
	ExpiresAt *time.Time
	LiftedAt  *time.Time
	LiftedBy  *int64
	CreatedAt time.Time

	// Joined fields for display.
	IssuedByUsername string
	LiftedByUsername string
}
