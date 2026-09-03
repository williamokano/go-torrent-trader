package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// hnrValidActions is the closed set of penalty-stage actions — the daemon's
// escalate switch below is exhaustive over exactly this set.
var hnrValidActions = map[string]bool{
	model.HnRActionNotify:      true,
	model.HnRActionWarn:        true,
	model.HnRActionRestrict:    true,
	model.HnRActionFinalNotice: true,
	model.HnRActionBan:         true,
}

// HnRStageInput is the admin-supplied definition of one penalty ladder rung.
type HnRStageInput struct {
	MinActiveHnR     int      `json:"min_active_hnr"`
	MinDaysInPrev    int      `json:"min_days_in_prev"`
	Action           string   `json:"action"`
	RestrictionTypes []string `json:"restriction_types"`
	RestrictionDays  int      `json:"restriction_days"`
	MessageTemplate  string   `json:"message_template"`
}

// ListStages returns the whole ladder, ordered by stage.
func (s *HnRService) ListStages(ctx context.Context) ([]model.HnRPenaltyStage, error) {
	stages, err := s.hnr.ListStages(ctx)
	if err != nil {
		return nil, fmt.Errorf("list hnr stages: %w", err)
	}
	sort.Slice(stages, func(i, j int) bool { return stages[i].Stage < stages[j].Stage })
	return stages, nil
}

// UpsertStage creates or updates one rung of the ladder. stage must be >= 1
// (stage 0 is reserved for "off the ladder" and is never a configured row).
func (s *HnRService) UpsertStage(ctx context.Context, stage int, in HnRStageInput) (*model.HnRPenaltyStage, error) {
	if stage < 1 {
		return nil, fmt.Errorf("%w: stage must be at least 1", ErrHnRInvalidStage)
	}
	if in.MinActiveHnR < 1 {
		return nil, fmt.Errorf("%w: min_active_hnr must be at least 1", ErrHnRInvalidStage)
	}
	if in.MinDaysInPrev < 0 {
		return nil, fmt.Errorf("%w: min_days_in_prev must be zero or positive", ErrHnRInvalidStage)
	}
	if in.RestrictionDays < 0 {
		return nil, fmt.Errorf("%w: restriction_days must be zero or positive", ErrHnRInvalidStage)
	}
	if !hnrValidActions[in.Action] {
		return nil, fmt.Errorf("%w: unknown action %q", ErrHnRInvalidStage, in.Action)
	}
	if in.Action == model.HnRActionRestrict {
		if len(in.RestrictionTypes) == 0 {
			return nil, fmt.Errorf("%w: restrict stages need at least one restriction type", ErrHnRInvalidStage)
		}
		for _, rtype := range in.RestrictionTypes {
			if !isValidRestrictionType(rtype) {
				return nil, fmt.Errorf("%w: unknown restriction type %q", ErrHnRInvalidStage, rtype)
			}
		}
	} else if len(in.RestrictionTypes) > 0 {
		return nil, fmt.Errorf("%w: restriction_types only applies to the restrict action", ErrHnRInvalidStage)
	}

	row := &model.HnRPenaltyStage{
		Stage:            stage,
		MinActiveHnR:     in.MinActiveHnR,
		MinDaysInPrev:    in.MinDaysInPrev,
		Action:           in.Action,
		RestrictionTypes: in.RestrictionTypes,
		RestrictionDays:  in.RestrictionDays,
		MessageTemplate:  in.MessageTemplate,
	}
	if err := s.hnr.UpsertStage(ctx, row); err != nil {
		return nil, fmt.Errorf("upsert hnr stage: %w", err)
	}
	return row, nil
}

// DeleteStage removes one rung. A user currently sitting at the deleted
// stage is left alone until the next run re-evaluates them — see
// decideHnRLadderStage's handling of a stage no longer configured.
func (s *HnRService) DeleteStage(ctx context.Context, stage int) error {
	if err := s.hnr.DeleteStage(ctx, stage); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrHnRStageNotFound
		}
		return fmt.Errorf("delete hnr stage: %w", err)
	}
	return nil
}

// runLadder is the daemon's per-run pass over the penalty ladder: every user
// who currently has an open ('hnr'-state) record, plus every user already on
// the ladder (so a user whose count has fallen to zero still de-escalates),
// gets one decideHnRLadderStage call against their live count. Returns how
// many users advanced and how many decayed, for the run log.
func (s *HnRService) runLadder(ctx context.Context, now time.Time) (advanced, decayed int, err error) {
	stages, err := s.hnr.ListStages(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("list stages: %w", err)
	}
	if len(stages) == 0 {
		return 0, 0, nil // ladder not configured; nothing to evaluate
	}

	counts, err := s.hnr.ActiveHnRCounts(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("active hnr counts: %w", err)
	}
	onLadder, err := s.hnr.UsersOnLadder(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("users on ladder: %w", err)
	}

	candidates := make(map[int64]struct{}, len(counts)+len(onLadder))
	for uid := range counts {
		candidates[uid] = struct{}{}
	}
	for _, st := range onLadder {
		candidates[st.UserID] = struct{}{}
	}

	for uid := range candidates {
		didAdvance, didDecay := s.evaluateUserLadderStage(ctx, uid, stages, counts[uid], now)
		if didAdvance {
			advanced++
		}
		if didDecay {
			decayed++
		}
	}
	return advanced, decayed, nil
}

// evaluateUserLadderStage is one user's step of runLadder's loop, factored
// out so a member clearing an obligation with points (HnRService.ClearRecord)
// can re-evaluate just their own position immediately — through this exact
// function, never a re-derived copy — instead of waiting for the next
// scheduled sweep to lift a restriction paying off just earned them.
func (s *HnRService) evaluateUserLadderStage(ctx context.Context, userID int64, stages []model.HnRPenaltyStage, activeCount int, now time.Time) (advanced, decayed bool) {
	if err := s.hnr.EnsureUserState(ctx, userID, now); err != nil {
		slog.Error("hnr ladder: ensure user state", "user_id", userID, "error", err)
		return false, false
	}
	state, err := s.hnr.GetUserState(ctx, userID)
	if err != nil {
		slog.Error("hnr ladder: get user state", "user_id", userID, "error", err)
		return false, false
	}

	newStage, changed := decideHnRLadderStage(stages, activeCount, *state, now)
	if !changed {
		return false, false
	}

	ok, err := s.hnr.CASUserStage(ctx, userID, state.Stage, newStage, now)
	if err != nil {
		slog.Error("hnr ladder: cas user stage", "user_id", userID, "error", err)
		return false, false
	}
	if !ok {
		// Another instance already moved this user this run — the CAS is
		// what makes a double-run safe, exactly like MarkBreached.
		return false, false
	}

	if newStage > state.Stage {
		s.escalate(ctx, userID, state.Stage, newStage, stages, activeCount)
		return true, false
	}
	s.deescalate(ctx, userID, state.Stage, newStage, stages)
	return false, true
}

// reevaluateLadderForUser re-runs the ladder decision for exactly one user,
// against a freshly-loaded ladder and active-hnr count — what
// HnRService.ClearRecord calls right after a successful clear, so a
// restriction paid off with points lifts in the same request rather than
// waiting for the next scheduled run. A no-op (not an error) when the
// ladder is unconfigured, matching runLadder.
func (s *HnRService) reevaluateLadderForUser(ctx context.Context, userID int64, now time.Time) error {
	stages, err := s.hnr.ListStages(ctx)
	if err != nil {
		return fmt.Errorf("list stages: %w", err)
	}
	if len(stages) == 0 {
		return nil
	}
	counts, err := s.hnr.ActiveHnRCounts(ctx)
	if err != nil {
		return fmt.Errorf("active hnr counts: %w", err)
	}
	s.evaluateUserLadderStage(ctx, userID, stages, counts[userID], now)
	return nil
}

// escalate executes the newly-entered stage's configured action and always
// publishes the stage-change notification, regardless of action — a bare
// "notify" stage's entire effect IS that notification.
func (s *HnRService) escalate(ctx context.Context, userID int64, oldStage, newStage int, stages []model.HnRPenaltyStage, activeCount int) {
	row := findStage(stages, newStage)
	if row == nil {
		return
	}
	username := s.usernameFor(ctx, userID)
	message := ReplaceTemplateVars(row.MessageTemplate, map[string]string{
		"username":         username,
		"stage":            strconv.Itoa(newStage),
		"count":            strconv.Itoa(activeCount),
		"restriction_days": strconv.Itoa(row.RestrictionDays),
	})

	switch row.Action {
	case model.HnRActionWarn:
		if s.warnings != nil {
			if _, err := s.warnings.IssueHnRWarning(ctx, userID, message); err != nil {
				slog.Error("hnr ladder: issue warning", "user_id", userID, "stage", newStage, "error", err)
			}
		}
	case model.HnRActionRestrict:
		if s.restrictions != nil {
			var expiresAt *time.Time
			if row.RestrictionDays > 0 {
				t := time.Now().AddDate(0, 0, row.RestrictionDays)
				expiresAt = &t
			}
			for _, rtype := range row.RestrictionTypes {
				if _, err := s.restrictions.ApplyRestriction(ctx, userID, rtype, message, model.RestrictionSourceHnR, expiresAt, nil); err != nil {
					slog.Error("hnr ladder: apply restriction", "user_id", userID, "stage", newStage, "type", rtype, "error", err)
				}
			}
		}
	case model.HnRActionBan:
		if s.warnings != nil {
			if err := s.warnings.IssueHnRBan(ctx, userID, message); err != nil {
				slog.Error("hnr ladder: issue ban", "user_id", userID, "stage", newStage, "error", err)
			}
		}
	case model.HnRActionNotify, model.HnRActionFinalNotice:
		// The notification published below is the entire effect.
	}

	s.notifyStageChange(ctx, userID, username, oldStage, newStage, row.Action, message)
	if err := s.hnr.SetLastNotifiedStage(ctx, userID, newStage); err != nil {
		slog.Error("hnr ladder: set last notified stage", "user_id", userID, "error", err)
	}
}

// deescalate lifts every restriction type any 'restrict' stage could have
// applied. Lifting a type the user never actually had from this source is a
// safe no-op (LiftActiveBySource, see PR1) — this is simpler and just as
// correct as tracking exactly which stages a user passed through, since a
// user can only ever have accumulated HnR-sourced restrictions from stages
// they actually reached. A ban is never undone here: the "ban" action
// disables the account outright, and only staff reverses that, the same as
// a ratio ban.
func (s *HnRService) deescalate(ctx context.Context, userID int64, oldStage, newStage int, stages []model.HnRPenaltyStage) {
	username := s.usernameFor(ctx, userID)
	if s.restrictions != nil {
		for _, rtype := range restrictionTypesAcrossStages(stages) {
			if _, err := s.restrictions.LiftActiveBySource(ctx, userID, rtype, model.RestrictionSourceHnR, nil); err != nil {
				slog.Error("hnr ladder: lift restriction", "user_id", userID, "type", rtype, "error", err)
			}
		}
	}

	message := fmt.Sprintf("Your active hit-and-run count has dropped: you have been moved from stage %d to stage %d.", oldStage, newStage)
	if newStage == 0 {
		message = "Your active hit-and-run count has dropped to zero: you have been taken off the penalty ladder."
	}
	s.notifyStageChange(ctx, userID, username, oldStage, newStage, "", message)
	if err := s.hnr.SetLastNotifiedStage(ctx, userID, newStage); err != nil {
		slog.Error("hnr ladder: set last notified stage", "user_id", userID, "error", err)
	}
}

func (s *HnRService) notifyStageChange(ctx context.Context, userID int64, username string, oldStage, newStage int, action, message string) {
	if s.eventBus == nil {
		return
	}
	s.eventBus.Publish(ctx, &event.HnRStageChangedEvent{
		Base:     event.NewBase(event.HnRStageChanged, event.Actor{ID: 0, Username: "System"}),
		UserID:   userID,
		Username: username,
		OldStage: oldStage,
		NewStage: newStage,
		Action:   action,
		Message:  message,
	})
}

func (s *HnRService) usernameFor(ctx context.Context, userID int64) string {
	if s.users == nil {
		return ""
	}
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return ""
	}
	return u.Username
}

func findStage(stages []model.HnRPenaltyStage, stage int) *model.HnRPenaltyStage {
	for i := range stages {
		if stages[i].Stage == stage {
			return &stages[i]
		}
	}
	return nil
}

// restrictionTypesAcrossStages is the union of every restriction type any
// configured 'restrict' stage could apply — what deescalate must consider
// lifting, since a user descending the ladder may have accumulated
// restrictions from any stage they passed through on the way up.
func restrictionTypesAcrossStages(stages []model.HnRPenaltyStage) []string {
	seen := make(map[string]bool)
	var out []string
	for _, st := range stages {
		if st.Action != model.HnRActionRestrict {
			continue
		}
		for _, rtype := range st.RestrictionTypes {
			if !seen[rtype] {
				seen[rtype] = true
				out = append(out, rtype)
			}
		}
	}
	return out
}
