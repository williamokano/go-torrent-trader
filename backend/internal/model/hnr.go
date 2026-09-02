package model

import "time"

// HnR record states.
const (
	HnRStateActive    = "active"
	HnRStateBreach    = "hnr"
	HnRStateSatisfied = "satisfied"
	HnRStateCleared   = "cleared"
	HnRStateWaived    = "waived"
)

// HnR penalty stage actions.
const (
	HnRActionNotify      = "notify"
	HnRActionWarn        = "warn"
	HnRActionRestrict    = "restrict"
	HnRActionFinalNotice = "final_notice"
	HnRActionBan         = "ban"
)

// HnR run statuses and triggers.
const (
	HnRRunStatusRunning = "running"
	HnRRunStatusSuccess = "success"
	HnRRunStatusFailed  = "failed"

	HnRRunTriggerSchedule = "schedule"
	HnRRunTriggerManual   = "manual"
)

// HnRRule is the per-class HnR policy. A group is subject to HnR if and only
// if it has a row here — mirroring PromotionRule, a class with no rule (e.g.
// VIP) is exempt without special-case code anywhere else.
type HnRRule struct {
	GroupID              int64
	RequiredSeedHours    int
	RequiredRatio        float64
	InactivityGraceHours int
	MaxDaysToSatisfy     int // 0 = no hard cap
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// HnRRecord is one row per (user, torrent) snatch: the accumulator fed by the
// announce path and the state machine the daemon and the announce path both
// drive.
type HnRRecord struct {
	ID            int64
	UserID        int64
	TorrentID     int64
	State         string
	CompletedAt   time.Time
	LastSeenAt    time.Time
	SeededSeconds int64
	Uploaded      int64
	BreachedAt    *time.Time
	ResolvedAt    *time.Time

	// Joined fields for display (populated by queries that need them).
	TorrentName string
	TorrentSize int64
	Username    string
}

// HnRPenaltyStage is one ordered rung of the site-wide penalty ladder.
type HnRPenaltyStage struct {
	Stage            int
	MinActiveHnR     int
	MinDaysInPrev    int
	Action           string
	RestrictionTypes []string
	RestrictionDays  int // 0 = indefinite
	MessageTemplate  string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// HnRUserState is a user's current position on the penalty ladder.
type HnRUserState struct {
	UserID            int64
	Stage             int // 0 = not on the ladder
	StageEnteredAt    time.Time
	LastNotifiedStage int
	UpdatedAt         time.Time
}

// HnRRun is one daemon run's audit trail.
type HnRRun struct {
	ID             int64
	StartedAt      time.Time
	FinishedAt     *time.Time
	Status         string
	Trigger        string
	TriggeredBy    *int64
	Scanned        int
	Breached       int
	Satisfied      int
	StagesAdvanced int
	StagesDecayed  int
	Purged         int
	Error          *string
}
