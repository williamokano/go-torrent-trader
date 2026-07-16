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

// stubInviteDistRepo is an in-memory InviteDistributionRepository for handler
// tests; the engine-run inputs are inert since these tests exercise the rule
// endpoints and the disabled-skip path.
type stubInviteDistRepo struct {
	rules map[int64]model.InviteDistributionRule
}

func newStubInviteDistRepo() *stubInviteDistRepo {
	return &stubInviteDistRepo{rules: map[int64]model.InviteDistributionRule{}}
}

func (s *stubInviteDistRepo) ListRules(_ context.Context) ([]model.InviteDistributionRule, error) {
	out := make([]model.InviteDistributionRule, 0, len(s.rules))
	for _, r := range s.rules {
		out = append(out, r)
	}
	return out, nil
}

func (s *stubInviteDistRepo) UpsertRule(_ context.Context, r *model.InviteDistributionRule) error {
	s.rules[r.GroupID] = *r
	return nil
}

func (s *stubInviteDistRepo) DeleteRule(_ context.Context, groupID int64) error {
	if _, ok := s.rules[groupID]; !ok {
		return sql.ErrNoRows
	}
	delete(s.rules, groupID)
	return nil
}

func (s *stubInviteDistRepo) GroupMetrics(_ context.Context, _ []int64) ([]model.PromotionUserMetrics, error) {
	return nil, nil
}
func (s *stubInviteDistRepo) UserInviteStates(_ context.Context, _ []int64) (map[int64]model.UserInviteState, error) {
	return map[int64]model.UserInviteState{}, nil
}
func (s *stubInviteDistRepo) LastRunAt(_ context.Context) (time.Time, bool, error) {
	return time.Time{}, false, nil
}
func (s *stubInviteDistRepo) RecordRun(_ context.Context, _ int) error { return nil }

var _ repository.InviteDistributionRepository = (*stubInviteDistRepo)(nil)

// inviteDistSettingsStub is an empty SiteSettingsRepository, so every setting
// falls back to its default (invite distribution disabled).
type inviteDistSettingsStub struct{}

func (inviteDistSettingsStub) Get(_ context.Context, _ string) (*model.SiteSetting, error) {
	return nil, nil
}
func (inviteDistSettingsStub) Set(_ context.Context, _, _ string) error              { return nil }
func (inviteDistSettingsStub) GetAll(_ context.Context) ([]model.SiteSetting, error) { return nil, nil }

func setupInviteDistributionAdminRouter(repo *stubInviteDistRepo) (http.Handler, service.SessionStore) {
	userRepo := newMockUserRepo()
	groupStore := newGroupCRUDStore() // group 1 = admin (staff), 5 = user, 7 = vip
	sessions := testutil.NewMemorySessionStore()
	bus := event.NewInMemoryBus()
	settingsSvc := service.NewSiteSettingsService(inviteDistSettingsStub{}, bus)
	authSvc := service.NewAuthServiceWithTTL(userRepo, sessions, testutil.NewMemoryPasswordResetStore(), &testutil.NoopSender{}, "http://localhost:8080", service.DefaultAccessTokenTTL, service.DefaultRefreshTokenTTL, groupStore, bus)
	adminSvc := service.NewAdminService(userRepo, groupStore, bus)
	inviteDistSvc := service.NewInviteDistributionService(repo, groupStore, userRepo, settingsSvc, bus)

	router := handler.NewRouter(&handler.Deps{
		AuthService:               authSvc,
		SessionStore:              sessions,
		AdminService:              adminSvc,
		InviteDistributionService: inviteDistSvc,
	})
	return router, sessions
}

func TestInviteDistributionRules_ForbiddenForNonAdmin(t *testing.T) {
	router, sessions := setupInviteDistributionAdminRouter(newStubInviteDistRepo())
	regular := createSessionWithGroup(sessions, 5001, 5)

	rec := doGroupRequest(t, router, regular, http.MethodGet, "/api/v1/admin/invite-distribution/rules", nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestInviteDistributionRules_UpsertAndList(t *testing.T) {
	repo := newStubInviteDistRepo()
	router, sessions := setupInviteDistributionAdminRouter(repo)
	admin := createSessionWithGroup(sessions, 5002, 1)

	rec := doGroupRequest(t, router, admin, http.MethodPut, "/api/v1/admin/invite-distribution/rules/7", map[string]interface{}{
		"min_ratio": 1.5, "min_downloaded_bytes": 1000, "max_downloaded_bytes": 0, "max_invites": 3,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("upsert: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	rec = doGroupRequest(t, router, admin, http.MethodGet, "/api/v1/admin/invite-distribution/rules", nil)
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

func TestInviteDistributionRules_UpsertStaffGroupRejected(t *testing.T) {
	router, sessions := setupInviteDistributionAdminRouter(newStubInviteDistRepo())
	admin := createSessionWithGroup(sessions, 5003, 1)

	rec := doGroupRequest(t, router, admin, http.MethodPut, "/api/v1/admin/invite-distribution/rules/1", map[string]interface{}{"min_ratio": 1.0})
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for staff group, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestInviteDistributionRules_UpsertMissingGroup(t *testing.T) {
	router, sessions := setupInviteDistributionAdminRouter(newStubInviteDistRepo())
	admin := createSessionWithGroup(sessions, 5004, 1)

	rec := doGroupRequest(t, router, admin, http.MethodPut, "/api/v1/admin/invite-distribution/rules/999", map[string]interface{}{"min_ratio": 1.0})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing group, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestInviteDistributionRules_UpsertInvalidRangeRejected(t *testing.T) {
	router, sessions := setupInviteDistributionAdminRouter(newStubInviteDistRepo())
	admin := createSessionWithGroup(sessions, 5005, 1)

	rec := doGroupRequest(t, router, admin, http.MethodPut, "/api/v1/admin/invite-distribution/rules/7", map[string]interface{}{
		"min_downloaded_bytes": 2000, "max_downloaded_bytes": 1000,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for min > max, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestInviteDistributionRules_Delete(t *testing.T) {
	repo := newStubInviteDistRepo()
	repo.rules[7] = model.InviteDistributionRule{GroupID: 7, MaxInvites: 3}
	router, sessions := setupInviteDistributionAdminRouter(repo)
	admin := createSessionWithGroup(sessions, 5006, 1)

	rec := doGroupRequest(t, router, admin, http.MethodDelete, "/api/v1/admin/invite-distribution/rules/7", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if _, ok := repo.rules[7]; ok {
		t.Error("rule not deleted")
	}
}

func TestInviteDistributionRun_DisabledReturnsSkipped(t *testing.T) {
	router, sessions := setupInviteDistributionAdminRouter(newStubInviteDistRepo())
	admin := createSessionWithGroup(sessions, 5007, 1)

	rec := doGroupRequest(t, router, admin, http.MethodPost, "/api/v1/admin/invite-distribution/run", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["skipped"] != true {
		t.Errorf("expected skipped=true when disabled, got %v", resp["skipped"])
	}
}

func TestInviteDistributionRun_ForbiddenForNonAdmin(t *testing.T) {
	router, sessions := setupInviteDistributionAdminRouter(newStubInviteDistRepo())
	regular := createSessionWithGroup(sessions, 5008, 5)

	rec := doGroupRequest(t, router, regular, http.MethodPost, "/api/v1/admin/invite-distribution/run", nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}
