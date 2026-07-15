package handler_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/handler"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/williamokano/go-torrent-trader/backend/internal/testutil"
)

// groupCRUDStore implements both group repository interfaces in memory for the
// group management handler tests.
type groupCRUDStore struct {
	groups  map[int64]*model.Group
	members map[int64]int
	nextID  int64
}

func newGroupCRUDStore() *groupCRUDStore {
	return &groupCRUDStore{
		groups: map[int64]*model.Group{
			1: {ID: 1, Name: "Administrator", Slug: "administrator", Level: 100, IsAdmin: true, IsImmune: true, CanUpload: true, CanDownload: true, CanInvite: true, CanComment: true, CanForum: true},
			5: {ID: 5, Name: "User", Slug: "user", Level: 20, CanUpload: true, CanDownload: true, CanComment: true, CanForum: true},
			7: {ID: 7, Name: "VIP", Slug: "vip", Level: 60},
		},
		members: map[int64]int{},
		nextID:  8,
	}
}

func (s *groupCRUDStore) GetByID(_ context.Context, id int64) (*model.Group, error) {
	if g, ok := s.groups[id]; ok {
		cp := *g
		return &cp, nil
	}
	return nil, sql.ErrNoRows
}

func (s *groupCRUDStore) List(_ context.Context) ([]model.Group, error) {
	out := make([]model.Group, 0, len(s.groups))
	for _, g := range s.groups {
		out = append(out, *g)
	}
	return out, nil
}

func (s *groupCRUDStore) Create(_ context.Context, g *model.Group) error {
	g.ID = s.nextID
	s.nextID++
	cp := *g
	s.groups[g.ID] = &cp
	return nil
}

func (s *groupCRUDStore) Update(_ context.Context, g *model.Group) error {
	if _, ok := s.groups[g.ID]; !ok {
		return sql.ErrNoRows
	}
	cp := *g
	s.groups[g.ID] = &cp
	return nil
}

func (s *groupCRUDStore) Delete(_ context.Context, id int64) error {
	if _, ok := s.groups[id]; !ok {
		return sql.ErrNoRows
	}
	delete(s.groups, id)
	return nil
}

func (s *groupCRUDStore) CountMembers(_ context.Context, id int64) (int, error) {
	return s.members[id], nil
}

func setupGroupAdminRouter(store *groupCRUDStore) (http.Handler, service.SessionStore) {
	userRepo := newMockUserRepo()
	sessions := testutil.NewMemorySessionStore()
	bus := event.NewInMemoryBus()
	authSvc := service.NewAuthServiceWithTTL(userRepo, sessions, testutil.NewMemoryPasswordResetStore(), &testutil.NoopSender{}, "http://localhost:8080", service.DefaultAccessTokenTTL, service.DefaultRefreshTokenTTL, store, bus)
	adminSvc := service.NewAdminService(userRepo, store, bus)
	adminSvc.SetSessionStore(sessions)
	adminSvc.SetGroupWriter(store)

	router := handler.NewRouter(&handler.Deps{
		AuthService:  authSvc,
		SessionStore: sessions,
		AdminService: adminSvc,
	})
	return router, sessions
}

func doGroupRequest(t *testing.T, router http.Handler, token, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestHandleCreateGroup_AsAdmin(t *testing.T) {
	router, sessions := setupGroupAdminRouter(newGroupCRUDStore())
	admin := createSessionWithGroup(sessions, 3001, 1)

	rec := doGroupRequest(t, router, admin, http.MethodPost, "/api/v1/admin/groups", map[string]interface{}{
		"name": "Seedbox", "level": 50, "can_upload": true,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["group"]["slug"] != "seedbox" {
		t.Errorf("expected derived slug 'seedbox', got %v", resp["group"]["slug"])
	}
}

func TestHandleCreateGroup_ForbiddenForNonAdmin(t *testing.T) {
	router, sessions := setupGroupAdminRouter(newGroupCRUDStore())
	regular := createSessionWithGroup(sessions, 3002, 5)

	rec := doGroupRequest(t, router, regular, http.MethodPost, "/api/v1/admin/groups", map[string]interface{}{"name": "X"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCreateGroup_ValidationError(t *testing.T) {
	router, sessions := setupGroupAdminRouter(newGroupCRUDStore())
	admin := createSessionWithGroup(sessions, 3003, 1)

	rec := doGroupRequest(t, router, admin, http.MethodPost, "/api/v1/admin/groups", map[string]interface{}{"name": "   "})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleUpdateGroup_AsAdmin(t *testing.T) {
	router, sessions := setupGroupAdminRouter(newGroupCRUDStore())
	admin := createSessionWithGroup(sessions, 3004, 1)

	rec := doGroupRequest(t, router, admin, http.MethodPut, "/api/v1/admin/groups/7", map[string]interface{}{
		"name": "VIP Plus", "slug": "vip", "level": 65,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteGroup_ProtectedBuiltin(t *testing.T) {
	router, sessions := setupGroupAdminRouter(newGroupCRUDStore())
	admin := createSessionWithGroup(sessions, 3005, 1)

	rec := doGroupRequest(t, router, admin, http.MethodDelete, "/api/v1/admin/groups/1", nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for admin group, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteGroup_HasMembers(t *testing.T) {
	store := newGroupCRUDStore()
	store.members[7] = 4
	router, sessions := setupGroupAdminRouter(store)
	admin := createSessionWithGroup(sessions, 3006, 1)

	rec := doGroupRequest(t, router, admin, http.MethodDelete, "/api/v1/admin/groups/7", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for group with members, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteGroup_Success(t *testing.T) {
	store := newGroupCRUDStore()
	router, sessions := setupGroupAdminRouter(store)
	admin := createSessionWithGroup(sessions, 3007, 1)

	rec := doGroupRequest(t, router, admin, http.MethodDelete, "/api/v1/admin/groups/7", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if _, ok := store.groups[7]; ok {
		t.Error("group 7 still present after delete")
	}
}
