package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// fakeBonusRepo implements repository.BonusRepository in memory.
type fakeBonusRepo struct {
	items         []model.BonusStoreItem
	seeding       map[int64]int64
	awarded       []map[int64]int64
	awardReasons  []string
	setCalls      [][3]int64 // user, newBalance, actor
	purchaseErr   error
	purchasedItem *model.BonusStoreItem
	purchaseCalls [][2]int64 // user, item
}

func (f *fakeBonusRepo) ListEnabledItems(_ context.Context) ([]model.BonusStoreItem, error) {
	var out []model.BonusStoreItem
	for _, it := range f.items {
		if it.Enabled {
			out = append(out, it)
		}
	}
	return out, nil
}

func (f *fakeBonusRepo) GetItem(_ context.Context, id int64) (*model.BonusStoreItem, error) {
	for i := range f.items {
		if f.items[i].ID == id {
			return &f.items[i], nil
		}
	}
	return nil, sql.ErrNoRows
}

func (f *fakeBonusRepo) AwardPoints(_ context.Context, awards map[int64]int64, reason string) error {
	f.awarded = append(f.awarded, awards)
	f.awardReasons = append(f.awardReasons, reason)
	return nil
}

func (f *fakeBonusRepo) SetPoints(_ context.Context, userID, newBalance, actorID int64) error {
	f.setCalls = append(f.setCalls, [3]int64{userID, newBalance, actorID})
	return nil
}

func (f *fakeBonusRepo) PurchaseItem(_ context.Context, userID, itemID int64) (*model.BonusStoreItem, int64, error) {
	f.purchaseCalls = append(f.purchaseCalls, [2]int64{userID, itemID})
	if f.purchaseErr != nil {
		return nil, 0, f.purchaseErr
	}
	return f.purchasedItem, 42, nil
}

func (f *fakeBonusRepo) SeedingCounts(_ context.Context) (map[int64]int64, error) {
	return f.seeding, nil
}

func newBonusSvc(settings map[string]string, repo *fakeBonusRepo) *BonusService {
	settingsRepo := newMockSiteSettingsRepo()
	for k, v := range settings {
		settingsRepo.settings[k] = &model.SiteSetting{Key: k, Value: v}
	}
	ss := NewSiteSettingsService(settingsRepo, event.NewInMemoryBus())
	svc := NewBonusService(repo, ss)
	svc.AddSource(NewSeedingBonusSource(repo, ss))
	return svc
}

func bonusEnabledSettings(rate string) map[string]string {
	return map[string]string{
		SettingBonusEnabled:                 "true",
		SettingBonusPointsPerSeedingTorrent: rate,
	}
}

func TestBonusRunAward_DisabledSkips(t *testing.T) {
	repo := &fakeBonusRepo{seeding: map[int64]int64{1: 3}}
	svc := newBonusSvc(map[string]string{SettingBonusEnabled: "false"}, repo)

	sum, err := svc.RunAward(context.Background())
	if err != nil {
		t.Fatalf("RunAward: %v", err)
	}
	if !sum.Skipped || sum.Reason != "disabled" {
		t.Errorf("expected skipped/disabled, got %+v", sum)
	}
	if len(repo.awarded) != 0 {
		t.Error("awards applied while disabled")
	}
}

func TestBonusRunAward_RateTimesCount(t *testing.T) {
	repo := &fakeBonusRepo{seeding: map[int64]int64{1: 3, 2: 10, 3: 0}}
	svc := newBonusSvc(bonusEnabledSettings("2"), repo)

	sum, err := svc.RunAward(context.Background())
	if err != nil {
		t.Fatalf("RunAward: %v", err)
	}
	if len(repo.awarded) != 1 {
		t.Fatalf("awards batches = %d, want 1", len(repo.awarded))
	}
	awards := repo.awarded[0]
	if awards[1] != 6 || awards[2] != 20 {
		t.Errorf("awards = %v, want user1=6 user2=20", awards)
	}
	if _, ok := awards[3]; ok {
		t.Error("zero-count user must not be awarded")
	}
	if repo.awardReasons[0] != model.BonusReasonSeeding {
		t.Errorf("reason = %q, want seeding (source name)", repo.awardReasons[0])
	}
	if sum.UsersAwarded != 2 || sum.PointsAwarded != 26 {
		t.Errorf("summary = %+v, want 2 users / 26 points", sum)
	}
}

func TestBonusRunAward_FractionalRateRounds(t *testing.T) {
	repo := &fakeBonusRepo{seeding: map[int64]int64{1: 1, 2: 3}}
	svc := newBonusSvc(bonusEnabledSettings("0.5"), repo)

	if _, err := svc.RunAward(context.Background()); err != nil {
		t.Fatalf("RunAward: %v", err)
	}
	awards := repo.awarded[0]
	// 0.5*1 rounds to 1 (half up); 0.5*3 = 1.5 rounds to 2.
	if awards[1] != 1 || awards[2] != 2 {
		t.Errorf("awards = %v, want user1=1 user2=2", awards)
	}
}

func TestBonusRunAward_ZeroRateAwardsNothing(t *testing.T) {
	repo := &fakeBonusRepo{seeding: map[int64]int64{1: 5}}
	svc := newBonusSvc(bonusEnabledSettings("0"), repo)

	sum, err := svc.RunAward(context.Background())
	if err != nil {
		t.Fatalf("RunAward: %v", err)
	}
	if len(repo.awarded) != 0 || sum.PointsAwarded != 0 {
		t.Errorf("zero rate must award nothing: %+v / %+v", repo.awarded, sum)
	}
}

func TestBonusListItems_DisabledReturnsEmptyView(t *testing.T) {
	repo := &fakeBonusRepo{items: []model.BonusStoreItem{
		{ID: 1, Slug: "invite-1", Kind: model.BonusKindInvite, Enabled: true},
	}}
	svc := newBonusSvc(map[string]string{SettingBonusEnabled: "false"}, repo)

	view, err := svc.ListItems(context.Background())
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if view.Enabled || len(view.Items) != 0 {
		t.Errorf("disabled store view = %+v, want enabled=false items=[]", view)
	}
}

func TestBonusListItems_HidesUnimplementedKinds(t *testing.T) {
	repo := &fakeBonusRepo{items: []model.BonusStoreItem{
		{ID: 1, Slug: "invite-1", Kind: model.BonusKindInvite, Enabled: true},
		{ID: 2, Slug: "fl", Kind: model.BonusKindFreeleechTicket, Enabled: true}, // enabled but unimplemented
	}}
	svc := newBonusSvc(bonusEnabledSettings("1"), repo)

	view, err := svc.ListItems(context.Background())
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if !view.Enabled || len(view.Items) != 1 || view.Items[0].Slug != "invite-1" {
		t.Errorf("view = %+v, want only invite-1", view)
	}
}

func TestBonusPurchase_DisabledGate(t *testing.T) {
	repo := &fakeBonusRepo{}
	svc := newBonusSvc(map[string]string{SettingBonusEnabled: "false"}, repo)

	_, err := svc.Purchase(context.Background(), 1, 1)
	if !errors.Is(err, ErrBonusDisabled) {
		t.Errorf("err = %v, want ErrBonusDisabled", err)
	}
	if len(repo.purchaseCalls) != 0 {
		t.Error("repo purchase called while disabled")
	}
}

func TestBonusPurchase_ErrorMapping(t *testing.T) {
	tests := []struct {
		name    string
		repoErr error
		want    error
	}{
		{"not found", sql.ErrNoRows, ErrBonusItemNotFound},
		{"not available", repository.ErrBonusKindNotAvailable, ErrBonusItemNotAvailable},
		{"insufficient", repository.ErrInsufficientBonusPoints, ErrBonusInsufficientPoints},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeBonusRepo{purchaseErr: tc.repoErr}
			svc := newBonusSvc(bonusEnabledSettings("1"), repo)
			_, err := svc.Purchase(context.Background(), 1, 1)
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestBonusPurchase_Success(t *testing.T) {
	repo := &fakeBonusRepo{purchasedItem: &model.BonusStoreItem{ID: 7, Slug: "invite-1"}}
	svc := newBonusSvc(bonusEnabledSettings("1"), repo)

	res, err := svc.Purchase(context.Background(), 5, 7)
	if err != nil {
		t.Fatalf("Purchase: %v", err)
	}
	if res.ItemID != 7 || res.Slug != "invite-1" || res.NewBalance != 42 {
		t.Errorf("result = %+v", res)
	}
	if len(repo.purchaseCalls) != 1 || repo.purchaseCalls[0] != [2]int64{5, 7} {
		t.Errorf("purchase call = %v", repo.purchaseCalls)
	}
}
