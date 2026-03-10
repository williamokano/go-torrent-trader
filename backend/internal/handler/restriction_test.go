package handler_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/handler"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/williamokano/go-torrent-trader/backend/internal/testutil"
)

// --- mock restriction repo ---

type mockRestrictionRepoHandler struct {
	mu           sync.Mutex
	restrictions []*model.Restriction
	nextID       int64
}

func newMockRestrictionRepoHandler() *mockRestrictionRepoHandler {
	return &mockRestrictionRepoHandler{nextID: 1}
}

func (m *mockRestrictionRepoHandler) Create(_ context.Context, r *model.Restriction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r.ID = m.nextID
	m.nextID++
	r.CreatedAt = time.Now()
	cp := *r
	m.restrictions = append(m.restrictions, &cp)
	return nil
}

func (m *mockRestrictionRepoHandler) GetByID(_ context.Context, id int64) (*model.Restriction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.restrictions {
		if r.ID == id {
			cp := *r
			return &cp, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (m *mockRestrictionRepoHandler) ListByUser(_ context.Context, userID int64) ([]model.Restriction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []model.Restriction
	for _, r := range m.restrictions {
		if r.UserID == userID {
			result = append(result, *r)
		}
	}
	return result, nil
}

func (m *mockRestrictionRepoHandler) ListActive(_ context.Context) ([]model.Restriction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []model.Restriction
	for _, r := range m.restrictions {
		if r.LiftedAt == nil {
			result = append(result, *r)
		}
	}
	return result, nil
}

func (m *mockRestrictionRepoHandler) Lift(_ context.Context, id int64, liftedBy *int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.restrictions {
		if r.ID == id && r.LiftedAt == nil {
			now := time.Now()
			r.LiftedAt = &now
			r.LiftedBy = liftedBy
			return nil
		}
	}
	return sql.ErrNoRows
}

func (m *mockRestrictionRepoHandler) LiftExpired(_ context.Context) ([]model.Restriction, error) {
	return nil, nil
}

func (m *mockRestrictionRepoHandler) HasActiveByType(_ context.Context, userID int64, restrictionType string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.restrictions {
		if r.UserID == userID && r.RestrictionType == restrictionType && r.LiftedAt == nil {
			return true, nil
		}
	}
	return false, nil
}

// --- setup ---

func setupRestrictionRouter() (http.Handler, *service.RestrictionService) {
	userRepo := newMockUserRepo()
	sessions := testutil.NewMemorySessionStore()
	bus := event.NewInMemoryBus()
	groupRepo := &mockGroupRepo{}

	authSvc := service.NewAuthServiceWithTTL(userRepo, sessions, testutil.NewMemoryPasswordResetStore(), &testutil.NoopSender{}, "http://localhost:8080", service.DefaultAccessTokenTTL, service.DefaultRefreshTokenTTL, groupRepo, bus)

	restrictionRepo := newMockRestrictionRepoHandler()
	restrictionSvc := service.NewRestrictionService(restrictionRepo, userRepo, bus)
	adminSvc := service.NewAdminService(userRepo, groupRepo, bus)

	router := handler.NewRouter(&handler.Deps{
		AuthService:        authSvc,
		SessionStore:       sessions,
		AdminService:       adminSvc,
		RestrictionService: restrictionSvc,
	})

	return router, restrictionSvc
}

func registerRestrictionAdmin(t *testing.T, router http.Handler) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"username": "adminuser",
		"email":    "admin@example.com",
		"password": "password123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("register: expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	tokens := resp["tokens"].(map[string]interface{})
	return tokens["access_token"].(string)
}

func registerRestrictionUser(t *testing.T, router http.Handler) (string, float64) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"username": "regularuser",
		"email":    "user@example.com",
		"password": "password123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("register user: expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	tokens := resp["tokens"].(map[string]interface{})
	user := resp["user"].(map[string]interface{})
	return tokens["access_token"].(string), user["id"].(float64)
}

// --- tests ---

func TestHandleSetRestrictions_Apply(t *testing.T) {
	router, _ := setupRestrictionRouter()
	adminToken := registerRestrictionAdmin(t, router)
	_, userID := registerRestrictionUser(t, router)

	body, _ := json.Marshal(map[string]interface{}{
		"can_download": false,
		"reason":       "bad ratio",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/"+formatID(userID)+"/restrictions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleSetRestrictions_SelfRestriction(t *testing.T) {
	router, _ := setupRestrictionRouter()
	adminToken := registerRestrictionAdmin(t, router)

	// Admin user is always ID 1 (first registered user).
	body, _ := json.Marshal(map[string]interface{}{
		"can_download": false,
		"reason":       "testing self",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/1/restrictions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for self-restriction, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleSetRestrictions_Lift(t *testing.T) {
	router, restrictionSvc := setupRestrictionRouter()
	adminToken := registerRestrictionAdmin(t, router)
	_, userID := registerRestrictionUser(t, router)

	// Apply a restriction via service directly.
	adminID := int64(1)
	_, err := restrictionSvc.ApplyRestriction(context.Background(), int64(userID), model.RestrictionTypeDownload, "test", nil, &adminID)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	// Now lift via API.
	body, _ := json.Marshal(map[string]interface{}{
		"can_download": true,
		"reason":       "restored",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/"+formatID(userID)+"/restrictions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleListRestrictions(t *testing.T) {
	router, restrictionSvc := setupRestrictionRouter()
	adminToken := registerRestrictionAdmin(t, router)
	_, userID := registerRestrictionUser(t, router)

	// Apply some restrictions.
	adminID := int64(1)
	_, _ = restrictionSvc.ApplyRestriction(context.Background(), int64(userID), model.RestrictionTypeDownload, "reason1", nil, &adminID)
	_, _ = restrictionSvc.ApplyRestriction(context.Background(), int64(userID), model.RestrictionTypeUpload, "reason2", nil, &adminID)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/"+formatID(userID)+"/restrictions", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	restrictions, ok := resp["restrictions"].([]interface{})
	if !ok {
		t.Fatal("expected restrictions array in response")
	}
	if len(restrictions) != 2 {
		t.Errorf("expected 2 restrictions, got %d", len(restrictions))
	}
}

func TestHandleSetRestrictions_MissingReason(t *testing.T) {
	router, _ := setupRestrictionRouter()
	adminToken := registerRestrictionAdmin(t, router)
	_, userID := registerRestrictionUser(t, router)

	body, _ := json.Marshal(map[string]interface{}{
		"can_download": false,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/"+formatID(userID)+"/restrictions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing reason, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleSetRestrictions_DatetimeLocalFormat(t *testing.T) {
	router, _ := setupRestrictionRouter()
	adminToken := registerRestrictionAdmin(t, router)
	_, userID := registerRestrictionUser(t, router)

	// Use datetime-local format (no timezone, no seconds).
	body, _ := json.Marshal(map[string]interface{}{
		"can_download": false,
		"reason":       "test expiry",
		"expires_at":   "2027-06-15T14:30",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/"+formatID(userID)+"/restrictions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for datetime-local format, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func formatID(id float64) string {
	return fmt.Sprintf("%d", int64(id))
}
