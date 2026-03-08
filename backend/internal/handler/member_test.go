package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/handler"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/williamokano/go-torrent-trader/backend/internal/testutil"
)

func setupMemberRouter() (http.Handler, *mockUserRepo) {
	repo := newMockUserRepo()
	sessions := testutil.NewMemorySessionStore()
	groupRepo := &mockGroupRepo{}
	authSvc := service.NewAuthService(repo, sessions, testutil.NewMemoryPasswordResetStore(), &testutil.NoopSender{}, "http://localhost:8080", event.NewInMemoryBus())
	userSvc := service.NewUserService(repo, sessions, groupRepo, nil, nil)
	memberSvc := service.NewMemberService(repo, groupRepo)

	router := handler.NewRouter(&handler.Deps{
		AuthService:   authSvc,
		SessionStore:  sessions,
		UserService:   userSvc,
		MemberService: memberSvc,
	})

	return router, repo
}

func memberRegisterAndGetToken(t *testing.T, router http.Handler, username, email string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"username": username,
		"email":    email,
		"password": "password123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("registration failed: %d %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	tokens, ok := resp["tokens"].(map[string]interface{})
	if !ok {
		t.Fatal("no tokens in response")
	}
	return tokens["access_token"].(string)
}

func TestHandleListMembers_Success(t *testing.T) {
	router, _ := setupMemberRouter()
	token := memberRegisterAndGetToken(t, router, "alice", "alice@test.com")
	_ = memberRegisterAndGetToken(t, router, "bob", "bob@test.com")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users?page=1&per_page=25", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	users, ok := resp["users"].([]interface{})
	if !ok {
		t.Fatal("expected users array")
	}
	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}
}

func TestHandleListMembers_RequiresAuth(t *testing.T) {
	router, _ := setupMemberRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestHandleListMembers_WithSearch(t *testing.T) {
	router, _ := setupMemberRouter()
	token := memberRegisterAndGetToken(t, router, "alice", "alice@test.com")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users?search=alice", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleStaff_Success(t *testing.T) {
	router, repo := setupMemberRouter()
	token := memberRegisterAndGetToken(t, router, "admin", "admin@test.com")

	// Seed a staff user (group_id=1 = Administrator in mockGroupRepo)
	repo.mu.Lock()
	repo.users = append(repo.users, &model.User{
		ID:        100,
		Username:  "staffadmin",
		GroupID:   1,
		Enabled:   true,
		CreatedAt: time.Now(),
	})
	repo.mu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/staff", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	staff, ok := resp["staff"].([]interface{})
	if !ok {
		t.Fatal("expected staff array")
	}
	// The mockUserRepo.ListStaff returns ALL users (no filtering in mock),
	// so we just check the endpoint returns successfully with a staff array.
	if len(staff) == 0 {
		t.Error("expected at least one staff member")
	}
}

func TestHandleStaff_RequiresAuth(t *testing.T) {
	router, _ := setupMemberRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/staff", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestStaffRouteDoesNotConflictWithUserID(t *testing.T) {
	router, _ := setupMemberRouter()
	token := memberRegisterAndGetToken(t, router, "testuser", "test@test.com")

	// /users/staff should NOT be matched as /users/{id} with id="staff"
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/staff", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// Should get 200 from staff endpoint, not a 400 "invalid user ID" from profile endpoint
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 from staff endpoint, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	// Should have "staff" key, not "user" key
	if _, ok := resp["staff"]; !ok {
		t.Error("expected response to contain 'staff' key (from staff endpoint), not user profile")
	}
}
