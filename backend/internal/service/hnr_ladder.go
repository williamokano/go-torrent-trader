package service

import (
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// decideHnRLadderStage is the ladder's one decision function, mirroring
// EvaluateHnRRecord and decidePromotionTarget: a pure function over
// already-loaded state so the daemon (which persists and acts on the
// result) and its tests never re-derive the arithmetic separately.
//
// Escalation only ever advances one stage per call, gated by the *next*
// stage's MinDaysInPrev dwell against how long the user has sat at their
// current stage — this is the ladder's leniency: a user whose active count
// jumps straight past several thresholds in one run still climbs one rung
// at a time, never straight to a harsher stage. De-escalation has no such
// gate and can drop directly to the stage matching the live count, in one
// step, however many rungs that spans — improving is never rationed.
//
// stages need not be sorted; every match is considered. current.Stage may
// name a stage no longer configured (deleted by an admin after a user
// reached it) — decideHnRLadderStage stalls rather than guessing in that
// case, since there is nothing to advance into.
func decideHnRLadderStage(stages []model.HnRPenaltyStage, activeCount int, current model.HnRUserState, now time.Time) (newStage int, changed bool) {
	target := 0
	for _, st := range stages {
		if activeCount >= st.MinActiveHnR && st.Stage > target {
			target = st.Stage
		}
	}

	if target < current.Stage {
		return target, true
	}
	if target == current.Stage {
		return current.Stage, false
	}

	// Escalating: advance at most one stage, gated by that stage's own dwell
	// requirement against the current stage's entry time.
	next := current.Stage + 1
	if target < next {
		// The live count doesn't even support the very next rung yet.
		return current.Stage, false
	}
	var nextRow *model.HnRPenaltyStage
	for i := range stages {
		if stages[i].Stage == next {
			nextRow = &stages[i]
			break
		}
	}
	if nextRow == nil {
		// No stage configured at exactly current+1 (a gap in the ladder, or
		// current itself no longer exists) — nothing to advance into.
		return current.Stage, false
	}
	dwellDeadline := current.StageEnteredAt.AddDate(0, 0, nextRow.MinDaysInPrev)
	if now.Before(dwellDeadline) {
		return current.Stage, false
	}
	return next, true
}
