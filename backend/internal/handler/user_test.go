package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/handler"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/williamokano/go-torrent-trader/backend/internal/testutil"
)

func setupUserRouter() (service.SessionStore, http.Handler) {
	repo := newMockUserRepo()
	sessions := testutil.NewMemorySessionStore()
	authSvc := service.NewAuthService(repo, sessions, testutil.NewMemoryPasswordResetStore(), &testutil.NoopSender{}, "http://localhost:8080", event.NewInMemoryBus())
	userSvc := service.NewUserService(repo, sessions, nil, nil, nil)
	router := handler.NewRouter(&handler.Deps{
		AuthService:  authSvc,
		SessionStore: sessions,
		UserService:  userSvc,
	})
	return sessions, router
}

// registerUserAndGetToken is a helper that registers a user and returns the access token and user ID.
func registerUserAndGetToken(t *testing.T, router http.Handler, username, email, password string) (string, float64) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"username": username,
		"email":    email,
		"password": password,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("register failed: %d %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	tokens := resp["tokens"].(map[string]interface{})
	user := resp["user"].(map[string]interface{})
	return tokens["access_token"].(string), user["id"].(float64)
}

func TestHandleGetProfile_PublicView(t *testing.T) {
	_, router := setupUserRouter()

	_, userID := registerUserAndGetToken(t, router, "target", "target@example.com", "password123")
	viewerToken, _ := registerUserAndGetToken(t, router, "viewer", "viewer@example.com", "password123")

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/users/%d", int64(userID)), nil)
	req.Header.Set("Authorization", "Bearer "+viewerToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	user := resp["user"].(map[string]interface{})

	if user["username"] != "target" {
		t.Errorf("expected username target, got %v", user["username"])
	}
	// Public view should NOT have email
	if _, hasEmail := user["email"]; hasEmail {
		t.Error("public view should not include email")
	}
}

func TestHandleGetProfile_OwnerView(t *testing.T) {
	_, router := setupUserRouter()

	token, userID := registerUserAndGetToken(t, router, "owner", "owner@example.com", "password123")

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/users/%d", int64(userID)), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	user := resp["user"].(map[string]interface{})

	if user["email"] != "owner@example.com" {
		t.Errorf("owner view should include email, got %v", user["email"])
	}
	if _, hasPasskey := user["passkey"]; !hasPasskey {
		t.Error("owner view should include passkey")
	}
}

func TestHandleGetProfile_NotFound(t *testing.T) {
	_, router := setupUserRouter()

	token, _ := registerUserAndGetToken(t, router, "finder", "finder@example.com", "password123")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/9999", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandleGetProfile_InvalidID(t *testing.T) {
	_, router := setupUserRouter()

	token, _ := registerUserAndGetToken(t, router, "badid", "badid@example.com", "password123")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/abc", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleGetProfile_Unauthenticated(t *testing.T) {
	_, router := setupUserRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestHandleUpdateProfile_Success(t *testing.T) {
	_, router := setupUserRouter()

	token, _ := registerUserAndGetToken(t, router, "updater", "updater@example.com", "password123")

	body, _ := json.Marshal(map[string]string{
		"avatar": "https://example.com/avatar.jpg",
		"title":  "New Title",
		"info":   "New bio",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/users/me/profile", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	user := resp["user"].(map[string]interface{})

	if user["avatar"] != "https://example.com/avatar.jpg" {
		t.Errorf("expected avatar update, got %v", user["avatar"])
	}
	if user["title"] != "New Title" {
		t.Errorf("expected title update, got %v", user["title"])
	}
}

func TestHandleUpdateProfile_InvalidAvatarURL(t *testing.T) {
	_, router := setupUserRouter()

	token, _ := registerUserAndGetToken(t, router, "badurl", "badurl@example.com", "password123")

	body, _ := json.Marshal(map[string]string{
		"avatar": "not-a-url",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/users/me/profile", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

func TestHandleUpdateProfile_InvalidBody(t *testing.T) {
	_, router := setupUserRouter()

	token, _ := registerUserAndGetToken(t, router, "badbody", "badbody@example.com", "password123")

	req := httptest.NewRequest(http.MethodPut, "/api/v1/users/me/profile", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleChangePassword_Success(t *testing.T) {
	_, router := setupUserRouter()

	token, _ := registerUserAndGetToken(t, router, "chpw", "chpw@example.com", "oldpassword1")

	body, _ := json.Marshal(map[string]string{
		"current_password": "oldpassword1",
		"new_password":     "newpassword1",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/users/me/password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Verify can login with new password
	loginBody, _ := json.Marshal(map[string]string{
		"username": "chpw",
		"password": "newpassword1",
	})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	router.ServeHTTP(loginRec, loginReq)

	if loginRec.Code != http.StatusOK {
		t.Errorf("expected login with new password to succeed, got %d", loginRec.Code)
	}
}

func TestHandleChangePassword_WrongCurrent(t *testing.T) {
	_, router := setupUserRouter()

	token, _ := registerUserAndGetToken(t, router, "wrongcur", "wrongcur@example.com", "password123")

	body, _ := json.Marshal(map[string]string{
		"current_password": "wrongpassword",
		"new_password":     "newpassword1",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/users/me/password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestHandleChangePassword_WeakPassword(t *testing.T) {
	_, router := setupUserRouter()

	token, _ := registerUserAndGetToken(t, router, "weakpw", "weakpw@example.com", "password123")

	body, _ := json.Marshal(map[string]string{
		"current_password": "password123",
		"new_password":     "short",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/users/me/password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

func TestHandleRegeneratePasskey_Success(t *testing.T) {
	_, router := setupUserRouter()

	token, _ := registerUserAndGetToken(t, router, "regenkey", "regenkey@example.com", "password123")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/passkey", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	passkey, ok := resp["passkey"].(string)
	if !ok || len(passkey) != 32 {
		t.Errorf("expected 32-char passkey, got %q", passkey)
	}
}

func TestHandleRegeneratePasskey_Unauthenticated(t *testing.T) {
	_, router := setupUserRouter()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/passkey", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestHandleMe_ReturnsFullProfile(t *testing.T) {
	_, router := setupUserRouter()

	token, _ := registerUserAndGetToken(t, router, "meprofile", "meprofile@example.com", "password123")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	user := resp["user"].(map[string]interface{})

	// Should have owner-only fields
	if _, hasEmail := user["email"]; !hasEmail {
		t.Error("/auth/me should include email")
	}
	if _, hasPasskey := user["passkey"]; !hasPasskey {
		t.Error("/auth/me should include passkey")
	}
	if _, hasRatio := user["ratio"]; !hasRatio {
		t.Error("/auth/me should include ratio")
	}
}
