package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/handler"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/williamokano/go-torrent-trader/backend/internal/testutil"
)

func setupAdminRouter() (http.Handler, service.SessionStore) {
	userRepo := newMockUserRepo()
	groupRepo := &mockGroupRepo{}
	sessions := testutil.NewMemorySessionStore()
	bus := event.NewInMemoryBus()
	authSvc := service.NewAuthServiceWithTTL(userRepo, sessions, testutil.NewMemoryPasswordResetStore(), &testutil.NoopSender{}, "http://localhost:8080", service.DefaultAccessTokenTTL, service.DefaultRefreshTokenTTL, groupRepo, bus)
	adminSvc := service.NewAdminService(userRepo, groupRepo, bus)
	adminSvc.SetSessionStore(sessions)
	adminSvc.SetEmailSender(&testutil.NoopSender{})

	router := handler.NewRouter(&handler.Deps{
		AuthService:  authSvc,
		SessionStore: sessions,
		AdminService: adminSvc,
	})
	return router, sessions
}

func setupAdminRouterWithRepo() (http.Handler, service.SessionStore, *mockUserRepo) {
	userRepo := newMockUserRepo()
	groupRepo := &mockGroupRepo{}
	sessions := testutil.NewMemorySessionStore()
	bus := event.NewInMemoryBus()
	authSvc := service.NewAuthServiceWithTTL(userRepo, sessions, testutil.NewMemoryPasswordResetStore(), &testutil.NoopSender{}, "http://localhost:8080", service.DefaultAccessTokenTTL, service.DefaultRefreshTokenTTL, groupRepo, bus)
	adminSvc := service.NewAdminService(userRepo, groupRepo, bus)
	adminSvc.SetSessionStore(sessions)
	adminSvc.SetEmailSender(&testutil.NoopSender{})

	router := handler.NewRouter(&handler.Deps{
		AuthService:  authSvc,
		SessionStore: sessions,
		AdminService: adminSvc,
	})
	return router, sessions, userRepo
}


func TestHandleListUsers_AdminOnly(t *testing.T) {
	router, sessions := setupAdminRouter()

	// Regular user should get 403
	userToken := createSessionWithGroup(sessions, 2000, 5)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-admin, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleListUsers_AsAdmin(t *testing.T) {
	router, sessions := setupAdminRouter()

	// Register a user first
	registerAndGetToken(t, router)

	adminToken := createSessionWithGroup(sessions, 2001, 1)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	users := resp["users"].([]interface{})
	if len(users) < 1 {
		t.Error("expected at least 1 user")
	}
}

func TestHandleUpdateUser_AsAdmin(t *testing.T) {
	router, sessions := setupAdminRouter()

	// Register a user
	registerAndGetToken(t, router)

	adminToken := createSessionWithGroup(sessions, 2002, 1)

	body, _ := json.Marshal(map[string]interface{}{
		"enabled": false,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	user := resp["user"].(map[string]interface{})
	if user["enabled"] != false {
		t.Errorf("expected enabled to be false, got %v", user["enabled"])
	}
}

func TestHandleUpdateUser_NotFound(t *testing.T) {
	router, sessions := setupAdminRouter()

	adminToken := createSessionWithGroup(sessions, 2003, 1)
	body, _ := json.Marshal(map[string]interface{}{
		"enabled": false,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/9999", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleUpdateUser_InvalidBody(t *testing.T) {
	router, sessions := setupAdminRouter()

	adminToken := createSessionWithGroup(sessions, 2004, 1)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/1", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleListGroups_AsAdmin(t *testing.T) {
	router, sessions := setupAdminRouter()

	adminToken := createSessionWithGroup(sessions, 2005, 1)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/groups", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	groups := resp["groups"].([]interface{})
	if len(groups) < 1 {
		t.Error("expected at least 1 group")
	}
}

func TestHandleListGroups_NonAdmin(t *testing.T) {
	router, sessions := setupAdminRouter()

	userToken := createSessionWithGroup(sessions, 2006, 5)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/groups", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleResetPassword_AutoGenerate(t *testing.T) {
	router, sessions, userRepo := setupAdminRouterWithRepo()

	// First registered user becomes admin (group 1, level 100)
	registerAndGetToken(t, router)
	// Second registered user becomes regular user (group 5, level 20)
	registerAndGetToken(t, router)

	// Seed admin session for user 1 (the first registered user is admin)
	adminToken := createSessionWithGroup(sessions, 1, 1)

	// Reset password of user 2 (regular user)
	body, _ := json.Marshal(map[string]interface{}{})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/2/reset-password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	newPass, ok := resp["new_password"].(string)
	if !ok || len(newPass) == 0 {
		t.Error("expected new_password in response")
	}
	// Verify unused imports don't accumulate
	_ = userRepo
}

func TestHandleResetPassword_WithPassword(t *testing.T) {
	router, sessions, _ := setupAdminRouterWithRepo()

	registerAndGetToken(t, router)
	registerAndGetToken(t, router)

	adminToken := createSessionWithGroup(sessions, 1, 1)

	body, _ := json.Marshal(map[string]interface{}{
		"new_password": "CustomPass123!",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/2/reset-password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["new_password"] != "CustomPass123!" {
		t.Errorf("expected CustomPass123!, got %v", resp["new_password"])
	}
}

func TestHandleResetPassword_NotFound(t *testing.T) {
	router, sessions, _ := setupAdminRouterWithRepo()

	// Register first user to become admin
	registerAndGetToken(t, router)
	adminToken := createSessionWithGroup(sessions, 1, 1)

	body, _ := json.Marshal(map[string]interface{}{})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/9999/reset-password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleResetPassword_NonAdmin(t *testing.T) {
	router, sessions, _ := setupAdminRouterWithRepo()

	// Register two users: first = admin, second = user
	registerAndGetToken(t, router)
	registerAndGetToken(t, router)

	// Non-admin session for user 2
	userToken := createSessionWithGroup(sessions, 2, 5)

	body, _ := json.Marshal(map[string]interface{}{})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/1/reset-password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleResetPasskey_Success(t *testing.T) {
	router, sessions, _ := setupAdminRouterWithRepo()

	registerAndGetToken(t, router)
	registerAndGetToken(t, router)

	adminToken := createSessionWithGroup(sessions, 1, 1)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/2/reset-passkey", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	newPasskey, ok := resp["new_passkey"].(string)
	if !ok || len(newPasskey) != 32 {
		t.Errorf("expected 32-char passkey, got %q", resp["new_passkey"])
	}
}

func TestHandleResetPasskey_NotFound(t *testing.T) {
	router, sessions, _ := setupAdminRouterWithRepo()

	registerAndGetToken(t, router)
	adminToken := createSessionWithGroup(sessions, 1, 1)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/9999/reset-passkey", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleResetPasskey_NonAdmin(t *testing.T) {
	router, sessions, _ := setupAdminRouterWithRepo()

	registerAndGetToken(t, router)
	registerAndGetToken(t, router)

	userToken := createSessionWithGroup(sessions, 2, 5)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/1/reset-passkey", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}
