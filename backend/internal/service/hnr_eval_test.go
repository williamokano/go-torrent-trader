package service

import (
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

func baseRule() *model.HnRRule {
	return &model.HnRRule{
		RequiredSeedHours:    10,
		RequiredRatio:        1.0,
		InactivityGraceHours: 48,
		MaxDaysToSatisfy:     30,
	}
}

func TestEvaluateHnRRecord_ExemptWhenTorrentFlagged(t *testing.T) {
	now := time.Now()
	in := repository.HnREvalInput{
		Record:        model.HnRRecord{CompletedAt: now, LastSeenAt: now},
		Rule:          baseRule(),
		TorrentSize:   1000,
		TorrentExempt: true,
	}
	if got := EvaluateHnRRecord(in, now); got != HnRStatusExempt {
		t.Errorf("got %s, want exempt", got)
	}
}

func TestEvaluateHnRRecord_ExemptWhenNoRuleForClass(t *testing.T) {
	now := time.Now()
	in := repository.HnREvalInput{
		Record:      model.HnRRecord{CompletedAt: now, LastSeenAt: now},
		Rule:        nil,
		TorrentSize: 1000,
	}
	if got := EvaluateHnRRecord(in, now); got != HnRStatusExempt {
		t.Errorf("got %s, want exempt (nil rule = user's class not subject to HnR)", got)
	}
}

func TestEvaluateHnRRecord_SatisfiedBySeedTime(t *testing.T) {
	now := time.Now()
	rule := baseRule()
	in := repository.HnREvalInput{
		Record: model.HnRRecord{
			CompletedAt: now, LastSeenAt: now,
			SeededSeconds: int64(rule.RequiredSeedHours) * 3600, // exactly at the threshold
			Uploaded:      0,
		},
		Rule:        rule,
		TorrentSize: 1000,
	}
	if got := EvaluateHnRRecord(in, now); got != HnRStatusSatisfied {
		t.Errorf("got %s, want satisfied (met seed-hours exactly)", got)
	}
}

func TestEvaluateHnRRecord_SatisfiedByRatio_IgnoresFreeleech(t *testing.T) {
	// The plan's explicit requirement: every torrent, including freeleech
	// ones, is eligible for HnR, because the ratio denominator is the
	// torrent's raw size — not counted (freeleech-discounted) download. This
	// test proves the evaluator never even sees a "counted" figure: it takes
	// TorrentSize directly, so a torrent that would count zero download
	// still has a real size to seed back.
	now := time.Now()
	rule := baseRule()
	in := repository.HnREvalInput{
		Record: model.HnRRecord{
			CompletedAt: now, LastSeenAt: now,
			SeededSeconds: 0,
			Uploaded:      1000, // == RequiredRatio(1.0) * TorrentSize(1000)
		},
		Rule:        rule,
		TorrentSize: 1000,
	}
	if got := EvaluateHnRRecord(in, now); got != HnRStatusSatisfied {
		t.Errorf("got %s, want satisfied (met ratio exactly)", got)
	}
}

func TestEvaluateHnRRecord_MonitoringWithinGrace(t *testing.T) {
	now := time.Now()
	rule := baseRule()
	in := repository.HnREvalInput{
		Record: model.HnRRecord{
			CompletedAt: now.Add(-time.Hour),
			LastSeenAt:  now.Add(-time.Minute), // just announced, well within the 48h grace
		},
		Rule:        rule,
		TorrentSize: 1000,
	}
	if got := EvaluateHnRRecord(in, now); got != HnRStatusMonitoring {
		t.Errorf("got %s, want monitoring (unmet but within grace)", got)
	}
}

func TestEvaluateHnRRecord_BreachAfterInactivityGrace(t *testing.T) {
	now := time.Now()
	rule := baseRule()
	rule.InactivityGraceHours = 48
	in := repository.HnREvalInput{
		Record: model.HnRRecord{
			CompletedAt: now.Add(-49 * time.Hour),
			LastSeenAt:  now.Add(-49 * time.Hour), // stopped announcing 49h ago, grace is 48h
		},
		Rule:        rule,
		TorrentSize: 1000,
	}
	if got := EvaluateHnRRecord(in, now); got != HnRStatusBreach {
		t.Errorf("got %s, want breach (inactivity grace exceeded)", got)
	}
}

func TestEvaluateHnRRecord_ZeroGraceIsZeroTolerance(t *testing.T) {
	// Unlike MaxDaysToSatisfy, a grace of 0 is not "disabled" — it means
	// breach the instant a seeding announce is overdue.
	now := time.Now()
	rule := baseRule()
	rule.InactivityGraceHours = 0
	in := repository.HnREvalInput{
		Record: model.HnRRecord{
			CompletedAt: now.Add(-time.Minute),
			LastSeenAt:  now.Add(-time.Second), // one second of inactivity
		},
		Rule:        rule,
		TorrentSize: 1000,
	}
	if got := EvaluateHnRRecord(in, now); got != HnRStatusBreach {
		t.Errorf("got %s, want breach (zero grace tolerates no inactivity)", got)
	}
}

func TestEvaluateHnRRecord_BreachAtHardCapEvenIfStillWithinGrace(t *testing.T) {
	now := time.Now()
	rule := baseRule()
	rule.InactivityGraceHours = 48
	rule.MaxDaysToSatisfy = 30
	in := repository.HnREvalInput{
		Record: model.HnRRecord{
			CompletedAt: now.Add(-31 * 24 * time.Hour), // 31 days since completion, cap is 30
			LastSeenAt:  now.Add(-time.Minute),         // actively seeding right now (within grace)
		},
		Rule:        rule,
		TorrentSize: 1000,
	}
	if got := EvaluateHnRRecord(in, now); got != HnRStatusBreach {
		t.Errorf("got %s, want breach (hard cap exceeded, regardless of recent activity)", got)
	}
}

func TestEvaluateHnRRecord_NoHardCapWhenZero(t *testing.T) {
	now := time.Now()
	rule := baseRule()
	rule.MaxDaysToSatisfy = 0 // 0 = no hard cap
	rule.InactivityGraceHours = 48
	in := repository.HnREvalInput{
		Record: model.HnRRecord{
			CompletedAt: now.Add(-365 * 24 * time.Hour), // a year ago
			LastSeenAt:  now.Add(-time.Minute),          // still actively seeding
		},
		Rule:        rule,
		TorrentSize: 1000,
	}
	if got := EvaluateHnRRecord(in, now); got != HnRStatusMonitoring {
		t.Errorf("got %s, want monitoring (no hard cap, still within inactivity grace)", got)
	}
}

func TestEvaluateHnRRecord_ZeroThresholdRuleIsAlwaysSatisfied(t *testing.T) {
	now := time.Now()
	rule := &model.HnRRule{RequiredSeedHours: 0, RequiredRatio: 0, InactivityGraceHours: 48, MaxDaysToSatisfy: 30}
	in := repository.HnREvalInput{
		Record:      model.HnRRecord{CompletedAt: now, LastSeenAt: now, SeededSeconds: 0, Uploaded: 0},
		Rule:        rule,
		TorrentSize: 1000,
	}
	if got := EvaluateHnRRecord(in, now); got != HnRStatusSatisfied {
		t.Errorf("got %s, want satisfied (a rule with both thresholds at zero has no requirement)", got)
	}
}
