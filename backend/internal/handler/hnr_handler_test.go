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
	rules       map[int64]model.HnRRule
	stages      map[int]model.HnRPenaltyStage
	runs        []model.HnRRun
	records     []model.HnRRecord
	bonusPoints map[int64]int64
}

func newStubHnRRepo() *stubHnRRepo {
	return &stubHnRRepo{
		rules: map[int64]model.HnRRule{}, stages: map[int]model.HnRPenaltyStage{},
		bonusPoints: map[int64]int64{},
	}
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
func (s *stubHnRRepo) ListStages(_ context.Context) ([]model.HnRPenaltyStage, error) {
	out := make([]model.HnRPenaltyStage, 0, len(s.stages))
	for _, st := range s.stages {
		out = append(out, st)
	}
	return out, nil
}
func (s *stubHnRRepo) UpsertStage(_ context.Context, st *model.HnRPenaltyStage) error {
	s.stages[st.Stage] = *st
	return nil
}
func (s *stubHnRRepo) DeleteStage(_ context.Context, stage int) error {
	if _, ok := s.stages[stage]; !ok {
		return sql.ErrNoRows
	}
	delete(s.stages, stage)
	return nil
}
func (s *stubHnRRepo) ActiveHnRCounts(context.Context) (map[int64]int, error)      { return nil, nil }
func (s *stubHnRRepo) UsersOnLadder(context.Context) ([]model.HnRUserState, error) { return nil, nil }
func (s *stubHnRRepo) GetUserState(context.Context, int64) (*model.HnRUserState, error) {
	return nil, sql.ErrNoRows
}
func (s *stubHnRRepo) EnsureUserState(context.Context, int64, time.Time) error { return nil }
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
func (s *stubHnRRepo) GetForUser(_ context.Context, userID, recordID int64) (*model.HnRRecord, error) {
	for i := range s.records {
		if s.records[i].ID == recordID && s.records[i].UserID == userID {
			return &s.records[i], nil
		}
	}
	return nil, sql.ErrNoRows
}
func (s *stubHnRRepo) LiveSeedingTorrentIDs(context.Context, int64, []int64) (map[int64]bool, error) {
	return nil, nil
}
func (s *stubHnRRepo) GetRuleForUser(context.Context, int64) (*model.HnRRule, error) { return nil, nil }
func (s *stubHnRRepo) ClearRecord(_ context.Context, userID, recordID, price int64) (int64, error) {
	for i := range s.records {
		if s.records[i].ID != recordID || s.records[i].UserID != userID {
			continue
		}
		rec := &s.records[i]
		if rec.State != model.HnRStateActive && rec.State != model.HnRStateBreach {
			return 0, repository.ErrHnRRecordNotClearable
		}
		if s.bonusPoints[userID] < price {
			return 0, repository.ErrInsufficientBonusPoints
		}
		s.bonusPoints[userID] -= price
		rec.State = model.HnRStateCleared
		return s.bonusPoints[userID], nil
	}
	return 0, repository.ErrHnRRecordNotClearable
}
func (s *stubHnRRepo) AdminList(_ context.Context, opts repository.HnRAdminListOptions) ([]model.HnRRecord, int64, error) {
	var matched []model.HnRRecord
	for _, r := range s.records {
		if opts.State != nil && r.State != *opts.State {
			continue
		}
		if opts.UserID != nil && r.UserID != *opts.UserID {
			continue
		}
		matched = append(matched, r)
	}
	return matched, int64(len(matched)), nil
}
func (s *stubHnRRepo) AggregateStats(_ context.Context) (repository.HnRAggregateStats, error) {
	var stats repository.HnRAggregateStats
	for _, r := range s.records {
		switch r.State {
		case model.HnRStateBreach:
			stats.ActiveHnR++
		case model.HnRStateActive:
			stats.Monitored++
		case model.HnRStateSatisfied:
			stats.Satisfied++
		case model.HnRStateCleared:
			stats.Cleared++
		case model.HnRStateWaived:
			stats.Waived++
		}
	}
	return stats, nil
}
func (s *stubHnRRepo) TopOffenders(_ context.Context, limit int) ([]repository.HnROffender, error) {
	byUser := map[int64]*repository.HnROffender{}
	for _, r := range s.records {
		o, ok := byUser[r.UserID]
		if !ok {
			o = &repository.HnROffender{UserID: r.UserID, Username: r.Username}
			byUser[r.UserID] = o
		}
		o.TotalRecords++
		if r.State == model.HnRStateBreach {
			o.ActiveHnR++
		}
	}
	var out []repository.HnROffender
	for _, o := range byUser {
		if o.ActiveHnR > 0 {
			out = append(out, *o)
		}
	}
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out, nil
}

var _ repository.HnRRepository = (*stubHnRRepo)(nil)

// nopWarningRepo, nopRestrictionRepo and nopMessageRepo exist only so
// WarningService/RestrictionService can be constructed for HnRService's
// dependency list — none of the rule/run-log CRUD tests below exercise the
// ladder engine itself (that lives in internal/service's own tests, against
// the full fake), so these never need to do anything.
type nopWarningRepo struct{}

func (nopWarningRepo) Create(context.Context, *model.Warning) error { return nil }
func (nopWarningRepo) GetByID(context.Context, int64) (*model.Warning, error) {
	return nil, sql.ErrNoRows
}
func (nopWarningRepo) ListByUser(context.Context, int64, bool) ([]model.Warning, error) {
	return nil, nil
}
func (nopWarningRepo) ListAll(context.Context, repository.ListWarningsOptions) ([]model.Warning, int64, error) {
	return nil, 0, nil
}
func (nopWarningRepo) Update(context.Context, *model.Warning) error                { return nil }
func (nopWarningRepo) CountActiveByUser(context.Context, int64) (int, error)       { return 0, nil }
func (nopWarningRepo) CountActiveManualByUser(context.Context, int64) (int, error) { return 0, nil }
func (nopWarningRepo) GetActiveRatioWarning(context.Context, int64) (*model.Warning, error) {
	return nil, sql.ErrNoRows
}
func (nopWarningRepo) GetUsersWithLowRatio(context.Context, float64, int64) ([]model.User, error) {
	return nil, nil
}
func (nopWarningRepo) ResolveExpiredManualWarnings(context.Context) ([]int64, error) {
	return nil, nil
}

var _ repository.WarningRepository = nopWarningRepo{}

type nopRestrictionRepo struct{}

func (nopRestrictionRepo) Create(context.Context, *model.Restriction) error { return nil }
func (nopRestrictionRepo) GetByID(context.Context, int64) (*model.Restriction, error) {
	return nil, sql.ErrNoRows
}
func (nopRestrictionRepo) ListByUser(context.Context, int64) ([]model.Restriction, error) {
	return nil, nil
}
func (nopRestrictionRepo) ListActive(context.Context) ([]model.Restriction, error) { return nil, nil }
func (nopRestrictionRepo) Lift(context.Context, int64, *int64) error               { return nil }
func (nopRestrictionRepo) LiftExpired(context.Context) ([]model.Restriction, error) {
	return nil, nil
}
func (nopRestrictionRepo) HasActiveByType(context.Context, int64, string) (bool, error) {
	return false, nil
}
func (nopRestrictionRepo) LiftActiveBySource(context.Context, int64, string, string) (int, error) {
	return 0, nil
}

var _ repository.RestrictionRepository = nopRestrictionRepo{}

type nopMessageRepo struct{}

func (nopMessageRepo) Create(context.Context, *model.Message) error { return nil }
func (nopMessageRepo) GetByID(context.Context, int64) (*model.Message, error) {
	return nil, sql.ErrNoRows
}
func (nopMessageRepo) ListInbox(context.Context, int64, int, int) ([]model.Message, int64, error) {
	return nil, 0, nil
}
func (nopMessageRepo) ListOutbox(context.Context, int64, int, int) ([]model.Message, int64, error) {
	return nil, 0, nil
}
func (nopMessageRepo) MarkAsRead(context.Context, int64, int64) error    { return nil }
func (nopMessageRepo) DeleteForUser(context.Context, int64, int64) error { return nil }
func (nopMessageRepo) CountUnread(context.Context, int64) (int, error)   { return 0, nil }

var _ repository.MessageRepository = nopMessageRepo{}

func setupHnRAdminRouter(hnrRepo *stubHnRRepo) (http.Handler, service.SessionStore) {
	userRepo := newMockUserRepo()
	groupStore := newGroupCRUDStore() // group 1 = admin (staff), 5 = user, 7 = vip
	sessions := testutil.NewMemorySessionStore()
	bus := event.NewInMemoryBus()
	settingsSvc := service.NewSiteSettingsService(promoSettingsStub{}, bus)
	authSvc := service.NewAuthServiceWithTTL(userRepo, sessions, testutil.NewMemoryPasswordResetStore(), &testutil.NoopSender{}, "http://localhost:8080", service.DefaultAccessTokenTTL, service.DefaultRefreshTokenTTL, groupStore, bus)
	adminSvc := service.NewAdminService(userRepo, groupStore, bus)
	warningSvc := service.NewWarningService(nopWarningRepo{}, userRepo, nopMessageRepo{}, bus)
	restrictionSvc := service.NewRestrictionService(nopRestrictionRepo{}, userRepo, bus)
	// db is nil: RunDaemon then returns ErrHnRDaemonUnavailable, exercised
	// explicitly below rather than worked around, since a handler test has no
	// real database to lock against anyway.
	hnrSvc := service.NewHnRService(nil, hnrRepo, groupStore, userRepo, warningSvc, restrictionSvc, settingsSvc, bus)

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

func TestHnRStages_UpsertAndList(t *testing.T) {
	repo := newStubHnRRepo()
	router, sessions := setupHnRAdminRouter(repo)
	admin := createSessionWithGroup(sessions, 5009, 1)

	rec := doGroupRequest(t, router, admin, http.MethodPut, "/api/v1/admin/hnr/stages/1", map[string]interface{}{
		"min_active_hnr": 1, "min_days_in_prev": 0, "action": "notify", "message_template": "hi {{username}}",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("upsert: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var upsertResp struct {
		Stage map[string]interface{} `json:"stage"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &upsertResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Every field must round-trip snake_case — this is exactly the bug that
	// hit model.HnRRun before it carried json tags.
	if upsertResp.Stage["min_active_hnr"] != float64(1) || upsertResp.Stage["action"] != "notify" {
		t.Fatalf("unexpected upsert response: %+v", upsertResp.Stage)
	}

	rec = doGroupRequest(t, router, admin, http.MethodGet, "/api/v1/admin/hnr/stages", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", rec.Code)
	}
	var listResp struct {
		Stages []map[string]interface{} `json:"stages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(listResp.Stages) != 1 || listResp.Stages[0]["stage"].(float64) != 1 {
		t.Fatalf("unexpected stages list: %+v", listResp.Stages)
	}
}

func TestHnRStages_UpsertRejectsInvalidAction(t *testing.T) {
	router, sessions := setupHnRAdminRouter(newStubHnRRepo())
	admin := createSessionWithGroup(sessions, 5010, 1)

	rec := doGroupRequest(t, router, admin, http.MethodPut, "/api/v1/admin/hnr/stages/1", map[string]interface{}{
		"min_active_hnr": 1, "action": "explode",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an unknown action, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHnRStages_Delete(t *testing.T) {
	repo := newStubHnRRepo()
	repo.stages[1] = model.HnRPenaltyStage{Stage: 1, MinActiveHnR: 1, Action: model.HnRActionNotify}
	router, sessions := setupHnRAdminRouter(repo)
	admin := createSessionWithGroup(sessions, 5011, 1)

	rec := doGroupRequest(t, router, admin, http.MethodDelete, "/api/v1/admin/hnr/stages/1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if _, ok := repo.stages[1]; ok {
		t.Error("stage not deleted")
	}

	rec = doGroupRequest(t, router, admin, http.MethodDelete, "/api/v1/admin/hnr/stages/1", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a missing stage, got %d; body: %s", rec.Code, rec.Body.String())
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

func TestHnRClear_RequiresAuth(t *testing.T) {
	router, _ := setupHnRAdminRouter(newStubHnRRepo())
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/hnr/1/clear", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHnRClear_HappyPath(t *testing.T) {
	repo := newStubHnRRepo()
	repo.records = []model.HnRRecord{
		{ID: 1, UserID: 5012, TorrentID: 100, State: model.HnRStateBreach, TorrentSize: 1 << 30},
	}
	repo.bonusPoints[5012] = 1000
	router, sessions := setupHnRAdminRouter(repo)
	member := createSessionWithGroup(sessions, 5012, 5)

	rec := doGroupRequest(t, router, member, http.MethodPost, "/api/v1/hnr/1/clear", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Price      float64 `json:"price"`
		NewBalance float64 `json:"new_balance"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Price != 60 { // base 50 + per-gib 10 * 1 GiB
		t.Errorf("expected price 60, got %v", resp.Price)
	}
	if resp.NewBalance != 940 {
		t.Errorf("expected new_balance 940, got %v", resp.NewBalance)
	}
	if repo.records[0].State != model.HnRStateCleared {
		t.Errorf("expected the record to be cleared, got state=%s", repo.records[0].State)
	}
}

func TestHnRClear_NotOwnedByCallerReturns400(t *testing.T) {
	repo := newStubHnRRepo()
	repo.records = []model.HnRRecord{
		{ID: 1, UserID: 9999, TorrentID: 100, State: model.HnRStateBreach, TorrentSize: 1 << 30},
	}
	router, sessions := setupHnRAdminRouter(repo)
	member := createSessionWithGroup(sessions, 5013, 5)

	rec := doGroupRequest(t, router, member, http.MethodPost, "/api/v1/hnr/1/clear", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a record owned by someone else, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHnRClear_InsufficientPointsReturns409(t *testing.T) {
	repo := newStubHnRRepo()
	repo.records = []model.HnRRecord{
		{ID: 1, UserID: 5014, TorrentID: 100, State: model.HnRStateBreach, TorrentSize: 1 << 30},
	}
	repo.bonusPoints[5014] = 1 // far less than the price
	router, sessions := setupHnRAdminRouter(repo)
	member := createSessionWithGroup(sessions, 5014, 5)

	rec := doGroupRequest(t, router, member, http.MethodPost, "/api/v1/hnr/1/clear", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for insufficient points, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHnRQuoteClear_MatchesTheActualPrice(t *testing.T) {
	repo := newStubHnRRepo()
	repo.records = []model.HnRRecord{
		{ID: 1, UserID: 5015, TorrentID: 100, State: model.HnRStateBreach, TorrentSize: 1 << 30},
	}
	repo.bonusPoints[5015] = 1000
	router, sessions := setupHnRAdminRouter(repo)
	member := createSessionWithGroup(sessions, 5015, 5)

	rec := doGroupRequest(t, router, member, http.MethodGet, "/api/v1/hnr/1/clear-price", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var quote struct {
		Price float64 `json:"price"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &quote); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	clearRec := doGroupRequest(t, router, member, http.MethodPost, "/api/v1/hnr/1/clear", nil)
	var clearResp struct {
		Price float64 `json:"price"`
	}
	if err := json.Unmarshal(clearRec.Body.Bytes(), &clearResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if quote.Price != clearResp.Price {
		t.Errorf("quote (%v) and actual charge (%v) disagree", quote.Price, clearResp.Price)
	}
}

func TestHnRClearAll_ClearsEverythingAffordable(t *testing.T) {
	repo := newStubHnRRepo()
	repo.records = []model.HnRRecord{
		{ID: 1, UserID: 5016, TorrentID: 100, State: model.HnRStateBreach, TorrentSize: 1 << 30},
		{ID: 2, UserID: 5016, TorrentID: 101, State: model.HnRStateBreach, TorrentSize: 1 << 30},
	}
	repo.bonusPoints[5016] = 1000
	router, sessions := setupHnRAdminRouter(repo)
	member := createSessionWithGroup(sessions, 5016, 5)

	rec := doGroupRequest(t, router, member, http.MethodPost, "/api/v1/hnr/clear-all", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Cleared                   float64 `json:"cleared"`
		StoppedInsufficientPoints bool    `json:"stopped_insufficient_points"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Cleared != 2 || resp.StoppedInsufficientPoints {
		t.Fatalf("expected both records cleared with no shortfall, got %+v", resp)
	}
}

func TestHnRAdminRecords_ForbiddenForNonAdmin(t *testing.T) {
	router, sessions := setupHnRAdminRouter(newStubHnRRepo())
	regular := createSessionWithGroup(sessions, 5017, 5)

	rec := doGroupRequest(t, router, regular, http.MethodGet, "/api/v1/admin/hnr/records", nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHnRAdminRecords_FiltersByState(t *testing.T) {
	repo := newStubHnRRepo()
	repo.records = []model.HnRRecord{
		{ID: 1, UserID: 1, Username: "alice", TorrentName: "A", State: model.HnRStateBreach},
		{ID: 2, UserID: 2, Username: "bob", TorrentName: "B", State: model.HnRStateActive},
	}
	router, sessions := setupHnRAdminRouter(repo)
	admin := createSessionWithGroup(sessions, 5018, 1)

	rec := doGroupRequest(t, router, admin, http.MethodGet, "/api/v1/admin/hnr/records?state=hnr", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Records []map[string]interface{} `json:"records"`
		Total   float64                  `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 1 || len(resp.Records) != 1 || resp.Records[0]["username"] != "alice" {
		t.Fatalf("expected only alice's breached record, got %+v", resp)
	}
}

func TestHnRStats_ReturnsAggregateAndOffenders(t *testing.T) {
	repo := newStubHnRRepo()
	repo.records = []model.HnRRecord{
		{ID: 1, UserID: 1, Username: "alice", State: model.HnRStateBreach},
		{ID: 2, UserID: 1, Username: "alice", State: model.HnRStateBreach},
		{ID: 3, UserID: 2, Username: "bob", State: model.HnRStateActive},
	}
	router, sessions := setupHnRAdminRouter(repo)
	admin := createSessionWithGroup(sessions, 5019, 1)

	rec := doGroupRequest(t, router, admin, http.MethodGet, "/api/v1/admin/hnr/stats", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		ActiveHnR    float64                  `json:"active_hnr"`
		Monitored    float64                  `json:"monitored"`
		TopOffenders []map[string]interface{} `json:"top_offenders"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.ActiveHnR != 2 || resp.Monitored != 1 {
		t.Fatalf("unexpected aggregate: %+v", resp)
	}
	if len(resp.TopOffenders) != 1 || resp.TopOffenders[0]["username"] != "alice" {
		t.Fatalf("unexpected top offenders: %+v", resp.TopOffenders)
	}
}
