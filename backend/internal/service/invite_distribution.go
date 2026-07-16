package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// Invite distribution errors are value errors (bad admin input), distinct
// from storage failures.
var (
	ErrInviteDistGroupNotFound    = errors.New("group not found")
	ErrInviteDistStaffGroup       = errors.New("staff groups cannot receive automatic invite grants")
	ErrInviteDistRuleNotFound     = errors.New("invite distribution rule not found")
	ErrInviteDistInvalidThreshold = errors.New("invite distribution thresholds must be zero or positive")
	ErrInviteDistInvalidRange     = errors.New("min downloaded must not exceed max downloaded when max is set")
)

// InviteDistributionRuleInput is the admin-supplied policy for one group.
type InviteDistributionRuleInput struct {
	MinRatio           float64 `json:"min_ratio"`
	MinDownloadedBytes int64   `json:"min_downloaded_bytes"`
	MaxDownloadedBytes int64   `json:"max_downloaded_bytes"`
	MaxInvites         int     `json:"max_invites"`
}

// InviteDistributionRuleView is a rule joined with its group, for the admin UI.
type InviteDistributionRuleView struct {
	GroupID            int64   `json:"group_id"`
	GroupName          string  `json:"group_name"`
	GroupLevel         int     `json:"group_level"`
	IsStaff            bool    `json:"is_staff"`
	MinRatio           float64 `json:"min_ratio"`
	MinDownloadedBytes int64   `json:"min_downloaded_bytes"`
	MaxDownloadedBytes int64   `json:"max_downloaded_bytes"`
	MaxInvites         int     `json:"max_invites"`
}

// InviteDistributionGrant records one user's auto-grant during a run.
type InviteDistributionGrant struct {
	UserID  int64
	GroupID int64
}

// InviteDistributionRunSummary is the outcome of a Run.
type InviteDistributionRunSummary struct {
	Skipped bool
	Reason  string
	Granted int
	Grants  []InviteDistributionGrant
}

// InviteDistributionService runs the auto invite distribution engine and
// manages its rules, mirroring PromotionService's shape.
type InviteDistributionService struct {
	rules    repository.InviteDistributionRepository
	groups   repository.GroupRepository
	users    inviteAdjustRepository
	settings *SiteSettingsService
	eventBus event.Bus
}

// NewInviteDistributionService creates a new InviteDistributionService.
func NewInviteDistributionService(
	rules repository.InviteDistributionRepository,
	groups repository.GroupRepository,
	users inviteAdjustRepository,
	settings *SiteSettingsService,
	bus event.Bus,
) *InviteDistributionService {
	return &InviteDistributionService{rules: rules, groups: groups, users: users, settings: settings, eventBus: bus}
}

// ListRules returns every rule joined with its group, ordered by level.
func (s *InviteDistributionService) ListRules(ctx context.Context) ([]InviteDistributionRuleView, error) {
	rules, err := s.rules.ListRules(ctx)
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

	views := make([]InviteDistributionRuleView, 0, len(rules))
	for _, r := range rules {
		g, ok := byID[r.GroupID]
		if !ok {
			continue // group vanished; rule will be cascaded away
		}
		views = append(views, InviteDistributionRuleView{
			GroupID:            r.GroupID,
			GroupName:          g.Name,
			GroupLevel:         g.Level,
			IsStaff:            g.IsAdmin || g.IsModerator,
			MinRatio:           r.MinRatio,
			MinDownloadedBytes: r.MinDownloadedBytes,
			MaxDownloadedBytes: r.MaxDownloadedBytes,
			MaxInvites:         r.MaxInvites,
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

// UpsertRule creates or updates a group's invite distribution policy,
// refusing staff groups and invalid thresholds.
func (s *InviteDistributionService) UpsertRule(ctx context.Context, groupID int64, in InviteDistributionRuleInput) (*model.InviteDistributionRule, error) {
	group, err := s.groups.GetByID(ctx, groupID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInviteDistGroupNotFound
		}
		return nil, fmt.Errorf("get group: %w", err)
	}
	if group.IsAdmin || group.IsModerator {
		return nil, ErrInviteDistStaffGroup
	}
	if in.MinRatio < 0 || in.MinDownloadedBytes < 0 || in.MaxDownloadedBytes < 0 || in.MaxInvites < 0 {
		return nil, ErrInviteDistInvalidThreshold
	}
	if in.MaxDownloadedBytes > 0 && in.MinDownloadedBytes > in.MaxDownloadedBytes {
		return nil, ErrInviteDistInvalidRange
	}

	rule := &model.InviteDistributionRule{
		GroupID:            groupID,
		MinRatio:           in.MinRatio,
		MinDownloadedBytes: in.MinDownloadedBytes,
		MaxDownloadedBytes: in.MaxDownloadedBytes,
		MaxInvites:         in.MaxInvites,
	}
	if err := s.rules.UpsertRule(ctx, rule); err != nil {
		return nil, fmt.Errorf("upsert invite distribution rule: %w", err)
	}
	return rule, nil
}

// DeleteRule removes a group's invite distribution policy.
func (s *InviteDistributionService) DeleteRule(ctx context.Context, groupID int64) error {
	if err := s.rules.DeleteRule(ctx, groupID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInviteDistRuleNotFound
		}
		return fmt.Errorf("delete invite distribution rule: %w", err)
	}
	return nil
}

// Run evaluates every user in a ruled group and grants +1 invite to each
// eligible one. It is a no-op when disabled. The scheduled job passes
// force=false so it also no-ops until the configured interval elapses; a
// manual admin trigger passes force=true to evaluate immediately.
func (s *InviteDistributionService) Run(ctx context.Context, force bool) (InviteDistributionRunSummary, error) {
	now := time.Now()

	if !s.settings.GetBool(ctx, SettingInviteDistributionEnabled, false) {
		return InviteDistributionRunSummary{Skipped: true, Reason: "disabled"}, nil
	}

	if !force {
		intervalDays := s.settings.GetInt(ctx, SettingInviteDistributionIntervalDays, 7)
		if last, ok, err := s.rules.LastRunAt(ctx); err != nil {
			return InviteDistributionRunSummary{}, err
		} else if ok && intervalDays > 0 && now.Sub(last) < time.Duration(intervalDays)*24*time.Hour {
			return InviteDistributionRunSummary{Skipped: true, Reason: "interval not elapsed"}, nil
		}
	}

	rules, err := s.rules.ListRules(ctx)
	if err != nil {
		return InviteDistributionRunSummary{}, err
	}
	if len(rules) == 0 {
		// Nothing configured; still record the run so the interval advances.
		if err := s.rules.RecordRun(ctx, 0); err != nil {
			return InviteDistributionRunSummary{}, err
		}
		return InviteDistributionRunSummary{Reason: "no rules configured"}, nil
	}

	ruleByGroup := make(map[int64]model.InviteDistributionRule, len(rules))
	groupIDs := make([]int64, 0, len(rules))
	for _, r := range rules {
		ruleByGroup[r.GroupID] = r
		groupIDs = append(groupIDs, r.GroupID)
	}

	metrics, err := s.rules.GroupMetrics(ctx, groupIDs)
	if err != nil {
		return InviteDistributionRunSummary{}, err
	}
	if len(metrics) == 0 {
		if err := s.rules.RecordRun(ctx, 0); err != nil {
			return InviteDistributionRunSummary{}, err
		}
		return InviteDistributionRunSummary{Reason: "no eligible users"}, nil
	}

	userIDs := make([]int64, len(metrics))
	for i, m := range metrics {
		userIDs[i] = m.UserID
	}
	states, err := s.rules.UserInviteStates(ctx, userIDs)
	if err != nil {
		return InviteDistributionRunSummary{}, err
	}

	var summary InviteDistributionRunSummary
	for _, m := range metrics {
		rule, ok := ruleByGroup[m.GroupID]
		if !ok {
			continue
		}
		state, ok := states[m.UserID]
		if !ok {
			continue // vanished between the metrics snapshot and this lookup
		}

		// A suspended invite privilege pauses auto-grants too, mirroring how
		// outstanding invite tokens are already voided while can_invite is
		// false (BE-8.15/8.16): the restriction means "no invite activity for
		// this user," not just "no manual invite creation." Granting anyway
		// would let the balance quietly bank rewards during a punitive
		// suspension, available in full the moment it's lifted.
		if !state.CanInvite {
			continue
		}
		if state.Invites >= rule.MaxInvites {
			continue // already at (or past) this role's per-user ceiling
		}
		if !inviteDistributionMeets(rule, m) {
			continue
		}

		if err := s.users.AdjustInvites(ctx, m.UserID, 1); err != nil {
			slog.Error("invite distribution: failed to grant invite", "user_id", m.UserID, "error", err)
			continue
		}

		summary.Grants = append(summary.Grants, InviteDistributionGrant{UserID: m.UserID, GroupID: m.GroupID})
		summary.Granted++

		s.eventBus.Publish(ctx, &event.InviteAutoGrantedEvent{
			Base:     event.NewBase(event.InviteAutoGranted, event.Actor{ID: 0, Username: "System"}),
			UserID:   m.UserID,
			Username: state.Username,
		})
	}

	if err := s.rules.RecordRun(ctx, summary.Granted); err != nil {
		return summary, err
	}
	slog.Info("invite distribution run complete", "granted", summary.Granted)
	return summary, nil
}

// inviteDistributionMeets reports whether the user's metrics satisfy a rule's
// eligibility criteria. MinRatio of 0 and MinDownloadedBytes of 0 are "no
// constraint" (PromotionRule's convention); MaxDownloadedBytes of 0 is
// unbounded. ratioValue is shared with the promotion engine (promotion.go).
func inviteDistributionMeets(rule model.InviteDistributionRule, m model.PromotionUserMetrics) bool {
	if rule.MinRatio > 0 && ratioValue(m.Uploaded, m.Downloaded) < rule.MinRatio {
		return false
	}
	if m.Downloaded < rule.MinDownloadedBytes {
		return false
	}
	if rule.MaxDownloadedBytes > 0 && m.Downloaded > rule.MaxDownloadedBytes {
		return false
	}
	return true
}
