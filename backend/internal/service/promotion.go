package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// promotionSeedGapCapSeconds caps each announce-to-announce gap counted as seed
// time, so an offline stretch between two seeding announces is not billed as
// continuous seeding. Two announce intervals is a generous ceiling.
const promotionSeedGapCapSeconds = 2 * DefaultInterval

// Promotion errors are value errors (bad admin input), distinct from storage failures.
var (
	ErrPromotionGroupNotFound    = fmt.Errorf("group not found")
	ErrPromotionStaffGroup       = fmt.Errorf("staff groups cannot be part of the promotion ladder")
	ErrPromotionRuleNotFound     = fmt.Errorf("promotion rule not found")
	ErrPromotionInvalidThreshold = fmt.Errorf("promotion thresholds must be zero or positive")
)

// PromotionRuleInput is the admin-supplied threshold set for one class.
type PromotionRuleInput struct {
	MinRatio     float64 `json:"min_ratio"`
	MinUploaded  int64   `json:"min_uploaded"`
	MinAgeDays   int     `json:"min_age_days"`
	MinTorrents  int     `json:"min_torrents"`
	MinSeedHours int     `json:"min_seed_hours"`
}

// PromotionRuleView is a rule joined with its group, for the admin UI. The
// ladder is these views ordered by group level.
type PromotionRuleView struct {
	GroupID      int64   `json:"group_id"`
	GroupName    string  `json:"group_name"`
	GroupLevel   int     `json:"group_level"`
	IsStaff      bool    `json:"is_staff"`
	MinRatio     float64 `json:"min_ratio"`
	MinUploaded  int64   `json:"min_uploaded"`
	MinAgeDays   int     `json:"min_age_days"`
	MinTorrents  int     `json:"min_torrents"`
	MinSeedHours int     `json:"min_seed_hours"`
}

// PromotionChange records one user's move during a run.
type PromotionChange struct {
	UserID    int64
	FromGroup int64
	ToGroup   int64
	Promoted  bool
}

// PromotionRunSummary is the outcome of a Run.
type PromotionRunSummary struct {
	Skipped  bool
	Reason   string
	Promoted int
	Demoted  int
	Changes  []PromotionChange
}

// PromotionService runs the auto class promotion/demotion engine and manages its rules.
type PromotionService struct {
	promotions repository.PromotionRepository
	groups     repository.GroupRepository
	settings   *SiteSettingsService
}

// NewPromotionService creates a new PromotionService.
func NewPromotionService(promotions repository.PromotionRepository, groups repository.GroupRepository, settings *SiteSettingsService) *PromotionService {
	return &PromotionService{promotions: promotions, groups: groups, settings: settings}
}

// ListRules returns every rule joined with its group, ordered by level (ladder order).
func (s *PromotionService) ListRules(ctx context.Context) ([]PromotionRuleView, error) {
	rules, err := s.promotions.ListRules(ctx)
	if err != nil {
		return nil, err
	}
	groups, err := s.groups.List(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]model.Group, len(groups))
	for _, g := range groups {
		byID[g.ID] = g
	}

	views := make([]PromotionRuleView, 0, len(rules))
	for _, r := range rules {
		g, ok := byID[r.GroupID]
		if !ok {
			continue // group vanished; rule will be cascaded away
		}
		views = append(views, PromotionRuleView{
			GroupID:      r.GroupID,
			GroupName:    g.Name,
			GroupLevel:   g.Level,
			IsStaff:      g.IsAdmin || g.IsModerator,
			MinRatio:     r.MinRatio,
			MinUploaded:  r.MinUploaded,
			MinAgeDays:   r.MinAgeDays,
			MinTorrents:  r.MinTorrents,
			MinSeedHours: r.MinSeedHours,
		})
	}
	sort.Slice(views, func(i, j int) bool {
		if views[i].GroupLevel != views[j].GroupLevel {
			return views[i].GroupLevel < views[j].GroupLevel
		}
		return views[i].GroupID < views[j].GroupID
	})
	return views, nil
}

// UpsertRule creates or updates a class's promotion rule, refusing staff groups
// and negative thresholds.
func (s *PromotionService) UpsertRule(ctx context.Context, groupID int64, in PromotionRuleInput) (*model.PromotionRule, error) {
	group, err := s.groups.GetByID(ctx, groupID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPromotionGroupNotFound
		}
		return nil, fmt.Errorf("get group: %w", err)
	}
	if group.IsAdmin || group.IsModerator {
		return nil, ErrPromotionStaffGroup
	}
	if in.MinRatio < 0 || in.MinUploaded < 0 || in.MinAgeDays < 0 || in.MinTorrents < 0 || in.MinSeedHours < 0 {
		return nil, ErrPromotionInvalidThreshold
	}

	rule := &model.PromotionRule{
		GroupID:      groupID,
		MinRatio:     in.MinRatio,
		MinUploaded:  in.MinUploaded,
		MinAgeDays:   in.MinAgeDays,
		MinTorrents:  in.MinTorrents,
		MinSeedHours: in.MinSeedHours,
	}
	if err := s.promotions.UpsertRule(ctx, rule); err != nil {
		return nil, fmt.Errorf("upsert promotion rule: %w", err)
	}
	return rule, nil
}

// DeleteRule removes a class from the ladder.
func (s *PromotionService) DeleteRule(ctx context.Context, groupID int64) error {
	if err := s.promotions.DeleteRule(ctx, groupID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPromotionRuleNotFound
		}
		return fmt.Errorf("delete promotion rule: %w", err)
	}
	return nil
}

// Run evaluates every ladder user and moves each at most one rung. It is a no-op
// when disabled. The scheduled job passes force=false so it also no-ops until
// the configured interval elapses; a manual admin trigger passes force=true to
// evaluate new rules immediately regardless of the interval.
func (s *PromotionService) Run(ctx context.Context, force bool) (PromotionRunSummary, error) {
	now := time.Now()

	if !s.settings.GetBool(ctx, SettingPromotionEnabled, false) {
		return PromotionRunSummary{Skipped: true, Reason: "disabled"}, nil
	}

	if !force {
		intervalDays := s.settings.GetInt(ctx, SettingPromotionIntervalDays, 7)
		if last, ok, err := s.promotions.LastRunAt(ctx); err != nil {
			return PromotionRunSummary{}, err
		} else if ok && intervalDays > 0 && now.Sub(last) < time.Duration(intervalDays)*24*time.Hour {
			return PromotionRunSummary{Skipped: true, Reason: "interval not elapsed"}, nil
		}
	}

	ladder, ruleByGroup, groupByID, err := s.buildLadder(ctx)
	if err != nil {
		return PromotionRunSummary{}, err
	}
	if len(ladder) < 2 {
		// Nothing to move between; still record the run so the interval advances.
		if err := s.promotions.RecordRun(ctx, 0, 0); err != nil {
			return PromotionRunSummary{}, err
		}
		return PromotionRunSummary{Reason: "ladder has fewer than two rungs"}, nil
	}

	metrics, err := s.promotions.LadderMetrics(ctx, ladder)
	if err != nil {
		return PromotionRunSummary{}, err
	}
	seedWindow := s.settings.GetInt(ctx, SettingPromotionSeedWindowDays, 30)
	seedHours, err := s.promotions.SeedHoursByUser(ctx, now.AddDate(0, 0, -seedWindow), promotionSeedGapCapSeconds)
	if err != nil {
		return PromotionRunSummary{}, err
	}

	indexByGroup := make(map[int64]int, len(ladder))
	for i, gid := range ladder {
		indexByGroup[gid] = i
	}

	var summary PromotionRunSummary
	for i := range metrics {
		m := metrics[i]
		m.SeedHours = seedHours[m.UserID]

		idx, ok := indexByGroup[m.GroupID]
		if !ok {
			continue
		}
		target, promoted, moved := decidePromotionTarget(ladder, ruleByGroup, idx, m, now)
		if !moved {
			continue
		}

		// Belt-and-suspenders: never move a user into a staff class even if a
		// rule was somehow attached to one.
		if tg, ok := groupByID[target]; ok && (tg.IsAdmin || tg.IsModerator) {
			slog.Warn("promotion: refusing to move user into staff group",
				"user_id", m.UserID, "target_group", target)
			continue
		}

		if err := s.promotions.SetUserGroup(ctx, m.UserID, target); err != nil {
			slog.Error("promotion: failed to move user", "user_id", m.UserID, "target_group", target, "error", err)
			continue
		}
		summary.Changes = append(summary.Changes, PromotionChange{
			UserID: m.UserID, FromGroup: m.GroupID, ToGroup: target, Promoted: promoted,
		})
		if promoted {
			summary.Promoted++
		} else {
			summary.Demoted++
		}
	}

	if err := s.promotions.RecordRun(ctx, summary.Promoted, summary.Demoted); err != nil {
		return summary, err
	}
	slog.Info("promotion run complete", "promoted", summary.Promoted, "demoted", summary.Demoted)
	return summary, nil
}

// buildLadder returns the ordered ladder group IDs (ascending level), the rule
// for each ladder group, and a lookup of every group. Staff groups are excluded
// from the ladder even if they somehow carry a rule.
func (s *PromotionService) buildLadder(ctx context.Context) ([]int64, map[int64]model.PromotionRule, map[int64]model.Group, error) {
	rules, err := s.promotions.ListRules(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	groups, err := s.groups.List(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	groupByID := make(map[int64]model.Group, len(groups))
	for _, g := range groups {
		groupByID[g.ID] = g
	}

	ruleByGroup := make(map[int64]model.PromotionRule, len(rules))
	var ladder []int64
	for _, r := range rules {
		g, ok := groupByID[r.GroupID]
		if !ok || g.IsAdmin || g.IsModerator {
			continue
		}
		ruleByGroup[r.GroupID] = r
		ladder = append(ladder, r.GroupID)
	}
	sort.Slice(ladder, func(i, j int) bool {
		gi, gj := groupByID[ladder[i]], groupByID[ladder[j]]
		if gi.Level != gj.Level {
			return gi.Level < gj.Level
		}
		return gi.ID < gj.ID
	})
	return ladder, ruleByGroup, groupByID, nil
}

// decidePromotionTarget applies the one-step ladder rule for a user at rung idx:
// promote if they meet the next rung's requirement, else demote if they no
// longer meet their current rung's requirement (never below the floor).
func decidePromotionTarget(ladder []int64, ruleByGroup map[int64]model.PromotionRule, idx int, m model.PromotionUserMetrics, now time.Time) (target int64, promoted bool, moved bool) {
	if idx+1 < len(ladder) {
		if promotionMeets(ruleByGroup[ladder[idx+1]], m, now) {
			return ladder[idx+1], true, true
		}
	}
	if idx > 0 {
		if !promotionMeets(ruleByGroup[ladder[idx]], m, now) {
			return ladder[idx-1], false, true
		}
	}
	return m.GroupID, false, false
}

// promotionMeets reports whether the user's metrics satisfy every non-zero
// threshold on the rule. A zero threshold is "no constraint".
func promotionMeets(rule model.PromotionRule, m model.PromotionUserMetrics, now time.Time) bool {
	if rule.MinRatio > 0 && ratioValue(m.Uploaded, m.Downloaded) < rule.MinRatio {
		return false
	}
	if m.Uploaded < rule.MinUploaded {
		return false
	}
	if rule.MinAgeDays > 0 {
		if int(now.Sub(m.CreatedAt).Hours()/24) < rule.MinAgeDays {
			return false
		}
	}
	if m.Torrents < rule.MinTorrents {
		return false
	}
	if m.SeedHours < rule.MinSeedHours {
		return false
	}
	return true
}

// ratioValue returns uploaded/downloaded, treating a zero-download account with
// any upload as effectively infinite so it clears any finite ratio requirement.
func ratioValue(uploaded, downloaded int64) float64 {
	if downloaded == 0 {
		if uploaded == 0 {
			return 0
		}
		return float64(1 << 62) // effectively infinite for threshold comparison
	}
	return float64(uploaded) / float64(downloaded)
}
