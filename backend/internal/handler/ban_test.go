package handler_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/handler"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/williamokano/go-torrent-trader/backend/internal/testutil"
)

// mockBanRepo is an in-memory BanRepository for handler tests.
type mockBanRepo struct {
	mu          sync.Mutex
	emailBans   []model.BannedEmail
	ipBans      []model.BannedIP
	nextEmailID int64
	nextIPID    int64
}

func newMockBanRepo() *mockBanRepo {
	return &mockBanRepo{nextEmailID: 1, nextIPID: 1}
}

func (m *mockBanRepo) CreateEmailBan(_ context.Context, ban *model.BannedEmail) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ban.ID = m.nextEmailID
	m.nextEmailID++
	m.emailBans = append(m.emailBans, *ban)
	return nil
}

func (m *mockBanRepo) DeleteEmailBan(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, b := range m.emailBans {
		if b.ID == id {
			m.emailBans = append(m.emailBans[:i], m.emailBans[i+1:]...)
			return nil
		}
	}
	return sql.ErrNoRows
}

func (m *mockBanRepo) ListEmailBans(_ context.Context) ([]model.BannedEmail, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]model.BannedEmail, len(m.emailBans))
	copy(result, m.emailBans)
	return result, nil
}

func (m *mockBanRepo) IsEmailBanned(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (m *mockBanRepo) CreateIPBan(_ context.Context, ban *model.BannedIP) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ban.ID = m.nextIPID
	m.nextIPID++
	m.ipBans = append(m.ipBans, *ban)
	return nil
}

func (m *mockBanRepo) DeleteIPBan(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, b := range m.ipBans {
		if b.ID == id {
			m.ipBans = append(m.ipBans[:i], m.ipBans[i+1:]...)
			return nil
		}
	}
	return sql.ErrNoRows
}

func (m *mockBanRepo) ListIPBans(_ context.Context) ([]model.BannedIP, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]model.BannedIP, len(m.ipBans))
	copy(result, m.ipBans)
	return result, nil
}

func (m *mockBanRepo) IsIPBanned(_ context.Context, ip string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false, nil
	}
	for _, b := range m.ipBans {
		_, cidr, err := net.ParseCIDR(b.IPRange)
		if err != nil {
			continue
		}
		if cidr.Contains(parsedIP) {
			return true, nil
		}
	}
	return false, nil
}

func setupBanRouter() (http.Handler, *service.BanService) {
	userRepo := newMockUserRepo()
	sessions := testutil.NewMemorySessionStore()
	bus := event.NewInMemoryBus()
	groupRepo := &mockGroupRepo{}

	authSvc := service.NewAuthServiceWithTTL(userRepo, sessions, testutil.NewMemoryPasswordResetStore(), &testutil.NoopSender{}, "http://localhost:8080", service.DefaultAccessTokenTTL, service.DefaultRefreshTokenTTL, groupRepo, bus)

	banRepo := newMockBanRepo()
	banSvc := service.NewBanService(banRepo, bus)
	adminSvc := service.NewAdminService(userRepo, groupRepo, bus)

	router := handler.NewRouter(&handler.Deps{
		AuthService:  authSvc,
		SessionStore: sessions,
		AdminService: adminSvc,
		BanService:   banSvc,
	})

	return router, banSvc
}

func registerAdminUser(t *testing.T, router http.Handler) string {
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
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	tokens := resp["tokens"].(map[string]interface{})
	return tokens["access_token"].(string)
}

func TestHandleListEmailBans(t *testing.T) {
	router, banSvc := setupBanRouter()
	token := registerAdminUser(t, router)

	// Add a ban via service
	_ = banSvc.BanEmail(context.Background(), 1, "admin", &model.BannedEmail{Pattern: "%@spam.com"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/bans/emails", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	bans, ok := resp["email_bans"].([]interface{})
	if !ok {
		t.Fatalf("expected email_bans array in response")
	}
	if len(bans) != 1 {
		t.Errorf("expected 1 email ban, got %d", len(bans))
	}
}

func TestHandleCreateEmailBan(t *testing.T) {
	router, _ := setupBanRouter()
	token := registerAdminUser(t, router)

	body, _ := json.Marshal(map[string]interface{}{
		"pattern": "%@mailinator.com",
		"reason":  "disposable email provider",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/bans/emails", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCreateEmailBan_MissingPattern(t *testing.T) {
	router, _ := setupBanRouter()
	token := registerAdminUser(t, router)

	body, _ := json.Marshal(map[string]interface{}{
		"reason": "no pattern",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/bans/emails", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteEmailBan(t *testing.T) {
	router, banSvc := setupBanRouter()
	token := registerAdminUser(t, router)

	ban := &model.BannedEmail{Pattern: "%@spam.com"}
	_ = banSvc.BanEmail(context.Background(), 1, "admin", ban)

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/admin/bans/emails/%d", ban.ID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteEmailBan_NotFound(t *testing.T) {
	router, _ := setupBanRouter()
	token := registerAdminUser(t, router)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/bans/emails/999", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleListIPBans(t *testing.T) {
	router, banSvc := setupBanRouter()
	token := registerAdminUser(t, router)

	_ = banSvc.BanIP(context.Background(), 1, "admin", &model.BannedIP{IPRange: "10.0.0.0/8"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/bans/ips", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	bans, ok := resp["ip_bans"].([]interface{})
	if !ok {
		t.Fatalf("expected ip_bans array in response")
	}
	if len(bans) != 1 {
		t.Errorf("expected 1 IP ban, got %d", len(bans))
	}
}

func TestHandleCreateIPBan(t *testing.T) {
	router, _ := setupBanRouter()
	token := registerAdminUser(t, router)

	body, _ := json.Marshal(map[string]interface{}{
		"ip_range": "10.0.0.0/8",
		"reason":   "known VPN range",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/bans/ips", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCreateIPBan_MissingRange(t *testing.T) {
	router, _ := setupBanRouter()
	token := registerAdminUser(t, router)

	body, _ := json.Marshal(map[string]interface{}{
		"reason": "no range",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/bans/ips", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteIPBan(t *testing.T) {
	router, banSvc := setupBanRouter()
	token := registerAdminUser(t, router)

	ban := &model.BannedIP{IPRange: "10.0.0.0/8"}
	_ = banSvc.BanIP(context.Background(), 1, "admin", ban)

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/admin/bans/ips/%d", ban.ID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteIPBan_NotFound(t *testing.T) {
	router, _ := setupBanRouter()
	token := registerAdminUser(t, router)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/bans/ips/999", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestBanEndpoints_RequireAdmin(t *testing.T) {
	userRepo := newMockUserRepo()
	sessions := testutil.NewMemorySessionStore()
	bus := event.NewInMemoryBus()
	groupRepo := &mockGroupRepo{}

	authSvc := service.NewAuthServiceWithTTL(userRepo, sessions, testutil.NewMemoryPasswordResetStore(), &testutil.NoopSender{}, "http://localhost:8080", service.DefaultAccessTokenTTL, service.DefaultRefreshTokenTTL, groupRepo, bus)
	banSvc := service.NewBanService(newMockBanRepo(), bus)
	adminSvc := service.NewAdminService(userRepo, groupRepo, bus)

	router := handler.NewRouter(&handler.Deps{
		AuthService:  authSvc,
		SessionStore: sessions,
		AdminService: adminSvc,
		BanService:   banSvc,
	})

	// Register first user (admin)
	regBody, _ := json.Marshal(map[string]string{
		"username": "admin1",
		"email":    "admin1@example.com",
		"password": "password123",
	})
	regReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(regBody))
	regReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), regReq)

	// Register second user (regular user)
	regBody2, _ := json.Marshal(map[string]string{
		"username": "regularuser",
		"email":    "regular@example.com",
		"password": "password123",
	})
	regReq2 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(regBody2))
	regReq2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, regReq2)

	var resp2 map[string]interface{}
	_ = json.Unmarshal(rec2.Body.Bytes(), &resp2)
	tokens2 := resp2["tokens"].(map[string]interface{})
	userToken := tokens2["access_token"].(string)

	// Regular user should get 403
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/bans/emails", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-admin user, got %d", rec.Code)
	}
}
