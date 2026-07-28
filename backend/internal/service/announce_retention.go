package service

import "context"

// DefaultAnnounceLogRetentionDays is the seeded value of
// announce_log_retention_days, and the fallback every reader of that setting
// uses. One constant, because two readers disagreeing on a default is the same
// bug as two readers disagreeing on the value.
const DefaultAnnounceLogRetentionDays = 90

// AnnounceRetention separates what an operator asked for from what the site
// actually does.
//
// They are not always the same, and the difference used to be invisible. Class
// promotion gap-sums raw announce_events over its seeding window, so pruning
// inside that window would zero every member's seed hours and stop promotions
// with no other symptom — which is why the prune raises its own cutoff to cover
// it. That behaviour is correct and deliberate. What was wrong is that nothing
// said so: an operator who set 7 got 31, with a slog.Warn in the server log as
// the only trace, and members were told 7 about rows that live for 31.
//
// The operator most likely to be misled is the one who shortened the window for
// a privacy or legal reason and believes the raw announces are gone.
type AnnounceRetention struct {
	// ConfiguredDays is announce_log_retention_days as set. Zero means pruning is
	// switched off and raw announces are kept indefinitely.
	ConfiguredDays int
	// FloorDays is the shortest window other features still need. Zero when none
	// do.
	FloorDays int
	// EffectiveDays is how long raw announces actually survive: the configured
	// window, or the floor when that is longer. Zero still means "kept forever",
	// since a disabled prune deletes nothing regardless of any floor.
	EffectiveDays int
	// FloorReason names what is holding the window open, for an operator who
	// needs to know which setting to change. Empty when nothing is.
	FloorReason string
}

// Overridden reports whether the site is keeping announces longer than asked.
func (a AnnounceRetention) Overridden() bool {
	return a.FloorReason != ""
}

// ResolveAnnounceRetention works out the effective retention window.
//
// This is the single place that answers the question. tasks/lessons.md has the
// rule from the last time it was answered twice: "precedence chains belong in
// one function that every caller uses; a second implementation is a divergence
// waiting to happen". The prune, the member-facing endpoint and the admin panel
// all read this, so they cannot drift.
func ResolveAnnounceRetention(ctx context.Context, settings *SiteSettingsService) AnnounceRetention {
	var out AnnounceRetention
	if settings == nil {
		out.ConfiguredDays = DefaultAnnounceLogRetentionDays
		out.EffectiveDays = DefaultAnnounceLogRetentionDays
		return out
	}

	out.ConfiguredDays = settings.GetInt(ctx, SettingAnnounceLogRetentionDays, DefaultAnnounceLogRetentionDays)
	if out.ConfiguredDays < 0 {
		// A negative window is not a shorter window; it is a misconfiguration, and
		// the prune already treats non-positive as "disabled" rather than as a
		// cutoff in the future. Reported as 0 so every surface says the same thing.
		out.ConfiguredDays = 0
	}

	out.FloorDays, out.FloorReason = announceRetentionFloor(ctx, settings)

	// Pruning off means nothing is deleted, so no floor can be breached and none
	// needs reporting. Saying "held open by class promotion" to an operator who
	// has already chosen to keep everything forever would be noise.
	if out.ConfiguredDays == 0 {
		out.EffectiveDays = 0
		out.FloorReason = ""
		return out
	}

	out.EffectiveDays = out.ConfiguredDays
	if out.FloorDays > out.EffectiveDays {
		out.EffectiveDays = out.FloorDays
	} else {
		out.FloorReason = ""
	}
	return out
}

// announceRetentionFloor reports how far back other features still need raw
// announce rows, and what is asking.
//
// Only class promotion does today: PromotionRepo.SeedHoursByUser gap-sums
// announce_events over promotion_seed_window_days. When a second feature grows
// the same dependency it belongs here, so the floor stays one number with one
// explanation rather than a rule each caller has to remember.
func announceRetentionFloor(ctx context.Context, settings *SiteSettingsService) (int, string) {
	if !settings.GetBool(ctx, SettingPromotionEnabled, false) {
		return 0, ""
	}
	days := settings.GetInt(ctx, SettingPromotionSeedWindowDays, 30)
	if days < 1 {
		return 0, ""
	}
	// One day of headroom: the seeding estimate needs an announce on each side of
	// the window's start to measure the first gap, and the prune runs 45 minutes
	// before the promotion job reads it.
	return days + 1, "automatic class promotion, which reads raw announces over its seeding window"
}
