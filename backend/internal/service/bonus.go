package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// Bonus errors are value errors (the caller asked for something the economy
// refuses), distinct from storage failures.
var (
	ErrBonusDisabled           = errors.New("the bonus system is disabled")
	ErrBonusItemNotFound       = errors.New("store item not found")
	ErrBonusItemNotAvailable   = errors.New("this item is not available yet")
	ErrBonusInsufficientPoints = errors.New("not enough bonus points")
)

// purchasableKinds are the item kinds whose reward effects are implemented.
// Catalogue-only kinds (freeleech_ticket, double_upload) exist as foundation
// but cannot be bought until their effects land.
var purchasableKinds = map[string]bool{
	model.BonusKindInvite:       true,
	model.BonusKindUploadCredit: true,
}

// BonusSource computes one award cycle's points per user. Name doubles as the
// ledger reason, so each source's awards stay distinguishable in history.
// New earning strategies (announce-log seed time, upload deltas, campaigns)
// plug in here without touching the award pipeline.
type BonusSource interface {
	Name() string
	Compute(ctx context.Context) (map[int64]int64, error)
}

// SeedingBonusSource is the basic snapshot source: each cycle it counts the
// torrents a user is currently seeding and awards rate × count, the classic
// tracker model.
type SeedingBonusSource struct {
	repo     repository.BonusRepository
	settings *SiteSettingsService
}

// NewSeedingBonusSource creates the seeding snapshot source.
func NewSeedingBonusSource(repo repository.BonusRepository, settings *SiteSettingsService) *SeedingBonusSource {
	return &SeedingBonusSource{repo: repo, settings: settings}
}

// Name implements BonusSource; it is the ledger reason for these awards.
func (s *SeedingBonusSource) Name() string { return model.BonusReasonSeeding }

// Compute awards rate × currently-seeding-torrents per user, rounded to the
// nearest point per cycle.
func (s *SeedingBonusSource) Compute(ctx context.Context) (map[int64]int64, error) {
	rate := s.settings.GetFloat64(ctx, SettingBonusPointsPerSeedingTorrent, 1)
	if rate <= 0 {
		return nil, nil
	}
	counts, err := s.repo.SeedingCounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("seeding counts: %w", err)
	}
	awards := make(map[int64]int64, len(counts))
	for userID, n := range counts {
		if pts := int64(math.Round(rate * float64(n))); pts > 0 {
			awards[userID] = pts
		}
	}
	return awards, nil
}

// BonusService runs the award pipeline and the bonus store.
type BonusService struct {
	repo     repository.BonusRepository
	settings *SiteSettingsService
	sources  []BonusSource
}

// NewBonusService creates a new BonusService.
func NewBonusService(repo repository.BonusRepository, settings *SiteSettingsService) *BonusService {
	return &BonusService{repo: repo, settings: settings}
}

// AddSource registers an earning source with the award pipeline.
func (s *BonusService) AddSource(src BonusSource) {
	s.sources = append(s.sources, src)
}

// BonusAwardSummary is the outcome of one RunAward cycle.
type BonusAwardSummary struct {
	Skipped       bool
	Reason        string
	UsersAwarded  int
	PointsAwarded int64
}

// RunAward executes every registered source and applies its awards. It is a
// no-op while the bonus system is disabled.
func (s *BonusService) RunAward(ctx context.Context) (BonusAwardSummary, error) {
	if !s.settings.GetBool(ctx, SettingBonusEnabled, false) {
		return BonusAwardSummary{Skipped: true, Reason: "disabled"}, nil
	}

	var summary BonusAwardSummary
	for _, src := range s.sources {
		awards, err := src.Compute(ctx)
		if err != nil {
			return summary, fmt.Errorf("bonus source %q: %w", src.Name(), err)
		}
		if len(awards) == 0 {
			continue
		}
		if err := s.repo.AwardPoints(ctx, awards, src.Name()); err != nil {
			return summary, fmt.Errorf("apply awards from %q: %w", src.Name(), err)
		}
		for _, pts := range awards {
			if pts > 0 {
				summary.UsersAwarded++
				summary.PointsAwarded += pts
			}
		}
	}
	slog.Info("bonus award cycle complete",
		"users_awarded", summary.UsersAwarded, "points_awarded", summary.PointsAwarded)
	return summary, nil
}

// BonusStoreItemView is the store item shape returned to clients.
type BonusStoreItemView struct {
	ID           int64  `json:"id"`
	Slug         string `json:"slug"`
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Price        int64  `json:"price"`
	RewardAmount int64  `json:"reward_amount"`
}

// BonusStoreView is the store listing: enabled flag plus purchasable items.
type BonusStoreView struct {
	Enabled bool                 `json:"enabled"`
	Items   []BonusStoreItemView `json:"items"`
}

// ListItems returns the store contents. When the bonus system is disabled the
// view says so with an empty catalogue (a 200, not an error — the page renders
// the disabled state).
func (s *BonusService) ListItems(ctx context.Context) (BonusStoreView, error) {
	view := BonusStoreView{Items: []BonusStoreItemView{}}
	if !s.settings.GetBool(ctx, SettingBonusEnabled, false) {
		return view, nil
	}
	view.Enabled = true

	items, err := s.repo.ListEnabledItems(ctx)
	if err != nil {
		return view, fmt.Errorf("list store items: %w", err)
	}
	for _, it := range items {
		// Hide catalogue-only kinds until their effects are implemented.
		if !purchasableKinds[it.Kind] {
			continue
		}
		view.Items = append(view.Items, BonusStoreItemView{
			ID: it.ID, Slug: it.Slug, Kind: it.Kind, Name: it.Name,
			Description: it.Description, Price: it.Price, RewardAmount: it.RewardAmount,
		})
	}
	return view, nil
}

// PurchaseResult reports a completed purchase.
type PurchaseResult struct {
	ItemID     int64  `json:"item_id"`
	Slug       string `json:"slug"`
	NewBalance int64  `json:"new_balance"`
}

// Purchase buys one store item for the user, spending points atomically.
func (s *BonusService) Purchase(ctx context.Context, userID, itemID int64) (PurchaseResult, error) {
	if !s.settings.GetBool(ctx, SettingBonusEnabled, false) {
		return PurchaseResult{}, ErrBonusDisabled
	}

	item, newBalance, err := s.repo.PurchaseItem(ctx, userID, itemID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return PurchaseResult{}, ErrBonusItemNotFound
	case errors.Is(err, repository.ErrBonusKindNotAvailable):
		return PurchaseResult{}, ErrBonusItemNotAvailable
	case errors.Is(err, repository.ErrInsufficientBonusPoints):
		return PurchaseResult{}, ErrBonusInsufficientPoints
	case err != nil:
		return PurchaseResult{}, fmt.Errorf("purchase item %d: %w", itemID, err)
	}

	return PurchaseResult{ItemID: item.ID, Slug: item.Slug, NewBalance: newBalance}, nil
}
