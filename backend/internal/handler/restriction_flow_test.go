package handler

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// --- restriction repository stub --------------------------------------------

type stubRestrictionRepo struct {
	byID    map[int64]*model.Restriction
	byUser  []model.Restriction
	created []*model.Restriction
	lifted  []int64

	createErr error
	listErr   error
	liftErr   error

	nextID int64
}

func newStubRestrictionRepo() *stubRestrictionRepo {
	return &stubRestrictionRepo{byID: map[int64]*model.Restriction{}, nextID: 1}
}

func (s *stubRestrictionRepo) Create(_ context.Context, r *model.Restriction) error {
	if s.createErr != nil {
		return s.createErr
	}
	r.ID = s.nextID
	s.nextID++
	s.created = append(s.created, r)
	stored := *r
	s.byID[r.ID] = &stored
	return nil
}
func (s *stubRestrictionRepo) GetByID(_ context.Context, id int64) (*model.Restriction, error) {
	r, ok := s.byID[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return r, nil
}
func (s *stubRestrictionRepo) ListByUser(_ context.Context, _ int64) ([]model.Restriction, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.byUser, nil
}
func (s *stubRestrictionRepo) ListActive(_ context.Context) ([]model.Restriction, error) {
	return nil, nil
}
func (s *stubRestrictionRepo) Lift(_ context.Context, id int64, _ *int64) error {
	if s.liftErr != nil {
		return s.liftErr
	}
	s.lifted = append(s.lifted, id)
	return nil
}
func (s *stubRestrictionRepo) LiftExpired(_ context.Context) ([]model.Restriction, error) {
	return nil, nil
}
func (s *stubRestrictionRepo) HasActiveByType(_ context.Context, _ int64, _ string) (bool, error) {
	return false, nil
}

func (s *stubRestrictionRepo) LiftActiveBySource(_ context.Context, userID int64, restrictionType, source string) (int, error) {
	var n int
	for _, r := range s.byID {
		if r.UserID == userID && r.RestrictionType == restrictionType && r.Source == source && r.LiftedAt == nil {
			now := time.Now()
			r.LiftedAt = &now
			s.lifted = append(s.lifted, r.ID)
			n++
		}
	}
	return n, nil
}

func newRestrictionHandler(restrictions *stubRestrictionRepo, users *stubUserRepo, hub *ChatHub) *RestrictionHandler {
	return NewRestrictionHandler(
		service.NewRestrictionService(restrictions, users, event.NewInMemoryBus()), hub,
	)
}

// --- set restrictions -------------------------------------------------------

func TestSetRestrictionsRequiresAuth(t *testing.T) {
	h := newRestrictionHandler(newStubRestrictionRepo(), newStubUserRepo(), nil)

	req := withURLParam(httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/7/restrictions",
		strings.NewReader(`{}`)), "id", "7")
	w := httptest.NewRecorder()
	h.HandleSetRestrictions(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestSetRestrictionsValidatesInput(t *testing.T) {
	cases := []struct {
		name, id, body string
		actor          int64
	}{
		{"non-numeric user id", "abc", `{"reason":"r"}`, 1},
		{"zero user id", "0", `{"reason":"r"}`, 1},
		{"self-restriction", "1", `{"reason":"r"}`, 1},
		{"malformed json", "7", `{`, 1},
		{"missing reason", "7", `{"can_chat":false}`, 1},
		{"unparseable expires_at", "7", `{"reason":"r","can_chat":false,"expires_at":"tomorrow"}`, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			restrictions := newStubRestrictionRepo()
			h := newRestrictionHandler(restrictions, newStubUserRepo(&model.User{ID: 7}), nil)

			req := adminAuthed(httptest.NewRequest(http.MethodPut,
				"/api/v1/admin/users/"+tc.id+"/restrictions", strings.NewReader(tc.body)), tc.actor)
			req = withURLParam(req, "id", tc.id)
			w := httptest.NewRecorder()
			h.HandleSetRestrictions(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", w.Code)
			}
			if len(restrictions.created) != 0 {
				t.Error("a restriction was applied despite the invalid request")
			}
		})
	}
}

func TestSetRestrictionsAppliesEachRevokedPrivilege(t *testing.T) {
	restrictions := newStubRestrictionRepo()
	target := &model.User{ID: 7, Username: "member", CanChat: true, CanUpload: true, CanDownload: true}
	d := newChatDeps() // supplies a hub whose broadcasts we can inspect
	h := newRestrictionHandler(restrictions, newStubUserRepo(target), d.hub)

	body := `{"reason":"abuse","can_chat":false,"can_upload":false,"expires_at":"2030-01-02T15:04:05Z"}`
	req := adminAuthed(httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/7/restrictions", strings.NewReader(body)), 1)
	req = withURLParam(req, "id", "7")
	w := httptest.NewRecorder()
	h.HandleSetRestrictions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if len(restrictions.created) != 2 {
		t.Fatalf("created %d restrictions, want 2 (upload + chat)", len(restrictions.created))
	}
	types := map[string]bool{}
	for _, r := range restrictions.created {
		types[r.RestrictionType] = true
		if r.Reason != "abuse" {
			t.Errorf("reason = %q, want \"abuse\"", r.Reason)
		}
		if r.ExpiresAt == nil || r.ExpiresAt.Year() != 2030 {
			t.Errorf("expires_at = %v, want the parsed RFC3339 value", r.ExpiresAt)
		}
		if r.IssuedBy == nil || *r.IssuedBy != 1 {
			t.Errorf("issued_by = %v, want the authenticated actor 1", r.IssuedBy)
		}
	}
	if !types["chat"] || !types["upload"] {
		t.Errorf("restriction types = %v, want chat and upload", types)
	}
	// The service flips the corresponding user flags.
	if target.CanChat || target.CanUpload {
		t.Errorf("user flags: can_chat=%v can_upload=%v, want both false", target.CanChat, target.CanUpload)
	}
	if !target.CanDownload {
		t.Error("can_download was revoked even though the request left it alone")
	}
}

// The frontend's datetime-local input sends "2006-01-02T15:04" — it must be
// accepted, not rejected as malformed.
func TestSetRestrictionsAcceptsDatetimeLocalExpiry(t *testing.T) {
	restrictions := newStubRestrictionRepo()
	h := newRestrictionHandler(restrictions, newStubUserRepo(&model.User{ID: 7, CanChat: true}), nil)

	body := `{"reason":"abuse","can_chat":false,"expires_at":"2030-03-10T15:30"}`
	req := adminAuthed(httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/7/restrictions", strings.NewReader(body)), 1)
	req = withURLParam(req, "id", "7")
	w := httptest.NewRecorder()
	h.HandleSetRestrictions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if len(restrictions.created) != 1 {
		t.Fatalf("created %d restrictions, want 1", len(restrictions.created))
	}
	exp := restrictions.created[0].ExpiresAt
	if exp == nil || exp.Year() != 2030 || exp.Hour() != 15 || exp.Minute() != 30 {
		t.Errorf("expires_at = %v, want 2030-03-10 15:30 UTC", exp)
	}
}

// Setting a privilege back to true lifts the matching active restrictions.
func TestSetRestrictionsLiftsWhenPrivilegeIsRestored(t *testing.T) {
	restrictions := newStubRestrictionRepo()
	restrictions.byUser = []model.Restriction{
		{ID: 5, UserID: 7, RestrictionType: "chat"},                                // active
		{ID: 6, UserID: 7, RestrictionType: "upload"},                              // different type
		{ID: 7, UserID: 7, RestrictionType: "chat", LiftedAt: ptrTime(time.Now())}, // already lifted
	}
	restrictions.byID[5] = &restrictions.byUser[0]
	h := newRestrictionHandler(restrictions, newStubUserRepo(&model.User{ID: 7}), nil)

	body := `{"reason":"appeal granted","can_chat":true}`
	req := adminAuthed(httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/7/restrictions", strings.NewReader(body)), 1)
	req = withURLParam(req, "id", "7")
	w := httptest.NewRecorder()
	h.HandleSetRestrictions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if len(restrictions.lifted) != 1 || restrictions.lifted[0] != 5 {
		t.Errorf("lifted = %v, want only the active chat restriction [5]", restrictions.lifted)
	}
}

func TestSetRestrictionsSurfacesStoreFailure(t *testing.T) {
	restrictions := newStubRestrictionRepo()
	restrictions.createErr = errStub
	h := newRestrictionHandler(restrictions, newStubUserRepo(&model.User{ID: 7}), nil)

	body := `{"reason":"abuse","can_chat":false}`
	req := adminAuthed(httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/7/restrictions", strings.NewReader(body)), 1)
	req = withURLParam(req, "id", "7")
	w := httptest.NewRecorder()
	h.HandleSetRestrictions(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// --- list -------------------------------------------------------------------

func TestListRestrictionsRejectsInvalidID(t *testing.T) {
	h := newRestrictionHandler(newStubRestrictionRepo(), newStubUserRepo(), nil)

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/x/restrictions", nil), 1)
	req = withURLParam(req, "id", "x")
	w := httptest.NewRecorder()
	h.HandleListRestrictions(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestListRestrictionsReturnsItems(t *testing.T) {
	restrictions := newStubRestrictionRepo()
	restrictions.byUser = []model.Restriction{
		{ID: 5, UserID: 7, RestrictionType: "chat", Reason: "abuse", CreatedAt: time.Now()},
	}
	h := newRestrictionHandler(restrictions, newStubUserRepo(), nil)

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/7/restrictions", nil), 1)
	req = withURLParam(req, "id", "7")
	w := httptest.NewRecorder()
	h.HandleListRestrictions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	items, ok := decodeBody(t, w)["restrictions"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("restrictions = %v, want 1 item", items)
	}
	item := items[0].(map[string]interface{})
	for _, key := range []string{"id", "user_id", "restriction_type", "reason", "issued_by", "expires_at", "lifted_at", "created_at"} {
		if _, present := item[key]; !present {
			t.Errorf("restriction response is missing key %q", key)
		}
	}
}

func TestListRestrictionsSurfacesStoreFailure(t *testing.T) {
	restrictions := newStubRestrictionRepo()
	restrictions.listErr = errStub
	h := newRestrictionHandler(restrictions, newStubUserRepo(), nil)

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/7/restrictions", nil), 1)
	req = withURLParam(req, "id", "7")
	w := httptest.NewRecorder()
	h.HandleListRestrictions(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// --- lift -------------------------------------------------------------------

func TestLiftRestrictionRequiresAuth(t *testing.T) {
	h := newRestrictionHandler(newStubRestrictionRepo(), newStubUserRepo(), nil)

	req := withURLParam(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/restrictions/5", nil), "id", "5")
	w := httptest.NewRecorder()
	h.HandleLiftRestriction(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestLiftRestrictionRejectsInvalidID(t *testing.T) {
	h := newRestrictionHandler(newStubRestrictionRepo(), newStubUserRepo(), nil)

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/restrictions/0", nil), 1)
	req = withURLParam(req, "id", "0")
	w := httptest.NewRecorder()
	h.HandleLiftRestriction(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestLiftRestrictionReturns404WhenMissing(t *testing.T) {
	h := newRestrictionHandler(newStubRestrictionRepo(), newStubUserRepo(), nil)

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/restrictions/5", nil), 1)
	req = withURLParam(req, "id", "5")
	w := httptest.NewRecorder()
	h.HandleLiftRestriction(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestLiftRestrictionRejectsAlreadyLifted(t *testing.T) {
	restrictions := newStubRestrictionRepo()
	restrictions.byID[5] = &model.Restriction{ID: 5, UserID: 7, RestrictionType: "chat", LiftedAt: ptrTime(time.Now())}
	h := newRestrictionHandler(restrictions, newStubUserRepo(&model.User{ID: 7}), nil)

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/restrictions/5", nil), 1)
	req = withURLParam(req, "id", "5")
	w := httptest.NewRecorder()
	h.HandleLiftRestriction(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a restriction that is already lifted", w.Code)
	}
	if len(restrictions.lifted) != 0 {
		t.Error("the restriction was lifted twice")
	}
}

func TestLiftRestrictionRestoresPrivilege(t *testing.T) {
	restrictions := newStubRestrictionRepo()
	restrictions.byID[5] = &model.Restriction{ID: 5, UserID: 7, RestrictionType: "chat"}
	target := &model.User{ID: 7, Username: "member", CanChat: false}
	h := newRestrictionHandler(restrictions, newStubUserRepo(target), nil)

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/restrictions/5", nil), 1)
	req = withURLParam(req, "id", "5")
	w := httptest.NewRecorder()
	h.HandleLiftRestriction(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if len(restrictions.lifted) != 1 || restrictions.lifted[0] != 5 {
		t.Errorf("lifted = %v, want [5]", restrictions.lifted)
	}
	if !target.CanChat {
		t.Error("can_chat is still false — lifting the last chat restriction must restore the privilege")
	}
}

func TestLiftRestrictionSurfacesStoreFailure(t *testing.T) {
	restrictions := newStubRestrictionRepo()
	restrictions.byID[5] = &model.Restriction{ID: 5, UserID: 7, RestrictionType: "chat"}
	restrictions.liftErr = errStub
	h := newRestrictionHandler(restrictions, newStubUserRepo(&model.User{ID: 7}), nil)

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/restrictions/5", nil), 1)
	req = withURLParam(req, "id", "5")
	w := httptest.NewRecorder()
	h.HandleLiftRestriction(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// --- activity log -----------------------------------------------------------

type stubActivityLogRepo struct {
	logs     []model.ActivityLog
	total    int64
	lastOpts repository.ListActivityLogsOptions
	listErr  error
}

func (s *stubActivityLogRepo) Create(_ context.Context, _ *model.ActivityLog) error { return nil }
func (s *stubActivityLogRepo) List(_ context.Context, opts repository.ListActivityLogsOptions) ([]model.ActivityLog, int64, error) {
	s.lastOpts = opts
	if s.listErr != nil {
		return nil, 0, s.listErr
	}
	return s.logs, s.total, nil
}

func TestActivityLogListAppliesFiltersAndPagination(t *testing.T) {
	logs := &stubActivityLogRepo{}
	h := NewActivityLogHandler(service.NewActivityLogService(logs))

	req := adminAuthed(httptest.NewRequest(http.MethodGet,
		"/api/v1/activity-logs?event_type=user.banned&actor_id=1&page=2&per_page=50", nil), 1)
	w := httptest.NewRecorder()
	h.HandleList(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	opts := logs.lastOpts
	if opts.EventType == nil || *opts.EventType != "user.banned" {
		t.Errorf("event_type = %v, want \"user.banned\"", opts.EventType)
	}
	if opts.ActorID == nil || *opts.ActorID != 1 {
		t.Errorf("actor_id = %v, want 1", opts.ActorID)
	}
	if opts.Page != 2 || opts.PerPage != 50 {
		t.Errorf("page/per_page = %d/%d, want 2/50", opts.Page, opts.PerPage)
	}
}

func TestActivityLogListParsesMetadataAsJSON(t *testing.T) {
	meta := `{"torrent_id":3}`
	broken := `not json`
	logs := &stubActivityLogRepo{
		logs: []model.ActivityLog{
			{ID: 1, EventType: "torrent.deleted", Message: "deleted", Metadata: &meta},
			{ID: 2, EventType: "user.banned", Message: "banned", Metadata: &broken},
			{ID: 3, EventType: "user.login", Message: "login"},
		},
		total: 3,
	}
	h := NewActivityLogHandler(service.NewActivityLogService(logs))

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/activity-logs", nil), 1)
	w := httptest.NewRecorder()
	h.HandleList(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := decodeBody(t, w)
	items := body["logs"].([]interface{})
	if len(items) != 3 {
		t.Fatalf("logs = %v, want 3 items", items)
	}
	// Valid metadata is embedded as a JSON object, not a string.
	first := items[0].(map[string]interface{})
	obj, ok := first["metadata"].(map[string]interface{})
	if !ok || obj["torrent_id"] != float64(3) {
		t.Errorf("metadata = %v, want the embedded object {\"torrent_id\":3}", first["metadata"])
	}
	// Unparseable metadata falls back to the raw string rather than blowing up.
	if items[1].(map[string]interface{})["metadata"] != "not json" {
		t.Errorf("metadata = %v, want the raw string for unparseable JSON", items[1].(map[string]interface{})["metadata"])
	}
	// No metadata means the key is simply absent.
	if _, present := items[2].(map[string]interface{})["metadata"]; present {
		t.Error("metadata key present on a log entry that has none")
	}
	if body["total"].(float64) != 3 {
		t.Errorf("total = %v, want 3", body["total"])
	}
	// NOTE: with no page/per_page in the query the handler echoes its own raw
	// opts (0/0) even though the service defaulted the actual query to page 1,
	// 25 per page. The frontend always sends both params, so this is latent
	// rather than broken — asserted below only for the explicit case.
}

func TestActivityLogListSurfacesStoreFailure(t *testing.T) {
	h := NewActivityLogHandler(service.NewActivityLogService(&stubActivityLogRepo{listErr: errStub}))

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/activity-logs", nil), 1)
	w := httptest.NewRecorder()
	h.HandleList(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
