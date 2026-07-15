package handler_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/handler"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/williamokano/go-torrent-trader/backend/internal/testutil"
)

// stubPromotionRepo is an in-memory PromotionRepository for handler tests; the
// engine-run methods are inert since these tests exercise the rule endpoints.
type stubPromotionRepo struct {
	rules map[int64]model.PromotionRule
}

func newStubPromotionRepo() *stubPromotionRepo {
	return &stubPromotionRepo{rules: map[int64]model.PromotionRule{}}
}

func (s *stubPromotionRepo) ListRules(_ context.Context) ([]model.PromotionRule, error) {
	out := make([]model.PromotionRule, 0, len(s.rules))
	for _, r := range s.rules {
		out = append(out, r)
	}
	return out, nil
}

func (s *stubPromotionRepo) UpsertRule(_ context.Context, r *model.PromotionRule) error {
	s.rules[r.GroupID] = *r
	return nil
}

func (s *stubPromotionRepo) DeleteRule(_ context.Context, groupID int64) error {
	if _, ok := s.rules[groupID]; !ok {
		return sql.ErrNoRows
	}
	delete(s.rules, groupID)
	return nil
}

func (s *stubPromotionRepo) LadderMetrics(_ context.Context, _ []int64) ([]model.PromotionUserMetrics, error) {
	return nil, nil
}
func (s *stubPromotionRepo) SeedHoursByUser(_ context.Context, _ time.Time, _ int) (map[int64]int, error) {
	return map[int64]int{}, nil
}
func (s *stubPromotionRepo) SetUserGroup(_ context.Context, _, _ int64) error { return nil }
func (s *stubPromotionRepo) LastRunAt(_ context.Context) (time.Time, bool, error) {
	return time.Time{}, false, nil
}
func (s *stubPromotionRepo) RecordRun(_ context.Context, _, _ int) error { return nil }

var _ repository.PromotionRepository = (*stubPromotionRepo)(nil)

// promoSettingsStub is an empty SiteSettingsRepository, so every setting falls
// back to its default (promotion disabled).
type promoSettingsStub struct{}

func (promoSettingsStub) Get(_ context.Context, _ string) (*model.SiteSetting, error) {
	return nil, nil
}
func (promoSettingsStub) Set(_ context.Context, _, _ string) error              { return nil }
func (promoSettingsStub) GetAll(_ context.Context) ([]model.SiteSetting, error) { return nil, nil }

func setupPromotionAdminRouter(promoRepo *stubPromotionRepo) (http.Handler, service.SessionStore) {
	userRepo := newMockUserRepo()
	groupStore := newGroupCRUDStore() // group 1 = admin (staff), 5 = user, 7 = vip
	sessions := testutil.NewMemorySessionStore()
	bus := event.NewInMemoryBus()
	settingsSvc := service.NewSiteSettingsService(promoSettingsStub{}, bus)
	authSvc := service.NewAuthServiceWithTTL(userRepo, sessions, testutil.NewMemoryPasswordResetStore(), &testutil.NoopSender{}, "http://localhost:8080", service.DefaultAccessTokenTTL, service.DefaultRefreshTokenTTL, groupStore, bus)
	adminSvc := service.NewAdminService(userRepo, groupStore, bus)
	promoSvc := service.NewPromotionService(promoRepo, groupStore, settingsSvc)

	router := handler.NewRouter(&handler.Deps{
		AuthService:      authSvc,
		SessionStore:     sessions,
		AdminService:     adminSvc,
		PromotionService: promoSvc,
	})
	return router, sessions
}

func TestPromotionRules_ForbiddenForNonAdmin(t *testing.T) {
	router, sessions := setupPromotionAdminRouter(newStubPromotionRepo())
	regular := createSessionWithGroup(sessions, 4001, 5)

	rec := doGroupRequest(t, router, regular, http.MethodGet, "/api/v1/admin/promotion/rules", nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestPromotionRules_UpsertAndList(t *testing.T) {
	repo := newStubPromotionRepo()
	router, sessions := setupPromotionAdminRouter(repo)
	admin := createSessionWithGroup(sessions, 4002, 1)

	rec := doGroupRequest(t, router, admin, http.MethodPut, "/api/v1/admin/promotion/rules/7", map[string]interface{}{
		"min_ratio": 1.5, "min_uploaded": 1000, "min_seed_hours": 20,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("upsert: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	rec = doGroupRequest(t, router, admin, http.MethodGet, "/api/v1/admin/promotion/rules", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", rec.Code)
	}
	var resp struct {
		Rules []map[string]interface{} `json:"rules"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Rules) != 1 || resp.Rules[0]["group_id"].(float64) != 7 {
		t.Fatalf("unexpected rules list: %+v", resp.Rules)
	}
}

func TestPromotionRules_UpsertStaffGroupRejected(t *testing.T) {
	router, sessions := setupPromotionAdminRouter(newStubPromotionRepo())
	admin := createSessionWithGroup(sessions, 4003, 1)

	rec := doGroupRequest(t, router, admin, http.MethodPut, "/api/v1/admin/promotion/rules/1", map[string]interface{}{"min_ratio": 1.0})
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for staff group, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestPromotionRules_UpsertMissingGroup(t *testing.T) {
	router, sessions := setupPromotionAdminRouter(newStubPromotionRepo())
	admin := createSessionWithGroup(sessions, 4004, 1)

	rec := doGroupRequest(t, router, admin, http.MethodPut, "/api/v1/admin/promotion/rules/999", map[string]interface{}{"min_ratio": 1.0})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing group, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestPromotionRules_Delete(t *testing.T) {
	repo := newStubPromotionRepo()
	repo.rules[7] = model.PromotionRule{GroupID: 7, MinRatio: 1.0}
	router, sessions := setupPromotionAdminRouter(repo)
	admin := createSessionWithGroup(sessions, 4005, 1)

	rec := doGroupRequest(t, router, admin, http.MethodDelete, "/api/v1/admin/promotion/rules/7", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if _, ok := repo.rules[7]; ok {
		t.Error("rule not deleted")
	}
}

func TestPromotionRun_DisabledReturnsSkipped(t *testing.T) {
	router, sessions := setupPromotionAdminRouter(newStubPromotionRepo())
	admin := createSessionWithGroup(sessions, 4006, 1)

	rec := doGroupRequest(t, router, admin, http.MethodPost, "/api/v1/admin/promotion/run", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["skipped"] != true {
		t.Errorf("expected skipped=true when disabled, got %v", resp["skipped"])
	}
}
