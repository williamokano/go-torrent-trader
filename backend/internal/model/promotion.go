package model

import "time"

// PromotionRule is the requirement to reach and hold one class on the
// auto-promotion ladder. A group with a PromotionRule is a rung; groups without
// one are off the ladder and never auto-assigned. A threshold of 0 means "no
// constraint" for that metric.
type PromotionRule struct {
	GroupID      int64
	MinRatio     float64
	MinUploaded  int64 // bytes
	MinAgeDays   int
	MinTorrents  int
	MinSeedHours int // estimated seeding hours within the configured window
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// PromotionUserMetrics is the per-user snapshot the promotion engine evaluates
// against a rule. Ratio is derived from uploaded/downloaded; SeedHours is
// estimated from the announce log over the configured window.
type PromotionUserMetrics struct {
	UserID     int64
	GroupID    int64
	Uploaded   int64
	Downloaded int64
	CreatedAt  time.Time
	Torrents   int
	SeedHours  int
}
