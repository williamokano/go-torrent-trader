package model

import "time"

// InviteDistributionRule is one group's auto-invite-grant policy. A group with
// a rule is eligible for the auto-distribution job; groups without one never
// receive automatic grants. MinRatio/MinDownloadedBytes follow
// PromotionRule's "0 = no constraint" convention, as does MaxDownloadedBytes
// (0 = unbounded). MaxInvites is a per-user ceiling rather than a threshold —
// a value of 0 means the role grants nothing, the safe default until an admin
// configures a real cap.
type InviteDistributionRule struct {
	GroupID            int64
	MinRatio           float64
	MinDownloadedBytes int64
	MaxDownloadedBytes int64 // 0 = unbounded
	MaxInvites         int   // per-user ceiling, not a shared pool; 0 = grant nothing
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// UserInviteState is the current invite balance and invite privilege flag for
// a user — the engine inputs the shared group-metrics query doesn't carry,
// needed to enforce the per-user ceiling and the can_invite gate.
type UserInviteState struct {
	Username  string
	Invites   int
	CanInvite bool
}
