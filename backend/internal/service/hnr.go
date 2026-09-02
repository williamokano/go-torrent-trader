package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// HnR errors are value errors (bad admin input), distinct from storage failures.
var (
	ErrHnRGroupNotFound    = fmt.Errorf("group not found")
	ErrHnRStaffGroup       = fmt.Errorf("staff groups cannot be subject to hit-and-run tracking")
	ErrHnRRuleNotFound     = fmt.Errorf("hit-and-run rule not found")
	ErrHnRInvalidThreshold = fmt.Errorf("hit-and-run thresholds must be zero or positive")
	ErrHnRStageNotFound    = fmt.Errorf("hit-and-run penalty stage not found")
	ErrHnRInvalidStage     = fmt.Errorf("invalid hit-and-run penalty stage")
	ErrHnRRecordNotFound   = fmt.Errorf("hit-and-run record not found")
)

// HnRRuleInput is the admin-supplied threshold set for one class.
type HnRRuleInput struct {
	RequiredSeedHours    int     `json:"required_seed_hours"`
	RequiredRatio        float64 `json:"required_ratio"`
	InactivityGraceHours int     `json:"inactivity_grace_hours"`
	MaxDaysToSatisfy     int     `json:"max_days_to_satisfy"`
}

// HnRRuleView is a rule joined with its group, for the admin UI.
type HnRRuleView struct {
	GroupID              int64   `json:"group_id"`
	GroupName            string  `json:"group_name"`
	GroupLevel           int     `json:"group_level"`
	IsStaff              bool    `json:"is_staff"`
	RequiredSeedHours    int     `json:"required_seed_hours"`
	RequiredRatio        float64 `json:"required_ratio"`
	InactivityGraceHours int     `json:"inactivity_grace_hours"`
	MaxDaysToSatisfy     int     `json:"max_days_to_satisfy"`
}

// HnRService handles hit-and-run tracking business logic: per-class rule
// configuration, plus (built out across the feature's remaining pieces) the
// penalty ladder, the member-facing read path, and clearing with points. The
// announce-path accounting itself (CreateIfNotExists / Accumulate) is called
// directly by TrackerService against repository.HnRRepository, the same way
// it already talks to TransferHistoryRepository and AnnounceEventRepository
// — not routed through this service.
type HnRService struct {
	hnr      repository.HnRRepository
	groups   repository.GroupRepository
	settings *SiteSettingsService
}

// NewHnRService creates a new HnRService.
func NewHnRService(hnr repository.HnRRepository, groups repository.GroupRepository, settings *SiteSettingsService) *HnRService {
	return &HnRService{hnr: hnr, groups: groups, settings: settings}
}

// ListRules returns every rule joined with its group, ordered by level.
func (s *HnRService) ListRules(ctx context.Context) ([]HnRRuleView, error) {
	rules, err := s.hnr.ListRules(ctx)
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

	views := make([]HnRRuleView, 0, len(rules))
	for _, r := range rules {
		g, ok := byID[r.GroupID]
		if !ok {
			continue // group vanished; rule will be cascaded away
		}
		views = append(views, HnRRuleView{
			GroupID:              r.GroupID,
			GroupName:            g.Name,
			GroupLevel:           g.Level,
			IsStaff:              g.IsAdmin || g.IsModerator,
			RequiredSeedHours:    r.RequiredSeedHours,
			RequiredRatio:        r.RequiredRatio,
			InactivityGraceHours: r.InactivityGraceHours,
			MaxDaysToSatisfy:     r.MaxDaysToSatisfy,
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

// UpsertRule creates or updates a class's HnR rule, refusing staff groups and
// negative thresholds. A class with no rule is not subject to HnR at all —
// this is how "VIP has no hit-and-run" is expressed, with no special-case
// code anywhere else.
func (s *HnRService) UpsertRule(ctx context.Context, groupID int64, in HnRRuleInput) (*model.HnRRule, error) {
	group, err := s.groups.GetByID(ctx, groupID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrHnRGroupNotFound
		}
		return nil, fmt.Errorf("get group: %w", err)
	}
	if group.IsAdmin || group.IsModerator {
		return nil, ErrHnRStaffGroup
	}
	if in.RequiredSeedHours < 0 || in.RequiredRatio < 0 || in.InactivityGraceHours < 0 || in.MaxDaysToSatisfy < 0 {
		return nil, ErrHnRInvalidThreshold
	}

	rule := &model.HnRRule{
		GroupID:              groupID,
		RequiredSeedHours:    in.RequiredSeedHours,
		RequiredRatio:        in.RequiredRatio,
		InactivityGraceHours: in.InactivityGraceHours,
		MaxDaysToSatisfy:     in.MaxDaysToSatisfy,
	}
	if err := s.hnr.UpsertRule(ctx, rule); err != nil {
		return nil, fmt.Errorf("upsert hnr rule: %w", err)
	}
	return rule, nil
}

// DeleteRule removes a class from HnR tracking entirely.
func (s *HnRService) DeleteRule(ctx context.Context, groupID int64) error {
	if err := s.hnr.DeleteRule(ctx, groupID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrHnRRuleNotFound
		}
		return fmt.Errorf("delete hnr rule: %w", err)
	}
	return nil
}
