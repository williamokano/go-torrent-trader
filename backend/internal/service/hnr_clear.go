package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// gibBytes is one GiB in bytes, the unit every hnr_clear_* setting prices by.
const gibBytes = 1 << 30

// HnRClearResult is what a successful clear reports back to the caller.
type HnRClearResult struct {
	Price      int64
	NewBalance int64
}

// hnrClearPrice computes the points cost to clear one open record, per the
// site's configured pricing mode — always server-side, inside the same call
// ClearRecord's transaction runs in, and never trusted from the client (a
// clear-all especially must never trust a client-sent total, since it is N
// of these added together).
//
// fixed = base + per_gib * torrent size. deficit = per_gib_deficit * however
// much upload is still needed to reach the class's required ratio — 0 when
// the record has no ratio requirement (rule nil or required_ratio <= 0) or
// has already met it, since there is nothing left to buy off in that case.
// Settings default to the values migration 081 seeds, so a HnRService built
// without a SiteSettingsService (tests) still prices sensibly.
func hnrClearPrice(ctx context.Context, settings *SiteSettingsService, rec model.HnRRecord, rule *model.HnRRule) int64 {
	getString := func(key, fallback string) string { return fallback }
	getInt := func(key string, fallback int) int { return fallback }
	if settings != nil {
		getString = func(key, fallback string) string { return settings.GetString(ctx, key, fallback) }
		getInt = func(key string, fallback int) int { return settings.GetInt(ctx, key, fallback) }
	}

	mode := getString(SettingHnRClearPricingMode, HnRClearPricingModeFixed)
	if mode == HnRClearPricingModeDeficit {
		perGiB := int64(getInt(SettingHnRClearPointsPerGiBDeficit, 25))
		var requiredUpload int64
		if rule != nil && rule.RequiredRatio > 0 {
			requiredUpload = int64(rule.RequiredRatio * float64(rec.TorrentSize))
		}
		deficit := requiredUpload - rec.Uploaded
		if deficit <= 0 {
			return 0
		}
		deficitGiB := float64(deficit) / gibBytes
		return int64(math.Ceil(deficitGiB * float64(perGiB)))
	}

	base := int64(getInt(SettingHnRClearBasePoints, 50))
	perGiB := int64(getInt(SettingHnRClearPointsPerGiB, 10))
	sizeGiB := float64(rec.TorrentSize) / gibBytes
	return base + int64(math.Ceil(sizeGiB*float64(perGiB)))
}

// QuoteClear reports what clearing recordID would cost right now, without
// spending anything — what the member page shows before the member commits.
func (s *HnRService) QuoteClear(ctx context.Context, userID, recordID int64) (int64, error) {
	rec, err := s.hnr.GetForUser(ctx, userID, recordID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, repository.ErrHnRRecordNotClearable
		}
		return 0, fmt.Errorf("get record: %w", err)
	}
	if rec.State != model.HnRStateActive && rec.State != model.HnRStateBreach {
		return 0, repository.ErrHnRRecordNotClearable
	}
	rule, err := s.hnr.GetRuleForUser(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("get rule: %w", err)
	}
	return hnrClearPrice(ctx, s.settings, *rec, rule), nil
}

// ClearRecord pays off one open obligation with bonus points. The price is
// computed fresh, server-side, right before the spend — see hnrClearPrice —
// and the spend itself is race-safe inside repository.ClearRecord's own
// transaction (mirrors BonusRepo.PurchaseItem). On success the ladder is
// re-evaluated for just this user immediately, so a restriction the clear
// paid off lifts in this same request rather than waiting for the next
// scheduled run.
func (s *HnRService) ClearRecord(ctx context.Context, userID, recordID int64) (HnRClearResult, error) {
	rec, err := s.hnr.GetForUser(ctx, userID, recordID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return HnRClearResult{}, repository.ErrHnRRecordNotClearable
		}
		return HnRClearResult{}, fmt.Errorf("get record: %w", err)
	}
	if rec.State != model.HnRStateActive && rec.State != model.HnRStateBreach {
		return HnRClearResult{}, repository.ErrHnRRecordNotClearable
	}

	rule, err := s.hnr.GetRuleForUser(ctx, userID)
	if err != nil {
		return HnRClearResult{}, fmt.Errorf("get rule: %w", err)
	}
	price := hnrClearPrice(ctx, s.settings, *rec, rule)

	newBalance, err := s.hnr.ClearRecord(ctx, userID, recordID, price)
	if err != nil {
		// ErrHnRRecordNotClearable / repository.ErrInsufficientBonusPoints
		// pass straight through — the handler maps them.
		return HnRClearResult{}, err
	}

	if err := s.reevaluateLadderForUser(ctx, userID, time.Now()); err != nil {
		slog.Error("hnr clear: ladder re-evaluation failed", "user_id", userID, "record_id", recordID, "error", err)
	}

	return HnRClearResult{Price: price, NewBalance: newBalance}, nil
}

// HnRClearAllResult tallies a clear-all sweep across every open obligation a
// member currently has.
type HnRClearAllResult struct {
	Cleared    int
	TotalSpent int64
	NewBalance int64
	// StoppedInsufficientPoints is true when the sweep stopped partway
	// through because the balance ran out — a partial clear-all is a
	// success, not an error: everything affordable, cheapest first, got
	// cleared.
	StoppedInsufficientPoints bool
}

// ClearAll clears every open obligation the member can currently afford,
// cheapest first, stopping the moment the balance can't cover the next one.
// Each record is priced and spent through the exact same ClearRecord this
// clears one at a time — a clear-all is not a separate, unaudited code path,
// it is this one called in a loop, so it can never total a price a client
// could have sent instead of computing.
func (s *HnRService) ClearAll(ctx context.Context, userID int64) (HnRClearAllResult, error) {
	records, err := s.hnr.ListForUser(ctx, userID)
	if err != nil {
		return HnRClearAllResult{}, fmt.Errorf("list records: %w", err)
	}

	rule, err := s.hnr.GetRuleForUser(ctx, userID)
	if err != nil {
		return HnRClearAllResult{}, fmt.Errorf("get rule: %w", err)
	}

	type priced struct {
		id    int64
		price int64
	}
	var open []priced
	for _, rec := range records {
		if rec.State != model.HnRStateActive && rec.State != model.HnRStateBreach {
			continue
		}
		open = append(open, priced{id: rec.ID, price: hnrClearPrice(ctx, s.settings, rec, rule)})
	}
	// Cheapest first, so a limited balance clears as many obligations as
	// possible rather than running out on the priciest one first.
	for i := 0; i < len(open); i++ {
		for j := i + 1; j < len(open); j++ {
			if open[j].price < open[i].price {
				open[i], open[j] = open[j], open[i]
			}
		}
	}

	var result HnRClearAllResult
	for _, p := range open {
		res, err := s.ClearRecord(ctx, userID, p.id)
		if err != nil {
			if errors.Is(err, repository.ErrInsufficientBonusPoints) {
				result.StoppedInsufficientPoints = true
				break
			}
			// A record that changed state between the list above and this
			// call (satisfied by a fresh seeding announce, say) is not this
			// sweep's problem — skip it and keep clearing the rest.
			if errors.Is(err, repository.ErrHnRRecordNotClearable) {
				continue
			}
			return result, fmt.Errorf("clear record %d: %w", p.id, err)
		}
		result.Cleared++
		result.TotalSpent += res.Price
		result.NewBalance = res.NewBalance
	}
	return result, nil
}
