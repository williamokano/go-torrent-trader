package service

import (
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// HnRStatus is the shared evaluator's verdict for one open hnr_records row.
// The daemon persists this verdict (and acts on it); the member-facing read
// path evaluates it live and never writes it. Keeping both callers on the
// same function is what stops them from quietly disagreeing about what
// counts as a breach — see EvaluateHnRRecord's doc comment.
type HnRStatus string

const (
	// HnRStatusMonitoring: still within grace and/or the hard cap, policy not
	// yet met. Nothing to do.
	HnRStatusMonitoring HnRStatus = "monitoring"
	// HnRStatusSatisfied: the seed-hours or ratio requirement was met.
	HnRStatusSatisfied HnRStatus = "satisfied"
	// HnRStatusBreach: the inactivity grace or the hard cap was exceeded
	// without the policy being met.
	HnRStatusBreach HnRStatus = "breach"
	// HnRStatusExempt: not applicable at all — the torrent was flagged
	// hnr_exempt after the snatch, or the user's current class carries no
	// HnR rule (e.g. promoted to VIP). Resolved via MarkWaived, not
	// MarkSatisfied: neither case means the seeding requirement was met.
	HnRStatusExempt HnRStatus = "exempt"
)

// EvaluateHnRRecord is the single place "is this a breach" gets decided, used
// by both the daemon (which persists the verdict and acts on it) and the
// member-facing read path (which evaluates it live so the page is never
// stale between daemon runs — see the "Real-time member status" design note).
// Two implementations of this decision is exactly how they would quietly
// disagree, so nothing else in this codebase may re-derive it.
//
// Satisfaction is "seed for N hours OR reach ratio R" — required_ratio is
// checked against the torrent's raw size, deliberately not against counted
// (freeleech-discounted) download, which is what makes every torrent,
// including free ones, eligible: a freeleech snatch still has a real size to
// seed back.
//
// A zero threshold is unconstrained on that dimension, exactly like
// promotion_rules: RequiredSeedHours=0 makes the seed-time arm trivially true
// (0 accumulated seconds already satisfies "at least 0"), and
// RequiredRatio<=0 makes the ratio arm trivially true the same way. A rule
// with both at zero is a valid (if unusual) "no requirement" configuration,
// not a bug to guard against.
func EvaluateHnRRecord(in repository.HnREvalInput, now time.Time) HnRStatus {
	if in.TorrentExempt || in.Rule == nil {
		return HnRStatusExempt
	}

	requiredSeedSeconds := int64(in.Rule.RequiredSeedHours) * 3600
	seedTimeOK := in.Record.SeededSeconds >= requiredSeedSeconds
	ratioOK := in.Rule.RequiredRatio <= 0 ||
		float64(in.Record.Uploaded) >= in.Rule.RequiredRatio*float64(in.TorrentSize)

	if seedTimeOK || ratioOK {
		return HnRStatusSatisfied
	}

	// Unlike MaxDaysToSatisfy below, grace has no "0 disables" special case: a
	// grace of 0 means zero tolerance for inactivity (breach the instant a
	// seeding announce is overdue), which is what the plain arithmetic already
	// gives — now.After(LastSeenAt.Add(0)) is true as soon as any time has
	// passed. Only the hard cap treats 0 as "unlimited", because "breach the
	// instant a torrent completes" would not be a meaningful cap at all.
	graceDeadline := in.Record.LastSeenAt.Add(time.Duration(in.Rule.InactivityGraceHours) * time.Hour)
	if now.After(graceDeadline) {
		return HnRStatusBreach
	}
	if in.Rule.MaxDaysToSatisfy > 0 && now.After(in.Record.CompletedAt.AddDate(0, 0, in.Rule.MaxDaysToSatisfy)) {
		return HnRStatusBreach
	}
	return HnRStatusMonitoring
}
