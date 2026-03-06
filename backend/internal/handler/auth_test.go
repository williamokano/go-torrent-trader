package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/handler"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// mockUserRepo is an in-memory user repository for handler tests.
type mockUserRepo struct {
	mu     sync.Mutex
	users  []*model.User
	nextID int64
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{nextID: 1}
}

func (m *mockUserRepo) GetByID(_ context.Context, id int64) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *mockUserRepo) GetByUsername(_ context.Context, username string) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.Username == username {
			return u, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *mockUserRepo) GetByEmail(_ context.Context, email string) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *mockUserRepo) GetByPasskey(_ context.Context, _ string) (*model.User, error) {
	return nil, errors.New("not found")
}

func (m *mockUserRepo) Count(_ context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return int64(len(m.users)), nil
}

func (m *mockUserRepo) Create(_ context.Context, user *model.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	user.ID = m.nextID
	m.nextID++
	m.users = append(m.users, user)
	return nil
}

func (m *mockUserRepo) Update(_ context.Context, user *model.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, u := range m.users {
		if u.ID == user.ID {
			m.users[i] = user
			return nil
		}
	}
	return errors.New("not found")
}

func (m *mockUserRepo) IncrementStats(_ context.Context, id int64, uploadedDelta, downloadedDelta int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			u.Uploaded += uploadedDelta
			u.Downloaded += downloadedDelta
			return nil
		}
	}
	return errors.New("not found")
}

func setupRouter() (*handler.AuthHandler, *service.SessionStore, http.Handler) {
	repo := newMockUserRepo()
	sessions := service.NewSessionStore()
	authSvc := service.NewAuthService(repo, sessions)
	return handler.NewAuthHandler(authSvc), sessions, handler.NewRouter(&handler.Deps{
		AuthService:  authSvc,
		SessionStore: sessions,
	})
}

func TestHandleRegister_Success(t *testing.T) {
	_, _, router := setupRouter()

	body, _ := json.Marshal(map[string]string{
		"username": "testuser",
		"email":    "test@example.com",
		"password": "password123",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if resp["user"] == nil {
		t.Error("expected user in response")
	}
	if resp["tokens"] == nil {
		t.Error("expected tokens in response")
	}
}

func TestHandleRegister_InvalidBody(t *testing.T) {
	_, _, router := setupRouter()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleRegister_ValidationError(t *testing.T) {
	_, _, router := setupRouter()

	body, _ := json.Marshal(map[string]string{
		"username": "ab",
		"email":    "test@example.com",
		"password": "password123",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

func TestHandleLogin_Success(t *testing.T) {
	_, _, router := setupRouter()

	// Register first
	regBody, _ := json.Marshal(map[string]string{
		"username": "loginuser",
		"email":    "login@example.com",
		"password": "password123",
	})
	regReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(regBody))
	regReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), regReq)

	// Login
	loginBody, _ := json.Marshal(map[string]string{
		"username": "loginuser",
		"password": "password123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleLogin_WrongPassword(t *testing.T) {
	_, _, router := setupRouter()

	// Register
	regBody, _ := json.Marshal(map[string]string{
		"username": "wrongpw",
		"email":    "wrong@example.com",
		"password": "password123",
	})
	regReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(regBody))
	regReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), regReq)

	// Login with wrong password
	loginBody, _ := json.Marshal(map[string]string{
		"username": "wrongpw",
		"password": "wrongpassword",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestHandleRefresh_Success(t *testing.T) {
	_, _, router := setupRouter()

	// Register to get tokens
	regBody, _ := json.Marshal(map[string]string{
		"username": "refreshuser",
		"email":    "refresh@example.com",
		"password": "password123",
	})
	regReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(regBody))
	regReq.Header.Set("Content-Type", "application/json")
	regRec := httptest.NewRecorder()
	router.ServeHTTP(regRec, regReq)

	var regResp map[string]interface{}
	_ = json.Unmarshal(regRec.Body.Bytes(), &regResp)
	tokens := regResp["tokens"].(map[string]interface{})
	refreshToken := tokens["refresh_token"].(string)

	// Refresh
	refreshBody, _ := json.Marshal(map[string]string{
		"refresh_token": refreshToken,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(refreshBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleLogout(t *testing.T) {
	_, _, router := setupRouter()

	// Register to get tokens
	regBody, _ := json.Marshal(map[string]string{
		"username": "logoutuser",
		"email":    "logout@example.com",
		"password": "password123",
	})
	regReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(regBody))
	regReq.Header.Set("Content-Type", "application/json")
	regRec := httptest.NewRecorder()
	router.ServeHTTP(regRec, regReq)

	var regResp map[string]interface{}
	_ = json.Unmarshal(regRec.Body.Bytes(), &regResp)
	tokens := regResp["tokens"].(map[string]interface{})
	accessToken := tokens["access_token"].(string)

	// Logout
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
}

func TestHandleMe_Authenticated(t *testing.T) {
	_, _, router := setupRouter()

	// Register
	regBody, _ := json.Marshal(map[string]string{
		"username": "meuser",
		"email":    "me@example.com",
		"password": "password123",
	})
	regReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(regBody))
	regReq.Header.Set("Content-Type", "application/json")
	regRec := httptest.NewRecorder()
	router.ServeHTTP(regRec, regReq)

	var regResp map[string]interface{}
	_ = json.Unmarshal(regRec.Body.Bytes(), &regResp)
	tokens := regResp["tokens"].(map[string]interface{})
	accessToken := tokens["access_token"].(string)

	// GET /auth/me
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var meResp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &meResp)
	user := meResp["user"].(map[string]interface{})
	if user["username"] != "meuser" {
		t.Errorf("expected username meuser, got %v", user["username"])
	}
}

func TestHandleMe_Unauthenticated(t *testing.T) {
	_, _, router := setupRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestHealthzStillWorks(t *testing.T) {
	_, _, router := setupRouter()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}
