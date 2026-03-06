package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
)

type mockValidator struct {
	sessions map[string]struct {
		userID  int64
		groupID int64
	}
}

func (m *mockValidator) ValidateSession(token string) (int64, int64, bool) {
	s, ok := m.sessions[token]
	if !ok {
		return 0, 0, false
	}
	return s.userID, s.groupID, true
}

func TestRequireAuth_ValidToken(t *testing.T) {
	v := &mockValidator{
		sessions: map[string]struct {
			userID  int64
			groupID int64
		}{
			"valid-token": {userID: 42, groupID: 5},
		},
	}

	var gotUserID int64
	var gotGroupID int64
	handler := middleware.RequireAuth(v)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID, _ = middleware.UserIDFromContext(r.Context())
		gotGroupID, _ = middleware.GroupIDFromContext(r.Context())
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
}

func TestRequireAuth_MissingHeader(t *testing.T) {
	v := &mockValidator{sessions: map[string]struct {
		userID  int64
		groupID int64
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
		userID  int64
		groupID int64
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

func TestRequireAdmin_AdminGroup(t *testing.T) {
	v := &mockValidator{
		sessions: map[string]struct {
			userID  int64
			groupID int64
		}{
			"admin-token": {userID: 1, groupID: 1},
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

func TestRequireAdmin_NonAdminGroup(t *testing.T) {
	v := &mockValidator{
		sessions: map[string]struct {
			userID  int64
			groupID int64
		}{
			"user-token": {userID: 2, groupID: 5},
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
