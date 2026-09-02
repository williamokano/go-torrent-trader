package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

func TestHnRClearPrice_FixedMode_DefaultsWithNilSettings(t *testing.T) {
	// A HnRService built without a SiteSettingsService (many tests, and any
	// deployment that somehow omits it) must still price sensibly, using
	// exactly the values migration 081 seeds.
	price := hnrClearPrice(context.Background(), nil, model.HnRRecord{TorrentSize: 2 * gibBytes}, nil)
	if price != 70 { // base 50 + 10*2
		t.Errorf("expected 70, got %d", price)
	}
}

func TestHnRClearPrice_FixedMode_RoundsUpPartialGiB(t *testing.T) {
	settings := settingsWith(map[string]string{
		SettingHnRClearPricingMode:  HnRClearPricingModeFixed,
		SettingHnRClearBasePoints:   "50",
		SettingHnRClearPointsPerGiB: "10",
	})
	// One byte over 1 GiB must still cost more than exactly 1 GiB would —
	// math.Ceil, not truncation, so a sliver of size is never priced free.
	price := hnrClearPrice(context.Background(), settings, model.HnRRecord{TorrentSize: gibBytes + 1}, nil)
	if price != 61 { // 50 + ceil(10 * (1 + 1/gibBytes)) = 50 + 11
		t.Errorf("expected 61, got %d", price)
	}
}

func TestHnRClearPrice_DeficitMode_PricesOnlyTheRemainingUpload(t *testing.T) {
	settings := settingsWith(map[string]string{
		SettingHnRClearPricingMode:         HnRClearPricingModeDeficit,
		SettingHnRClearPointsPerGiBDeficit: "25",
	})
	rule := &model.HnRRule{RequiredRatio: 1.0}
	rec := model.HnRRecord{TorrentSize: 4 * gibBytes, Uploaded: 1 * gibBytes} // needs 4 GiB, has 1 -> 3 GiB short
	price := hnrClearPrice(context.Background(), settings, rec, rule)
	if price != 75 { // 25 * 3
		t.Errorf("expected 75, got %d", price)
	}
}

func TestHnRClearPrice_DeficitMode_ZeroWhenAlreadyMet(t *testing.T) {
	settings := settingsWith(map[string]string{
		SettingHnRClearPricingMode:         HnRClearPricingModeDeficit,
		SettingHnRClearPointsPerGiBDeficit: "25",
	})
	rule := &model.HnRRule{RequiredRatio: 1.0}
	rec := model.HnRRecord{TorrentSize: 4 * gibBytes, Uploaded: 5 * gibBytes} // already over
	price := hnrClearPrice(context.Background(), settings, rec, rule)
	if price != 0 {
		t.Errorf("expected 0 once the ratio requirement is already met, got %d", price)
	}
}

func TestHnRClearPrice_DeficitMode_ZeroWithNoRatioRequirement(t *testing.T) {
	settings := settingsWith(map[string]string{
		SettingHnRClearPricingMode:         HnRClearPricingModeDeficit,
		SettingHnRClearPointsPerGiBDeficit: "25",
	})
	rec := model.HnRRecord{TorrentSize: 4 * gibBytes, Uploaded: 0}
	price := hnrClearPrice(context.Background(), settings, rec, nil) // nil rule
	if price != 0 {
		t.Errorf("expected 0 with no rule (no ratio requirement to buy off), got %d", price)
	}
}

func setupHnRServiceForClearing() (svc *HnRService, hnr *fakeHnRRepo, settings *SiteSettingsService, users *mockUserRepoForRestrictions) {
	hnr = newFakeHnRRepo()
	settings = NewSiteSettingsService(newMockSiteSettingsRepo(), event.NewInMemoryBus())
	users = newMockUserRepoForRestrictions()
	bus := event.NewInMemoryBus()
	warnings := NewWarningService(newMockWarningRepo(), users, newMockMessageRepoForWarnings(), bus)
	restrictions := NewRestrictionService(newMockRestrictionRepo(), users, bus)
	svc = NewHnRService(nil, hnr, &fakeHnRGroupRepo{groups: hnrTestGroups()}, users, warnings, restrictions, settings, bus)
	return svc, hnr, settings, users
}

func TestHnRService_ClearRecord_HappyPath(t *testing.T) {
	svc, hnr, _, _ := setupHnRServiceForClearing()
	hnr.setTorrent(100, 1*gibBytes, false)
	hnr.setBonusPoints(1, 1000)
	if _, err := hnr.CreateIfNotExists(context.Background(), 1, 100, time.Now()); err != nil {
		t.Fatalf("CreateIfNotExists: %v", err)
	}
	rec := hnr.recordByUserTorrent(1, 100)

	result, err := svc.ClearRecord(context.Background(), 1, rec.ID)
	if err != nil {
		t.Fatalf("ClearRecord: %v", err)
	}
	if result.Price != 60 { // base 50 + per-gib 10 * 1
		t.Errorf("expected price 60, got %d", result.Price)
	}
	if result.NewBalance != 940 {
		t.Errorf("expected new balance 940, got %d", result.NewBalance)
	}
	if rec.State != model.HnRStateCleared {
		t.Errorf("expected state cleared, got %s", rec.State)
	}
}

func TestHnRService_ClearRecord_InsufficientPoints(t *testing.T) {
	svc, hnr, _, _ := setupHnRServiceForClearing()
	hnr.setTorrent(100, 1*gibBytes, false)
	hnr.setBonusPoints(1, 10) // far less than the 60-point price
	if _, err := hnr.CreateIfNotExists(context.Background(), 1, 100, time.Now()); err != nil {
		t.Fatalf("CreateIfNotExists: %v", err)
	}
	rec := hnr.recordByUserTorrent(1, 100)

	_, err := svc.ClearRecord(context.Background(), 1, rec.ID)
	if !errors.Is(err, repository.ErrInsufficientBonusPoints) {
		t.Fatalf("got %v, want ErrInsufficientBonusPoints", err)
	}
	if rec.State == model.HnRStateCleared {
		t.Error("record must not be cleared when the spend failed")
	}
}

func TestHnRService_ClearRecord_NotOwnedByCaller(t *testing.T) {
	svc, hnr, _, _ := setupHnRServiceForClearing()
	hnr.setTorrent(100, 1*gibBytes, false)
	hnr.setBonusPoints(2, 1000)
	if _, err := hnr.CreateIfNotExists(context.Background(), 1, 100, time.Now()); err != nil {
		t.Fatalf("CreateIfNotExists: %v", err)
	}
	rec := hnr.recordByUserTorrent(1, 100)

	// User 2 tries to clear user 1's record.
	_, err := svc.ClearRecord(context.Background(), 2, rec.ID)
	if !errors.Is(err, repository.ErrHnRRecordNotClearable) {
		t.Fatalf("got %v, want ErrHnRRecordNotClearable", err)
	}
}

func TestHnRService_ClearRecord_AlreadyResolved(t *testing.T) {
	svc, hnr, _, _ := setupHnRServiceForClearing()
	hnr.records[1] = &model.HnRRecord{
		ID: 1, UserID: 1, TorrentID: 100, State: model.HnRStateSatisfied,
		CompletedAt: time.Now(), LastSeenAt: time.Now(),
	}
	hnr.nextID = 2
	hnr.setBonusPoints(1, 1000)

	_, err := svc.ClearRecord(context.Background(), 1, 1)
	if !errors.Is(err, repository.ErrHnRRecordNotClearable) {
		t.Fatalf("got %v, want ErrHnRRecordNotClearable for an already-resolved record", err)
	}
}

func TestHnRService_ClearRecord_LiftsRestrictionImmediately(t *testing.T) {
	svc, hnr, _, users := setupHnRServiceForClearing()
	seedStages(t, hnr, standardLadder())
	hnr.setUserGroup(1, 10)
	users.addUser(&model.User{ID: 1, Username: "irene", Enabled: true, CanDownload: true})
	hnr.setBonusPoints(1, 10000)

	// Climb to stage 3 (restrict, per standardLadder) with three obligations.
	for i := int64(1); i <= 3; i++ {
		hnr.setTorrent(100+i, gibBytes, false)
		if _, err := hnr.CreateIfNotExists(context.Background(), 1, 100+i, time.Now()); err != nil {
			t.Fatalf("CreateIfNotExists %d: %v", i, err)
		}
		rec := hnr.recordByUserTorrent(1, 100+i)
		rec.State = model.HnRStateBreach
	}
	now := time.Now()
	for step := 0; step < 3; step++ {
		if _, _, err := svc.runLadder(context.Background(), now); err != nil {
			t.Fatalf("runLadder step %d: %v", step, err)
		}
	}
	state, _ := hnr.GetUserState(context.Background(), 1)
	if state.Stage != 3 {
		t.Fatalf("expected stage 3 before clearing, got %d", state.Stage)
	}
	if u, _ := users.GetByID(context.Background(), 1); u.CanDownload {
		t.Fatal("expected download restricted at stage 3, before clearing")
	}

	// Clear all three; the count drops to 0, which should de-escalate and
	// lift the restriction within ClearRecord itself, not a later run.
	recIDs := []int64{}
	for _, r := range hnr.records {
		if r.UserID == 1 {
			recIDs = append(recIDs, r.ID)
		}
	}
	for _, id := range recIDs {
		if _, err := svc.ClearRecord(context.Background(), 1, id); err != nil {
			t.Fatalf("ClearRecord %d: %v", id, err)
		}
	}

	finalState, _ := hnr.GetUserState(context.Background(), 1)
	if finalState.Stage != 0 {
		t.Fatalf("expected the ladder to de-escalate to stage 0 immediately after clearing, got %d", finalState.Stage)
	}
	if u, _ := users.GetByID(context.Background(), 1); !u.CanDownload {
		t.Error("expected the download restriction to be lifted immediately after the last clear, not on the next daemon run")
	}
}

func TestHnRService_QuoteClear_MatchesActualClearPrice(t *testing.T) {
	svc, hnr, _, _ := setupHnRServiceForClearing()
	hnr.setTorrent(100, 1*gibBytes, false)
	hnr.setBonusPoints(1, 1000)
	if _, err := hnr.CreateIfNotExists(context.Background(), 1, 100, time.Now()); err != nil {
		t.Fatalf("CreateIfNotExists: %v", err)
	}
	rec := hnr.recordByUserTorrent(1, 100)

	quoted, err := svc.QuoteClear(context.Background(), 1, rec.ID)
	if err != nil {
		t.Fatalf("QuoteClear: %v", err)
	}
	result, err := svc.ClearRecord(context.Background(), 1, rec.ID)
	if err != nil {
		t.Fatalf("ClearRecord: %v", err)
	}
	if quoted != result.Price {
		t.Errorf("quote (%d) and actual charge (%d) disagree", quoted, result.Price)
	}
}

func TestHnRService_ClearAll_CheapestFirstStopsOnInsufficientBalance(t *testing.T) {
	svc, hnr, _, _ := setupHnRServiceForClearing()
	hnr.setTorrent(100, 1*gibBytes, false)  // price 60
	hnr.setTorrent(101, 5*gibBytes, false)  // price 100
	hnr.setTorrent(102, 10*gibBytes, false) // price 150
	hnr.setBonusPoints(1, 165)              // covers the cheapest two (60+100=160), not the third
	for _, tid := range []int64{100, 101, 102} {
		if _, err := hnr.CreateIfNotExists(context.Background(), 1, tid, time.Now()); err != nil {
			t.Fatalf("CreateIfNotExists %d: %v", tid, err)
		}
	}

	result, err := svc.ClearAll(context.Background(), 1)
	if err != nil {
		t.Fatalf("ClearAll: %v", err)
	}
	if result.Cleared != 2 || result.TotalSpent != 160 {
		t.Fatalf("expected 2 cleared totalling 160, got %+v", result)
	}
	if !result.StoppedInsufficientPoints {
		t.Error("expected StoppedInsufficientPoints=true")
	}
	if result.NewBalance != 5 {
		t.Errorf("expected 5 points remaining, got %d", result.NewBalance)
	}

	open := 0
	for _, r := range hnr.records {
		if r.UserID == 1 && (r.State == model.HnRStateActive || r.State == model.HnRStateBreach) {
			open++
		}
	}
	if open != 1 {
		t.Errorf("expected exactly 1 obligation still open, got %d", open)
	}
}

func TestHnRService_ClearAll_EvenCheapestUnaffordableReportsRealBalance(t *testing.T) {
	svc, hnr, _, users := setupHnRServiceForClearing()
	hnr.setTorrent(100, 1*gibBytes, false) // price 60
	hnr.setBonusPoints(1, 10)              // can't afford even this one
	users.addUser(&model.User{ID: 1, BonusPoints: 10})
	if _, err := hnr.CreateIfNotExists(context.Background(), 1, 100, time.Now()); err != nil {
		t.Fatalf("CreateIfNotExists: %v", err)
	}

	result, err := svc.ClearAll(context.Background(), 1)
	if err != nil {
		t.Fatalf("ClearAll: %v", err)
	}
	if result.Cleared != 0 || !result.StoppedInsufficientPoints {
		t.Fatalf("expected nothing cleared, stopped for insufficient points, got %+v", result)
	}
	if result.NewBalance != 10 {
		t.Errorf("expected untouched balance 10, got %d", result.NewBalance)
	}
}

func TestHnRService_ClearAll_ClearsEverythingAffordable(t *testing.T) {
	svc, hnr, _, _ := setupHnRServiceForClearing()
	seedStages(t, hnr, standardLadder())
	hnr.setTorrent(100, 1*gibBytes, false)
	hnr.setTorrent(101, 1*gibBytes, false)
	hnr.setBonusPoints(1, 1000)
	for _, tid := range []int64{100, 101} {
		if _, err := hnr.CreateIfNotExists(context.Background(), 1, tid, time.Now()); err != nil {
			t.Fatalf("CreateIfNotExists %d: %v", tid, err)
		}
	}

	result, err := svc.ClearAll(context.Background(), 1)
	if err != nil {
		t.Fatalf("ClearAll: %v", err)
	}
	if result.Cleared != 2 || result.StoppedInsufficientPoints {
		t.Fatalf("expected both cleared with no shortfall, got %+v", result)
	}
	// The ladder re-evaluation (a full active-count scan) must run once for
	// the whole sweep, not once per record cleared.
	if hnr.activeCountsCalls != 1 {
		t.Errorf("expected exactly 1 ladder re-evaluation for a 2-record clear-all, got %d", hnr.activeCountsCalls)
	}
}

func TestHnRService_ClearAll_NothingOpenIsANoop(t *testing.T) {
	svc, _, _, users := setupHnRServiceForClearing()
	users.addUser(&model.User{ID: 1, BonusPoints: 5000})

	result, err := svc.ClearAll(context.Background(), 1)
	if err != nil {
		t.Fatalf("ClearAll: %v", err)
	}
	if result.Cleared != 0 || result.StoppedInsufficientPoints {
		t.Fatalf("expected a no-op result, got %+v", result)
	}
	// NewBalance must reflect the member's real balance even when nothing was
	// cleared — it is an unconditional field in the documented response, and
	// leaving it at its zero value would misreport a member who simply has
	// nothing to clear.
	if result.NewBalance != 5000 {
		t.Errorf("expected new balance 5000 (untouched), got %d", result.NewBalance)
	}
}
