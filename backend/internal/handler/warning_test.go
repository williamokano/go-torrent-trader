package handler

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// --- warning repository stub ------------------------------------------------

type stubWarningRepo struct {
	byID     map[int64]*model.Warning
	byUser   []model.Warning
	all      []model.Warning
	total    int64
	active   int
	created  []*model.Warning
	updated  []*model.Warning
	lastOpts repository.ListWarningsOptions
	// includeInactive records the flag ListByUser was last called with, so a
	// test can prove staff get the full history and owners only active ones.
	includeInactive bool

	createErr error
	getErr    error
	listErr   error
	updateErr error

	nextID int64
}

func newStubWarningRepo() *stubWarningRepo {
	return &stubWarningRepo{byID: map[int64]*model.Warning{}, nextID: 1}
}

func (s *stubWarningRepo) Create(_ context.Context, warning *model.Warning) error {
	if s.createErr != nil {
		return s.createErr
	}
	warning.ID = s.nextID
	s.nextID++
	s.created = append(s.created, warning)
	stored := *warning
	s.byID[warning.ID] = &stored
	return nil
}

func (s *stubWarningRepo) GetByID(_ context.Context, id int64) (*model.Warning, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	w, ok := s.byID[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return w, nil
}

func (s *stubWarningRepo) ListByUser(_ context.Context, _ int64, includeInactive bool) ([]model.Warning, error) {
	s.includeInactive = includeInactive
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.byUser, nil
}

func (s *stubWarningRepo) ListAll(_ context.Context, opts repository.ListWarningsOptions) ([]model.Warning, int64, error) {
	s.lastOpts = opts
	if s.listErr != nil {
		return nil, 0, s.listErr
	}
	return s.all, s.total, nil
}

func (s *stubWarningRepo) Update(_ context.Context, warning *model.Warning) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	s.updated = append(s.updated, warning)
	return nil
}

func (s *stubWarningRepo) CountActiveByUser(_ context.Context, _ int64) (int, error) {
	return s.active, nil
}
func (s *stubWarningRepo) CountActiveManualByUser(_ context.Context, _ int64) (int, error) {
	return s.active, nil
}
func (s *stubWarningRepo) GetActiveRatioWarning(_ context.Context, _ int64) (*model.Warning, error) {
	return nil, sql.ErrNoRows
}
func (s *stubWarningRepo) GetUsersWithLowRatio(_ context.Context, _ float64, _ int64) ([]model.User, error) {
	return nil, nil
}
func (s *stubWarningRepo) ResolveExpiredManualWarnings(_ context.Context) ([]int64, error) {
	return nil, nil
}

func newWarningHandler(warnings *stubWarningRepo, users *stubUserRepo) *WarningHandler {
	return NewWarningHandler(
		service.NewWarningService(warnings, users, newStubMessageRepo(), event.NewInMemoryBus()),
	)
}

// --- issue ------------------------------------------------------------------

func TestIssueWarningRequiresAuth(t *testing.T) {
	h := newWarningHandler(newStubWarningRepo(), newStubUserRepo())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/warnings", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.HandleIssueWarning(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestIssueWarningRejectsBadJSON(t *testing.T) {
	h := newWarningHandler(newStubWarningRepo(), newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/warnings", strings.NewReader("{")), 1, 100)
	w := httptest.NewRecorder()
	h.HandleIssueWarning(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a malformed body", w.Code)
	}
}

func TestIssueWarningValidatesRequiredFields(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"missing user_id", `{"reason":"spam"}`},
		{"negative user_id", `{"user_id":-1,"reason":"spam"}`},
		{"missing reason", `{"user_id":9}`},
		{"unparseable expires_at", `{"user_id":9,"reason":"spam","expires_at":"tomorrow"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newWarningHandler(newStubWarningRepo(), newStubUserRepo(&model.User{ID: 9}))

			req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/warnings", strings.NewReader(tc.body)), 1, 100)
			w := httptest.NewRecorder()
			h.HandleIssueWarning(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", w.Code)
			}
		})
	}
}

func TestIssueWarningCreatesActiveWarningAndFlagsUser(t *testing.T) {
	warnings := newStubWarningRepo()
	target := &model.User{ID: 9, Username: "target"}
	users := newStubUserRepo(target, &model.User{ID: 1, Username: "admin"})
	h := newWarningHandler(warnings, users)

	body := `{"user_id":9,"reason":"spamming the forums","expires_at":"2030-01-02T15:04:05Z"}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/warnings", strings.NewReader(body)), 1, 100)
	w := httptest.NewRecorder()
	h.HandleIssueWarning(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	if len(warnings.created) != 1 {
		t.Fatalf("created %d warnings, want 1", len(warnings.created))
	}
	created := warnings.created[0]
	if created.UserID != 9 || created.Status != model.WarningStatusActive || created.Type != model.WarningTypeManual {
		t.Errorf("warning = %+v, want an active manual warning for user 9", created)
	}
	if created.IssuedBy == nil || *created.IssuedBy != 1 {
		t.Errorf("issued_by = %v, want the authenticated actor 1", created.IssuedBy)
	}
	if created.ExpiresAt == nil || created.ExpiresAt.Year() != 2030 {
		t.Errorf("expires_at = %v, want the parsed RFC3339 value", created.ExpiresAt)
	}
	if !target.Warned {
		t.Error("user.Warned = false, want true after a warning is issued")
	}
	if _, ok := decodeBody(t, w)["warning"]; !ok {
		t.Error("response is missing the \"warning\" key")
	}
}

func TestIssueWarningReturns400ForUnknownUser(t *testing.T) {
	h := newWarningHandler(newStubWarningRepo(), newStubUserRepo())

	body := `{"user_id":404,"reason":"spam"}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/warnings", strings.NewReader(body)), 1, 100)
	w := httptest.NewRecorder()
	h.HandleIssueWarning(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 when the target user does not exist", w.Code)
	}
}

func TestIssueWarningSurfacesStoreFailure(t *testing.T) {
	warnings := newStubWarningRepo()
	warnings.createErr = errStub
	h := newWarningHandler(warnings, newStubUserRepo(&model.User{ID: 9}))

	body := `{"user_id":9,"reason":"spam"}`
	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/warnings", strings.NewReader(body)), 1, 100)
	w := httptest.NewRecorder()
	h.HandleIssueWarning(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 — a failed insert must not report success", w.Code)
	}
}

// --- list -------------------------------------------------------------------

func TestListWarningsPassesFiltersThrough(t *testing.T) {
	warnings := newStubWarningRepo()
	warnings.all = []model.Warning{{ID: 1, UserID: 9, Status: model.WarningStatusActive}}
	warnings.total = 1
	h := newWarningHandler(warnings, newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodGet,
		"/api/v1/admin/warnings?user_id=9&status=active&search=bob&page=2&per_page=10", nil), 1, 100)
	w := httptest.NewRecorder()
	h.HandleListWarnings(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	opts := warnings.lastOpts
	if opts.UserID == nil || *opts.UserID != 9 {
		t.Errorf("opts.UserID = %v, want 9", opts.UserID)
	}
	if opts.Status == nil || *opts.Status != "active" {
		t.Errorf("opts.Status = %v, want \"active\"", opts.Status)
	}
	if opts.Search != "bob" || opts.Page != 2 || opts.PerPage != 10 {
		t.Errorf("opts = %+v, want search=bob page=2 per_page=10", opts)
	}

	body := decodeBody(t, w)
	if items, ok := body["warnings"].([]interface{}); !ok || len(items) != 1 {
		t.Errorf("warnings = %v, want 1 item", body["warnings"])
	}
	if body["total"].(float64) != 1 {
		t.Errorf("total = %v, want 1", body["total"])
	}
}

// A nil slice must serialise as [] so the frontend can iterate unconditionally.
func TestListWarningsReturnsEmptyArrayNotNull(t *testing.T) {
	h := newWarningHandler(newStubWarningRepo(), newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/warnings", nil), 1, 100)
	w := httptest.NewRecorder()
	h.HandleListWarnings(w, req)

	if items, ok := decodeBody(t, w)["warnings"].([]interface{}); !ok || len(items) != 0 {
		t.Errorf("warnings = %v, want an empty array", items)
	}
}

func TestListWarningsSurfacesStoreFailure(t *testing.T) {
	warnings := newStubWarningRepo()
	warnings.listErr = errStub
	h := newWarningHandler(warnings, newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/warnings", nil), 1, 100)
	w := httptest.NewRecorder()
	h.HandleListWarnings(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// --- lift -------------------------------------------------------------------

func TestLiftWarningRequiresAuth(t *testing.T) {
	h := newWarningHandler(newStubWarningRepo(), newStubUserRepo())

	req := withURLParam(httptest.NewRequest(http.MethodPost, "/api/v1/admin/warnings/1/lift", strings.NewReader(`{}`)), "id", "1")
	w := httptest.NewRecorder()
	h.HandleLiftWarning(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestLiftWarningRejectsInvalidID(t *testing.T) {
	h := newWarningHandler(newStubWarningRepo(), newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/warnings/x/lift", strings.NewReader(`{}`)), 1, 100)
	req = withURLParam(req, "id", "x")
	w := httptest.NewRecorder()
	h.HandleLiftWarning(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestLiftWarningRejectsBadJSON(t *testing.T) {
	h := newWarningHandler(newStubWarningRepo(), newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/warnings/1/lift", strings.NewReader("{")), 1, 100)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleLiftWarning(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestLiftWarningReturns404WhenMissing(t *testing.T) {
	h := newWarningHandler(newStubWarningRepo(), newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/warnings/1/lift", strings.NewReader(`{"reason":"ok"}`)), 1, 100)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleLiftWarning(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// Lifting an already-lifted warning is a client error, not a silent success.
func TestLiftWarningRejectsInactiveWarning(t *testing.T) {
	warnings := newStubWarningRepo()
	warnings.byID[1] = &model.Warning{ID: 1, UserID: 9, Status: model.WarningStatusLifted}
	h := newWarningHandler(warnings, newStubUserRepo(&model.User{ID: 9}))

	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/warnings/1/lift", strings.NewReader(`{"reason":"ok"}`)), 1, 100)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleLiftWarning(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a warning that is not active", w.Code)
	}
}

func TestLiftWarningRecordsWhoLiftedIt(t *testing.T) {
	warnings := newStubWarningRepo()
	warnings.byID[1] = &model.Warning{ID: 1, UserID: 9, Status: model.WarningStatusActive}
	target := &model.User{ID: 9, Username: "target", Warned: true}
	h := newWarningHandler(warnings, newStubUserRepo(target, &model.User{ID: 1, Username: "admin"}))

	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/warnings/1/lift", strings.NewReader(`{"reason":"appealed"}`)), 1, 100)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleLiftWarning(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", w.Code, w.Body.String())
	}
	if len(warnings.updated) != 1 {
		t.Fatalf("updated %d warnings, want 1", len(warnings.updated))
	}
	updated := warnings.updated[0]
	if updated.Status != model.WarningStatusLifted {
		t.Errorf("status = %q, want %q", updated.Status, model.WarningStatusLifted)
	}
	if updated.LiftedBy == nil || *updated.LiftedBy != 1 {
		t.Errorf("lifted_by = %v, want the authenticated actor 1", updated.LiftedBy)
	}
	if updated.LiftedReason == nil || *updated.LiftedReason != "appealed" {
		t.Errorf("lifted_reason = %v, want \"appealed\"", updated.LiftedReason)
	}
	// No active warnings remain, so the user's warned flag must be cleared.
	if target.Warned {
		t.Error("user.Warned = true, want false once the last active warning is lifted")
	}
}

func TestLiftWarningSurfacesStoreFailure(t *testing.T) {
	warnings := newStubWarningRepo()
	warnings.byID[1] = &model.Warning{ID: 1, UserID: 9, Status: model.WarningStatusActive}
	warnings.updateErr = errStub
	h := newWarningHandler(warnings, newStubUserRepo(&model.User{ID: 9}))

	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/warnings/1/lift", strings.NewReader(`{}`)), 1, 100)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleLiftWarning(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// --- per-user warnings ------------------------------------------------------

func TestGetUserWarningsRequiresAuth(t *testing.T) {
	h := newWarningHandler(newStubWarningRepo(), newStubUserRepo())

	req := withURLParam(httptest.NewRequest(http.MethodGet, "/api/v1/users/9/warnings", nil), "id", "9")
	w := httptest.NewRecorder()
	h.HandleGetUserWarnings(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestGetUserWarningsRejectsInvalidID(t *testing.T) {
	h := newWarningHandler(newStubWarningRepo(), newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/users/0/warnings", nil), 7, 20)
	req = withURLParam(req, "id", "0")
	w := httptest.NewRecorder()
	h.HandleGetUserWarnings(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// A regular member must not be able to read another member's warnings.
func TestGetUserWarningsForbiddenForOtherMembers(t *testing.T) {
	h := newWarningHandler(newStubWarningRepo(), newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/users/9/warnings", nil), 7, 20)
	req = withURLParam(req, "id", "9")
	w := httptest.NewRecorder()
	h.HandleGetUserWarnings(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 — only the owner or staff may read a user's warnings", w.Code)
	}
}

// The owner sees only their active warnings; lifted/resolved history is hidden.
func TestGetUserWarningsOwnerSeesOnlyActive(t *testing.T) {
	warnings := newStubWarningRepo()
	warnings.byUser = []model.Warning{{ID: 1, UserID: 7, Status: model.WarningStatusActive}}
	h := newWarningHandler(warnings, newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/users/7/warnings", nil), 7, 20)
	req = withURLParam(req, "id", "7")
	w := httptest.NewRecorder()
	h.HandleGetUserWarnings(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if warnings.includeInactive {
		t.Error("include_inactive = true for the owner, want false — history is staff-only")
	}
	if items, ok := decodeBody(t, w)["warnings"].([]interface{}); !ok || len(items) != 1 {
		t.Errorf("warnings = %v, want 1 item", items)
	}
}

func TestGetUserWarningsStaffSeesFullHistory(t *testing.T) {
	warnings := newStubWarningRepo()
	warnings.byUser = []model.Warning{
		{ID: 1, UserID: 9, Status: model.WarningStatusActive},
		{ID: 2, UserID: 9, Status: model.WarningStatusLifted},
	}
	h := newWarningHandler(warnings, newStubUserRepo())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/9/warnings", nil)
	req = staffAuthed(req, 1)
	req = withURLParam(req, "id", "9")
	w := httptest.NewRecorder()
	h.HandleGetUserWarnings(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !warnings.includeInactive {
		t.Error("include_inactive = false for staff, want true")
	}
	if items, ok := decodeBody(t, w)["warnings"].([]interface{}); !ok || len(items) != 2 {
		t.Errorf("warnings = %v, want 2 items", items)
	}
}

func TestGetUserWarningsReturnsEmptyArrayNotNull(t *testing.T) {
	h := newWarningHandler(newStubWarningRepo(), newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/users/7/warnings", nil), 7, 20)
	req = withURLParam(req, "id", "7")
	w := httptest.NewRecorder()
	h.HandleGetUserWarnings(w, req)

	if items, ok := decodeBody(t, w)["warnings"].([]interface{}); !ok || len(items) != 0 {
		t.Errorf("warnings = %v, want an empty array", items)
	}
}

func TestGetUserWarningsSurfacesStoreFailure(t *testing.T) {
	warnings := newStubWarningRepo()
	warnings.listErr = errStub
	h := newWarningHandler(warnings, newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodGet, "/api/v1/users/7/warnings", nil), 7, 20)
	req = withURLParam(req, "id", "7")
	w := httptest.NewRecorder()
	h.HandleGetUserWarnings(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}
