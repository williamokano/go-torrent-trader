package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// HnRExemptRuleInput is the admin-supplied definition of one auto-exemption
// criterion.
type HnRExemptRuleInput struct {
	Criterion string `json:"criterion"`
	Threshold int64  `json:"threshold"`
	Enabled   bool   `json:"enabled"`
}

func validHnRExemptCriterion(c string) bool {
	return c == model.HnRExemptCriterionMinSeeders || c == model.HnRExemptCriterionMaxSize
}

func (in HnRExemptRuleInput) validate() error {
	if !validHnRExemptCriterion(in.Criterion) {
		return fmt.Errorf("%w: unknown criterion %q", ErrHnRInvalidExemptRule, in.Criterion)
	}
	if in.Threshold < 0 {
		return fmt.Errorf("%w: threshold must be zero or positive", ErrHnRInvalidExemptRule)
	}
	// min_seeders with threshold 0 would match every torrent (seeders >= 0),
	// silently exempting the whole library from HnR. "Well seeded" needs at
	// least one seeder. max_size_bytes 0 is harmless — no torrent is 0 bytes.
	if in.Criterion == model.HnRExemptCriterionMinSeeders && in.Threshold < 1 {
		return fmt.Errorf("%w: min_seeders threshold must be at least 1", ErrHnRInvalidExemptRule)
	}
	return nil
}

// ListExemptRules returns every automatic-exemption criterion, oldest first.
func (s *HnRService) ListExemptRules(ctx context.Context) ([]model.HnRExemptRule, error) {
	return s.hnr.ListExemptRules(ctx)
}

// CreateExemptRule adds a new automatic-exemption criterion. It does not run a
// pass — the next daemon sweep applies it (or the admin triggers "run now").
func (s *HnRService) CreateExemptRule(ctx context.Context, in HnRExemptRuleInput) (*model.HnRExemptRule, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	rule := &model.HnRExemptRule{Criterion: in.Criterion, Threshold: in.Threshold, Enabled: in.Enabled}
	if err := s.hnr.CreateExemptRule(ctx, rule); err != nil {
		return nil, fmt.Errorf("create hnr exempt rule: %w", err)
	}
	return rule, nil
}

// UpdateExemptRule edits an existing criterion.
func (s *HnRService) UpdateExemptRule(ctx context.Context, id int64, in HnRExemptRuleInput) (*model.HnRExemptRule, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	rule := &model.HnRExemptRule{ID: id, Criterion: in.Criterion, Threshold: in.Threshold, Enabled: in.Enabled}
	if err := s.hnr.UpdateExemptRule(ctx, rule); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrHnRExemptRuleNotFound
		}
		return nil, fmt.Errorf("update hnr exempt rule: %w", err)
	}
	return rule, nil
}

// DeleteExemptRule removes a criterion. Torrents this rule alone auto-exempted
// are released on the next daemon sweep (they no longer match any rule).
func (s *HnRService) DeleteExemptRule(ctx context.Context, id int64) error {
	if err := s.hnr.DeleteExemptRule(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrHnRExemptRuleNotFound
		}
		return fmt.Errorf("delete hnr exempt rule: %w", err)
	}
	return nil
}
