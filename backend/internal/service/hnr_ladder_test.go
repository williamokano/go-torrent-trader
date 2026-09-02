package service

import (
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

func ladderTestStages() []model.HnRPenaltyStage {
	return []model.HnRPenaltyStage{
		{Stage: 1, MinActiveHnR: 1, MinDaysInPrev: 0, Action: model.HnRActionNotify},
		{Stage: 2, MinActiveHnR: 2, MinDaysInPrev: 3, Action: model.HnRActionWarn},
		{Stage: 3, MinActiveHnR: 3, MinDaysInPrev: 5, Action: model.HnRActionRestrict},
		{Stage: 4, MinActiveHnR: 4, MinDaysInPrev: 7, Action: model.HnRActionFinalNotice},
		{Stage: 5, MinActiveHnR: 5, MinDaysInPrev: 3, Action: model.HnRActionBan},
	}
}

func TestDecideHnRLadderStage_OffLadderWithNoActiveHnRStaysOff(t *testing.T) {
	newStage, changed := decideHnRLadderStage(ladderTestStages(), 0, model.HnRUserState{Stage: 0}, time.Now())
	if changed || newStage != 0 {
		t.Errorf("expected no change at stage 0, got newStage=%d changed=%v", newStage, changed)
	}
}

func TestDecideHnRLadderStage_FirstBreachEntersStageOneImmediately(t *testing.T) {
	// Stage 1 has MinDaysInPrev=0, so a user entering the ladder right now
	// (StageEnteredAt=now, Stage=0) advances immediately — no dwell to serve
	// before the first, lightest rung.
	now := time.Now()
	current := model.HnRUserState{Stage: 0, StageEnteredAt: now}
	newStage, changed := decideHnRLadderStage(ladderTestStages(), 1, current, now)
	if !changed || newStage != 1 {
		t.Errorf("expected immediate advance to stage 1, got newStage=%d changed=%v", newStage, changed)
	}
}

func TestDecideHnRLadderStage_EscalationWaitsForDwell(t *testing.T) {
	now := time.Now()
	// Entered stage 1 just now; stage 2 requires 3 days in the previous
	// stage before advancing, so a count that already supports stage 2
	// must still wait.
	current := model.HnRUserState{Stage: 1, StageEnteredAt: now}
	newStage, changed := decideHnRLadderStage(ladderTestStages(), 2, current, now)
	if changed || newStage != 1 {
		t.Errorf("expected to stay at stage 1 during the dwell window, got newStage=%d changed=%v", newStage, changed)
	}

	later := now.AddDate(0, 0, 3)
	newStage, changed = decideHnRLadderStage(ladderTestStages(), 2, current, later)
	if !changed || newStage != 2 {
		t.Errorf("expected to advance to stage 2 once the dwell has elapsed, got newStage=%d changed=%v", newStage, changed)
	}
}

func TestDecideHnRLadderStage_EscalationNeverSkipsARung(t *testing.T) {
	// Active count jumps straight to 5 (ban-eligible by count alone) while
	// the user is still at stage 1 — must advance to stage 2 only, one rung
	// at a time, regardless of how far the count has run ahead.
	now := time.Now()
	current := model.HnRUserState{Stage: 1, StageEnteredAt: now.AddDate(0, 0, -10)}
	newStage, changed := decideHnRLadderStage(ladderTestStages(), 5, current, now)
	if !changed || newStage != 2 {
		t.Errorf("expected a single-rung advance to stage 2, got newStage=%d changed=%v", newStage, changed)
	}
}

func TestDecideHnRLadderStage_DeescalationDropsDirectlyToTarget(t *testing.T) {
	// At stage 4, active count has fallen all the way to 1 — de-escalation
	// is not rationed the way escalation is, so this drops straight to
	// stage 1 in one step.
	now := time.Now()
	current := model.HnRUserState{Stage: 4, StageEnteredAt: now}
	newStage, changed := decideHnRLadderStage(ladderTestStages(), 1, current, now)
	if !changed || newStage != 1 {
		t.Errorf("expected a direct drop to stage 1, got newStage=%d changed=%v", newStage, changed)
	}
}

func TestDecideHnRLadderStage_DeescalationToOffLadder(t *testing.T) {
	now := time.Now()
	current := model.HnRUserState{Stage: 2, StageEnteredAt: now}
	newStage, changed := decideHnRLadderStage(ladderTestStages(), 0, current, now)
	if !changed || newStage != 0 {
		t.Errorf("expected a drop to stage 0, got newStage=%d changed=%v", newStage, changed)
	}
}

func TestDecideHnRLadderStage_NoChangeWhenCountStillMatchesCurrentStage(t *testing.T) {
	now := time.Now()
	current := model.HnRUserState{Stage: 3, StageEnteredAt: now.AddDate(0, 0, -100)}
	// Count of 3 supports exactly stage 3 (not stage 4, which needs 4) —
	// nothing to do even though the dwell for stage 4 has long elapsed.
	newStage, changed := decideHnRLadderStage(ladderTestStages(), 3, current, now)
	if changed || newStage != 3 {
		t.Errorf("expected to stay at stage 3, got newStage=%d changed=%v", newStage, changed)
	}
}

func TestDecideHnRLadderStage_StallsWhenNextRungIsUnconfigured(t *testing.T) {
	// A ladder with a gap: stage 2 is missing entirely.
	stages := []model.HnRPenaltyStage{
		{Stage: 1, MinActiveHnR: 1, MinDaysInPrev: 0, Action: model.HnRActionNotify},
		{Stage: 3, MinActiveHnR: 3, MinDaysInPrev: 0, Action: model.HnRActionRestrict},
	}
	now := time.Now()
	current := model.HnRUserState{Stage: 1, StageEnteredAt: now.AddDate(0, 0, -30)}
	newStage, changed := decideHnRLadderStage(stages, 3, current, now)
	if changed || newStage != 1 {
		t.Errorf("expected to stall at stage 1 with no stage 2 configured, got newStage=%d changed=%v", newStage, changed)
	}
}

func TestDecideHnRLadderStage_DeescalationFromDeletedStageStillSettles(t *testing.T) {
	// current.Stage=6 no longer has a configured row (an admin deleted it) —
	// target from the live count (5, the highest configured) is less than
	// 6, so this is a de-escalation, which decideHnRLadderStage always
	// allows regardless of whether the row it's leaving still exists.
	now := time.Now()
	current := model.HnRUserState{Stage: 6, StageEnteredAt: now}
	newStage, changed := decideHnRLadderStage(ladderTestStages(), 5, current, now)
	if !changed || newStage != 5 {
		t.Errorf("expected to settle at stage 5 (the highest configured), got newStage=%d changed=%v", newStage, changed)
	}
}
