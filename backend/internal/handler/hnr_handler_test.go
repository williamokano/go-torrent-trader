package handler_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/handler"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/williamokano/go-torrent-trader/backend/internal/testutil"
)

// stubHnRRepo is an in-memory HnRRepository for handler tests: rule CRUD and
// run-log bookkeeping are real (map-based), matching stubPromotionRepo's
// approach; the accounting/evaluation surface is inert since these tests
// exercise the admin endpoints, not the daemon sweep itself (that lives in
// internal/service's fakeHnRRepo-backed tests).
type stubHnRRepo struct {
	rules   map[int64]model.HnRRule
	runs    []model.HnRRun
	records []model.HnRRecord
}

func newStubHnRRepo() *stubHnRRepo {
	return &stubHnRRepo{rules: map[int64]model.HnRRule{}}
}

func (s *stubHnRRepo) ListRules(_ context.Context) ([]model.HnRRule, error) {
	out := make([]model.HnRRule, 0, len(s.rules))
	for _, r := range s.rules {
		out = append(out, r)
	}
	return out, nil
}

func (s *stubHnRRepo) GetRuleForGroup(_ context.Context, groupID int64) (*model.HnRRule, error) {
	r, ok := s.rules[groupID]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return &r, nil
}

func (s *stubHnRRepo) UpsertRule(_ context.Context, r *model.HnRRule) error {
	s.rules[r.GroupID] = *r
	return nil
}

func (s *stubHnRRepo) DeleteRule(_ context.Context, groupID int64) error {
	if _, ok := s.rules[groupID]; !ok {
		return sql.ErrNoRows
	}
	delete(s.rules, groupID)
	return nil
}

func (s *stubHnRRepo) CreateIfNotExists(context.Context, int64, int64, time.Time) (bool, error) {
	return false, nil
}
func (s *stubHnRRepo) Accumulate(context.Context, int64, int64, int64, time.Duration, time.Time) error {
	return nil
}
func (s *stubHnRRepo) ListOpenForEvaluation(context.Context) ([]repository.HnREvalInput, error) {
	return nil, nil
}
func (s *stubHnRRepo) MarkBreached(context.Context, []int64, time.Time) (int64, error) { return 0, nil }
func (s *stubHnRRepo) MarkSatisfied(context.Context, []int64, time.Time) (int64, error) {
	return 0, nil
}
func (s *stubHnRRepo) MarkWaived(context.Context, []int64, time.Time) (int64, error) { return 0, nil }
func (s *stubHnRRepo) PurgeResolved(context.Context, time.Time) (int64, error)       { return 0, nil }
func (s *stubHnRRepo) ListStages(context.Context) ([]model.HnRPenaltyStage, error)   { return nil, nil }
func (s *stubHnRRepo) UpsertStage(context.Context, *model.HnRPenaltyStage) error     { return nil }
func (s *stubHnRRepo) DeleteStage(context.Context, int) error                        { return sql.ErrNoRows }
func (s *stubHnRRepo) ActiveHnRCounts(context.Context) (map[int64]int, error)        { return nil, nil }
func (s *stubHnRRepo) UsersOnLadder(context.Context) ([]model.HnRUserState, error)   { return nil, nil }
func (s *stubHnRRepo) GetUserState(context.Context, int64) (*model.HnRUserState, error) {
	return nil, sql.ErrNoRows
}
func (s *stubHnRRepo) EnsureUserState(context.Context, int64) error { return nil }
func (s *stubHnRRepo) CASUserStage(context.Context, int64, int, int, time.Time) (bool, error) {
	return false, nil
}
func (s *stubHnRRepo) SetLastNotifiedStage(context.Context, int64, int) error { return nil }

func (s *stubHnRRepo) StartRun(_ context.Context, trigger string, triggeredBy *int64) (int64, error) {
	id := int64(len(s.runs) + 1)
	s.runs = append(s.runs, model.HnRRun{ID: id, StartedAt: time.Now(), Status: model.HnRRunStatusRunning, Trigger: trigger, TriggeredBy: triggeredBy})
	return id, nil
}
func (s *stubHnRRepo) FinishRun(_ context.Context, runID int64, status string, counts repository.HnRRunCounts, errMsg *string) error {
	for i := range s.runs {
		if s.runs[i].ID == runID {
			now := time.Now()
			s.runs[i].FinishedAt = &now
			s.runs[i].Status = status
			s.runs[i].Scanned = counts.Scanned
			s.runs[i].Breached = counts.Breached
			s.runs[i].Satisfied = counts.Satisfied
			s.runs[i].Error = errMsg
			return nil
		}
	}
	return sql.ErrNoRows
}
func (s *stubHnRRepo) LastRun(_ context.Context) (*model.HnRRun, bool, error) {
	if len(s.runs) == 0 {
		return nil, false, nil
	}
	last := s.runs[len(s.runs)-1]
	return &last, true, nil
}
func (s *stubHnRRepo) ListRuns(_ context.Context, limit int) ([]model.HnRRun, error) {
	if limit <= 0 || limit > len(s.runs) {
		limit = len(s.runs)
	}
	out := make([]model.HnRRun, limit)
	for i := 0; i < limit; i++ {
		out[i] = s.runs[len(s.runs)-1-i]
	}
	return out, nil
}

func (s *stubHnRRepo) ListForUser(_ context.Context, userID int64) ([]model.HnRRecord, error) {
	var out []model.HnRRecord
	for _, r := range s.records {
		if r.UserID == userID {
			out = append(out, r)
		}
	}
	return out, nil
}
func (s *stubHnRRepo) GetForUser(context.Context, int64, int64) (*model.HnRRecord, error) {
	return nil, sql.ErrNoRows
}
func (s *stubHnRRepo) LiveSeedingTorrentIDs(context.Context, int64, []int64) (map[int64]bool, error) {
	return nil, nil
}
func (s *stubHnRRepo) GetRuleForUser(context.Context, int64) (*model.HnRRule, error) { return nil, nil }
func (s *stubHnRRepo) ClearRecord(context.Context, int64, int64, int64) (int64, error) {
	return 0, repository.ErrHnRRecordNotClearable
}
func (s *stubHnRRepo) AdminList(context.Context, repository.HnRAdminListOptions) ([]model.HnRRecord, int64, error) {
	return nil, 0, nil
}
func (s *stubHnRRepo) AggregateStats(context.Context) (repository.HnRAggregateStats, error) {
	return repository.HnRAggregateStats{}, nil
}
func (s *stubHnRRepo) TopOffenders(context.Context, int) ([]repository.HnROffender, error) {
	return nil, nil
}

var _ repository.HnRRepository = (*stubHnRRepo)(nil)

func setupHnRAdminRouter(hnrRepo *stubHnRRepo) (http.Handler, service.SessionStore) {
	userRepo := newMockUserRepo()
	groupStore := newGroupCRUDStore() // group 1 = admin (staff), 5 = user, 7 = vip
	sessions := testutil.NewMemorySessionStore()
	bus := event.NewInMemoryBus()
	settingsSvc := service.NewSiteSettingsService(promoSettingsStub{}, bus)
	authSvc := service.NewAuthServiceWithTTL(userRepo, sessions, testutil.NewMemoryPasswordResetStore(), &testutil.NoopSender{}, "http://localhost:8080", service.DefaultAccessTokenTTL, service.DefaultRefreshTokenTTL, groupStore, bus)
	adminSvc := service.NewAdminService(userRepo, groupStore, bus)
	// db is nil: RunDaemon then returns ErrHnRDaemonUnavailable, exercised
	// explicitly below rather than worked around, since a handler test has no
	// real database to lock against anyway.
	hnrSvc := service.NewHnRService(nil, hnrRepo, groupStore, settingsSvc)

	router := handler.NewRouter(&handler.Deps{
		AuthService:  authSvc,
		SessionStore: sessions,
		AdminService: adminSvc,
		HnRService:   hnrSvc,
	})
	return router, sessions
}

func TestHnRRules_ForbiddenForNonAdmin(t *testing.T) {
	router, sessions := setupHnRAdminRouter(newStubHnRRepo())
	regular := createSessionWithGroup(sessions, 5001, 5)

	rec := doGroupRequest(t, router, regular, http.MethodGet, "/api/v1/admin/hnr/rules", nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHnRRules_UpsertAndList(t *testing.T) {
	repo := newStubHnRRepo()
	router, sessions := setupHnRAdminRouter(repo)
	admin := createSessionWithGroup(sessions, 5002, 1)

	rec := doGroupRequest(t, router, admin, http.MethodPut, "/api/v1/admin/hnr/rules/7", map[string]interface{}{
		"required_seed_hours": 240, "required_ratio": 1.0, "inactivity_grace_hours": 48, "max_days_to_satisfy": 30,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("upsert: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	rec = doGroupRequest(t, router, admin, http.MethodGet, "/api/v1/admin/hnr/rules", nil)
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

func TestHnRRules_UpsertStaffGroupRejected(t *testing.T) {
	router, sessions := setupHnRAdminRouter(newStubHnRRepo())
	admin := createSessionWithGroup(sessions, 5003, 1)

	rec := doGroupRequest(t, router, admin, http.MethodPut, "/api/v1/admin/hnr/rules/1", map[string]interface{}{"required_seed_hours": 1})
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for staff group, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHnRRules_UpsertMissingGroup(t *testing.T) {
	router, sessions := setupHnRAdminRouter(newStubHnRRepo())
	admin := createSessionWithGroup(sessions, 5004, 1)

	rec := doGroupRequest(t, router, admin, http.MethodPut, "/api/v1/admin/hnr/rules/999", map[string]interface{}{"required_seed_hours": 1})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing group, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHnRRules_UpsertNegativeThresholdRejected(t *testing.T) {
	router, sessions := setupHnRAdminRouter(newStubHnRRepo())
	admin := createSessionWithGroup(sessions, 5005, 1)

	rec := doGroupRequest(t, router, admin, http.MethodPut, "/api/v1/admin/hnr/rules/7", map[string]interface{}{"required_seed_hours": -1})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for negative threshold, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHnRRules_Delete(t *testing.T) {
	repo := newStubHnRRepo()
	repo.rules[7] = model.HnRRule{GroupID: 7, RequiredSeedHours: 1}
	router, sessions := setupHnRAdminRouter(repo)
	admin := createSessionWithGroup(sessions, 5006, 1)

	rec := doGroupRequest(t, router, admin, http.MethodDelete, "/api/v1/admin/hnr/rules/7", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if _, ok := repo.rules[7]; ok {
		t.Error("rule not deleted")
	}
}

// TestHnRRun_NoDatabaseReturns503 exercises the handler's mapping of
// ErrHnRDaemonUnavailable — a deployment where the HnR service exists but
// was built without a *sql.DB (the locking has no connection to pin) — since
// a handler test has no real database for RunDaemon's advisory lock anyway.
func TestHnRRun_NoDatabaseReturns503(t *testing.T) {
	router, sessions := setupHnRAdminRouter(newStubHnRRepo())
	admin := createSessionWithGroup(sessions, 5007, 1)

	rec := doGroupRequest(t, router, admin, http.MethodPost, "/api/v1/admin/hnr/run", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHnRRuns_ListsRecordedRuns(t *testing.T) {
	repo := newStubHnRRepo()
	actorID := int64(9)
	repo.runs = append(repo.runs, model.HnRRun{
		ID: 1, StartedAt: time.Now(), Status: model.HnRRunStatusSuccess,
		Trigger: model.HnRRunTriggerSchedule, TriggeredBy: &actorID, Scanned: 5, Breached: 1,
	})
	router, sessions := setupHnRAdminRouter(repo)
	admin := createSessionWithGroup(sessions, 5008, 1)

	rec := doGroupRequest(t, router, admin, http.MethodGet, "/api/v1/admin/hnr/runs", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Runs []map[string]interface{} `json:"runs"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Runs) != 1 || resp.Runs[0]["scanned"].(float64) != 5 {
		t.Fatalf("unexpected runs list: %+v", resp.Runs)
	}
}

// TestHnRMember_RequiresAuth exercises the member-facing GET /api/v1/hnr,
// which is a plain endpoint at chi's / for its group in router.go, not a
// staff-gated one — any authenticated user reads their own records.
func TestHnRMember_RequiresAuth(t *testing.T) {
	router, _ := setupHnRAdminRouter(newStubHnRRepo())
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/hnr", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for an unauthenticated request, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHnRMember_ListsOnlyOwnRecords(t *testing.T) {
	repo := newStubHnRRepo()
	repo.records = []model.HnRRecord{
		{ID: 1, UserID: 5009, TorrentID: 100, State: model.HnRStateSatisfied, TorrentName: "mine", CompletedAt: time.Now(), LastSeenAt: time.Now()},
		{ID: 2, UserID: 9999, TorrentID: 200, State: model.HnRStateSatisfied, TorrentName: "not mine", CompletedAt: time.Now(), LastSeenAt: time.Now()},
	}
	router, sessions := setupHnRAdminRouter(repo)
	member := createSessionWithGroup(sessions, 5009, 5)

	rec := doGroupRequest(t, router, member, http.MethodGet, "/api/v1/hnr", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Records []map[string]interface{} `json:"records"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Records) != 1 {
		t.Fatalf("expected exactly the caller's own record, got %+v", resp.Records)
	}
	if resp.Records[0]["torrent_name"] != "mine" {
		t.Errorf("expected the caller's own record, got %+v", resp.Records[0])
	}
	// display_status must be present (and snake_case) even without a JSON
	// tag audit — this is exactly the bug that hit model.HnRRun earlier: an
	// untagged field silently serializes as its Go name instead.
	if resp.Records[0]["display_status"] != model.HnRStateSatisfied {
		t.Errorf("expected display_status=satisfied, got %+v", resp.Records[0]["display_status"])
	}
}
