package handler_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/handler"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/williamokano/go-torrent-trader/backend/internal/testutil"
)

// stubBonusRepo is an in-memory BonusRepository for handler tests.
type stubBonusRepo struct {
	items       []model.BonusStoreItem
	purchaseErr error
	balance     int64
}

func (s *stubBonusRepo) ListEnabledItems(_ context.Context) ([]model.BonusStoreItem, error) {
	var out []model.BonusStoreItem
	for _, it := range s.items {
		if it.Enabled {
			out = append(out, it)
		}
	}
	return out, nil
}

func (s *stubBonusRepo) GetItem(_ context.Context, id int64) (*model.BonusStoreItem, error) {
	for i := range s.items {
		if s.items[i].ID == id {
			return &s.items[i], nil
		}
	}
	return nil, sql.ErrNoRows
}

func (s *stubBonusRepo) AwardPoints(_ context.Context, _ map[int64]int64, _ string) error {
	return nil
}
func (s *stubBonusRepo) SetPoints(_ context.Context, _, _, _ int64) error { return nil }

func (s *stubBonusRepo) PurchaseItem(_ context.Context, _, itemID int64) (*model.BonusStoreItem, int64, error) {
	if s.purchaseErr != nil {
		return nil, 0, s.purchaseErr
	}
	it, err := s.GetItem(context.Background(), itemID)
	if err != nil {
		return nil, 0, err
	}
	return it, s.balance, nil
}

func (s *stubBonusRepo) SeedingCounts(_ context.Context) (map[int64]int64, error) {
	return map[int64]int64{}, nil
}

var _ repository.BonusRepository = (*stubBonusRepo)(nil)

// bonusSettingsStub serves bonus_enabled=true so the store is open in tests.
type bonusSettingsStub struct{}

func (bonusSettingsStub) Get(_ context.Context, key string) (*model.SiteSetting, error) {
	if key == service.SettingBonusEnabled {
		return &model.SiteSetting{Key: key, Value: "true"}, nil
	}
	return nil, nil
}
func (bonusSettingsStub) Set(_ context.Context, _, _ string) error { return nil }
func (bonusSettingsStub) GetAll(_ context.Context) ([]model.SiteSetting, error) {
	return nil, nil
}

func setupBonusRouter(repo *stubBonusRepo, enabled bool) (http.Handler, service.SessionStore) {
	userRepo := newMockUserRepo()
	groupStore := newGroupCRUDStore()
	sessions := testutil.NewMemorySessionStore()
	bus := event.NewInMemoryBus()

	var settingsSvc *service.SiteSettingsService
	if enabled {
		settingsSvc = service.NewSiteSettingsService(bonusSettingsStub{}, bus)
	} else {
		settingsSvc = service.NewSiteSettingsService(promoSettingsStub{}, bus) // all defaults → disabled
	}

	authSvc := service.NewAuthServiceWithTTL(userRepo, sessions, testutil.NewMemoryPasswordResetStore(), &testutil.NoopSender{}, "http://localhost:8080", service.DefaultAccessTokenTTL, service.DefaultRefreshTokenTTL, groupStore, bus)
	bonusSvc := service.NewBonusService(repo, settingsSvc)

	router := handler.NewRouter(&handler.Deps{
		AuthService:  authSvc,
		SessionStore: sessions,
		BonusService: bonusSvc,
	})
	return router, sessions
}

func seededStoreItems() []model.BonusStoreItem {
	return []model.BonusStoreItem{
		{ID: 1, Slug: "invite-1", Kind: model.BonusKindInvite, Name: "1 Invite", Price: 5000, RewardAmount: 1, Enabled: true},
		{ID: 2, Slug: "upload-5gib", Kind: model.BonusKindUploadCredit, Name: "5 GiB", Price: 3000, RewardAmount: 5 << 30, Enabled: true},
	}
}

func TestBonusStore_RequiresAuth(t *testing.T) {
	router, _ := setupBonusRouter(&stubBonusRepo{items: seededStoreItems()}, true)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/store/items", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without session, got %d", rec.Code)
	}
}

func TestBonusStore_ListItems(t *testing.T) {
	router, sessions := setupBonusRouter(&stubBonusRepo{items: seededStoreItems()}, true)
	token := createSessionWithGroup(sessions, 5001, 5)

	rec := doGroupRequest(t, router, token, http.MethodGet, "/api/v1/store/items", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var view struct {
		Enabled bool                     `json:"enabled"`
		Items   []map[string]interface{} `json:"items"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &view)
	if !view.Enabled || len(view.Items) != 2 {
		t.Errorf("view = %+v, want enabled with 2 items", view)
	}
}

func TestBonusStore_ListItemsDisabled(t *testing.T) {
	router, sessions := setupBonusRouter(&stubBonusRepo{items: seededStoreItems()}, false)
	token := createSessionWithGroup(sessions, 5002, 5)

	rec := doGroupRequest(t, router, token, http.MethodGet, "/api/v1/store/items", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 even when disabled, got %d", rec.Code)
	}
	var view struct {
		Enabled bool `json:"enabled"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &view)
	if view.Enabled {
		t.Error("store should report disabled")
	}
}

func TestBonusStore_PurchaseHappyPath(t *testing.T) {
	repo := &stubBonusRepo{items: seededStoreItems(), balance: 111}
	router, sessions := setupBonusRouter(repo, true)
	token := createSessionWithGroup(sessions, 5003, 5)

	rec := doGroupRequest(t, router, token, http.MethodPost, "/api/v1/store/purchase/1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var res struct {
		ItemID     int64  `json:"item_id"`
		Slug       string `json:"slug"`
		NewBalance int64  `json:"new_balance"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if res.ItemID != 1 || res.Slug != "invite-1" || res.NewBalance != 111 {
		t.Errorf("result = %+v", res)
	}
}

func TestBonusStore_PurchaseStatusMapping(t *testing.T) {
	tests := []struct {
		name    string
		repoErr error
		path    string
		enabled bool
		want    int
	}{
		{"disabled store", nil, "/api/v1/store/purchase/1", false, http.StatusForbidden},
		{"unknown item", sql.ErrNoRows, "/api/v1/store/purchase/999", true, http.StatusNotFound},
		{"not available", repository.ErrBonusKindNotAvailable, "/api/v1/store/purchase/1", true, http.StatusConflict},
		{"insufficient points", repository.ErrInsufficientBonusPoints, "/api/v1/store/purchase/1", true, http.StatusConflict},
		{"bad id", nil, "/api/v1/store/purchase/abc", true, http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &stubBonusRepo{items: seededStoreItems(), purchaseErr: tc.repoErr}
			router, sessions := setupBonusRouter(repo, tc.enabled)
			token := createSessionWithGroup(sessions, 5004, 5)

			rec := doGroupRequest(t, router, token, http.MethodPost, tc.path, nil)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d; body: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}
