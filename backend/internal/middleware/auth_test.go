package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

type mockValidator struct {
	sessions map[string]struct {
		userID int64
		perms  model.Permissions
	}
}

func (m *mockValidator) ValidateSession(token string) (int64, model.Permissions, bool) {
	s, ok := m.sessions[token]
	if !ok {
		return 0, model.Permissions{}, false
	}
	return s.userID, s.perms, true
}

func TestRequireAuth_ValidToken(t *testing.T) {
	v := &mockValidator{
		sessions: map[string]struct {
			userID int64
			perms  model.Permissions
		}{
			"valid-token": {userID: 42, perms: model.Permissions{GroupID: 5, GroupName: "User"}},
		},
	}

	var gotUserID int64
	var gotGroupID int64
	var gotPerms model.Permissions
	handler := middleware.RequireAuth(v)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID, _ = middleware.UserIDFromContext(r.Context())
		gotGroupID, _ = middleware.GroupIDFromContext(r.Context())
		gotPerms = middleware.PermissionsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if gotUserID != 42 {
		t.Errorf("expected userID=42, got %d", gotUserID)
	}
	if gotGroupID != 5 {
		t.Errorf("expected groupID=5, got %d", gotGroupID)
	}
	if gotPerms.GroupName != "User" {
		t.Errorf("expected groupName=User, got %s", gotPerms.GroupName)
	}
}

func TestRequireAuth_MissingHeader(t *testing.T) {
	v := &mockValidator{sessions: map[string]struct {
		userID int64
		perms  model.Permissions
	}{}}

	handler := middleware.RequireAuth(v)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestRequireAuth_InvalidToken(t *testing.T) {
	v := &mockValidator{sessions: map[string]struct {
		userID int64
		perms  model.Permissions
	}{}}

	handler := middleware.RequireAuth(v)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestRequireAdmin_AdminPermissions(t *testing.T) {
	v := &mockValidator{
		sessions: map[string]struct {
			userID int64
			perms  model.Permissions
		}{
			"admin-token": {userID: 1, perms: model.Permissions{GroupID: 1, IsAdmin: true}},
		},
	}

	handler := middleware.RequireAuth(v)(middleware.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestRequireAdmin_NonAdminPermissions(t *testing.T) {
	v := &mockValidator{
		sessions: map[string]struct {
			userID int64
			perms  model.Permissions
		}{
			"user-token": {userID: 2, perms: model.Permissions{GroupID: 5}},
		},
	}

	handler := middleware.RequireAuth(v)(middleware.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})))

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer user-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestRequireStaff_Moderator(t *testing.T) {
	v := &mockValidator{
		sessions: map[string]struct {
			userID int64
			perms  model.Permissions
		}{
			"mod-token": {userID: 3, perms: model.Permissions{GroupID: 2, IsModerator: true}},
		},
	}

	handler := middleware.RequireAuth(v)(middleware.RequireStaff(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest(http.MethodGet, "/staff", nil)
	req.Header.Set("Authorization", "Bearer mod-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestRequireStaff_RegularUser(t *testing.T) {
	v := &mockValidator{
		sessions: map[string]struct {
			userID int64
			perms  model.Permissions
		}{
			"user-token": {userID: 4, perms: model.Permissions{GroupID: 5}},
		},
	}

	handler := middleware.RequireAuth(v)(middleware.RequireStaff(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})))

	req := httptest.NewRequest(http.MethodGet, "/staff", nil)
	req.Header.Set("Authorization", "Bearer user-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestRequireCapability_Upload_Allowed(t *testing.T) {
	v := &mockValidator{
		sessions: map[string]struct {
			userID int64
			perms  model.Permissions
		}{
			"user-token": {userID: 5, perms: model.Permissions{GroupID: 5, CanUpload: true}},
		},
	}

	handler := middleware.RequireAuth(v)(middleware.RequireCapability("upload")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest(http.MethodPost, "/torrents", nil)
	req.Header.Set("Authorization", "Bearer user-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestRequireCapability_Upload_Denied(t *testing.T) {
	v := &mockValidator{
		sessions: map[string]struct {
			userID int64
			perms  model.Permissions
		}{
			"val-token": {userID: 6, perms: model.Permissions{GroupID: 6, CanUpload: false}},
		},
	}

	handler := middleware.RequireAuth(v)(middleware.RequireCapability("upload")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})))

	req := httptest.NewRequest(http.MethodPost, "/torrents", nil)
	req.Header.Set("Authorization", "Bearer val-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestRequireCapability_Comment_Denied(t *testing.T) {
	v := &mockValidator{
		sessions: map[string]struct {
			userID int64
			perms  model.Permissions
		}{
			"val-token": {userID: 7, perms: model.Permissions{GroupID: 6, CanComment: false}},
		},
	}

	handler := middleware.RequireAuth(v)(middleware.RequireCapability("comment")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})))

	req := httptest.NewRequest(http.MethodPost, "/comments", nil)
	req.Header.Set("Authorization", "Bearer val-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}
