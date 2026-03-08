package handler_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/handler"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/williamokano/go-torrent-trader/backend/internal/testutil"
)

// --- mock invite repo for handler tests ---

type mockInviteRepo struct {
	mu      sync.Mutex
	invites []*model.Invite
	nextID  int64
}

func newMockInviteRepo() *mockInviteRepo {
	return &mockInviteRepo{nextID: 1}
}

func (m *mockInviteRepo) Create(_ context.Context, invite *model.Invite) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	invite.ID = m.nextID
	invite.CreatedAt = time.Now()
	m.nextID++
	m.invites = append(m.invites, invite)
	return nil
}

func (m *mockInviteRepo) GetByToken(_ context.Context, token string) (*model.Invite, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, inv := range m.invites {
		if inv.Token == token {
			return inv, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (m *mockInviteRepo) ListByInviter(_ context.Context, inviterID int64, page, perPage int) ([]model.Invite, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var filtered []model.Invite
	for _, inv := range m.invites {
		if inv.InviterID == inviterID {
			filtered = append(filtered, *inv)
		}
	}

	total := int64(len(filtered))
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 25
	}
	start := (page - 1) * perPage
	if start >= len(filtered) {
		return nil, total, nil
	}
	end := start + perPage
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[start:end], total, nil
}

func (m *mockInviteRepo) Redeem(_ context.Context, token string, inviteeID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, inv := range m.invites {
		if inv.Token == token && inv.InviteeID == nil && time.Now().Before(inv.ExpiresAt) {
			inv.InviteeID = &inviteeID
			now := time.Now()
			inv.RedeemedAt = &now
			inv.Redeemed = true
			return nil
		}
	}
	return sql.ErrNoRows
}

func (m *mockInviteRepo) CountPendingByInviter(_ context.Context, inviterID int64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, inv := range m.invites {
		if inv.InviterID == inviterID && inv.InviteeID == nil && time.Now().Before(inv.ExpiresAt) {
			count++
		}
	}
	return count, nil
}

// --- mock user repo for invite handler tests ---

type mockInviteUserRepo struct {
	mu     sync.Mutex
	users  []*model.User
	nextID int64
}

func newMockInviteUserRepo() *mockInviteUserRepo {
	return &mockInviteUserRepo{nextID: 1}
}

func (m *mockInviteUserRepo) GetByID(_ context.Context, id int64) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *mockInviteUserRepo) GetByUsername(_ context.Context, username string) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.Username == username {
			return u, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *mockInviteUserRepo) GetByEmail(_ context.Context, email string) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *mockInviteUserRepo) GetByPasskey(_ context.Context, _ string) (*model.User, error) {
	return nil, errors.New("not found")
}

func (m *mockInviteUserRepo) Count(_ context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return int64(len(m.users)), nil
}

func (m *mockInviteUserRepo) Create(_ context.Context, user *model.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	user.ID = m.nextID
	m.nextID++
	m.users = append(m.users, user)
	return nil
}

func (m *mockInviteUserRepo) Update(_ context.Context, user *model.User) error {
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

func (m *mockInviteUserRepo) IncrementStats(_ context.Context, _ int64, _, _ int64) error {
	return nil
}

func (m *mockInviteUserRepo) List(_ context.Context, _ repository.ListUsersOptions) ([]model.User, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []model.User
	for _, u := range m.users {
		result = append(result, *u)
	}
	return result, int64(len(result)), nil
}

func (m *mockInviteUserRepo) ListStaff(_ context.Context) ([]model.User, error) {
	return nil, nil
}

// --- test helpers ---

func setupInviteRouter() (http.Handler, service.SessionStore, *mockInviteUserRepo) {
	userRepo := newMockInviteUserRepo()
	inviteRepo := newMockInviteRepo()
	sessions := testutil.NewMemorySessionStore()
	bus := event.NewInMemoryBus()
	authSvc := service.NewAuthServiceWithTTL(userRepo, sessions, testutil.NewMemoryPasswordResetStore(), &testutil.NoopSender{}, "http://localhost:8080", service.DefaultAccessTokenTTL, service.DefaultRefreshTokenTTL, &mockGroupRepo{}, bus)
	inviteSvc := service.NewInviteService(inviteRepo, userRepo, bus)

	router := handler.NewRouter(&handler.Deps{
		AuthService:   authSvc,
		SessionStore:  sessions,
		InviteService: inviteSvc,
	})
	return router, sessions, userRepo
}

// --- tests ---

func TestHandleCreateInvite_Success(t *testing.T) {
	router, sessions, userRepo := setupInviteRouter()

	// Create user with invites and admin group (which has CanInvite=true)
	_ = userRepo.Create(context.Background(), &model.User{
		Username: "inviter",
		Email:    "inviter@test.com",
		Invites:  5,
		Enabled:  true,
		GroupID:  1,
	})
	token := createSessionWithGroup(sessions, 1, 1)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/invites", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	invite := resp["invite"].(map[string]interface{})
	if invite["token"] == nil || invite["token"] == "" {
		t.Error("expected non-empty token in response")
	}
	if invite["status"] != "pending" {
		t.Errorf("expected status pending, got %v", invite["status"])
	}
}

func TestHandleCreateInvite_Unauthenticated(t *testing.T) {
	router, _, _ := setupInviteRouter()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/invites", nil)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestHandleCreateInvite_NoInviteCapability(t *testing.T) {
	router, sessions, userRepo := setupInviteRouter()

	// Regular user (groupID=5) does NOT have CanInvite
	_ = userRepo.Create(context.Background(), &model.User{
		Username: "regular",
		Email:    "regular@test.com",
		Invites:  5,
		Enabled:  true,
		GroupID:  5,
	})
	token := createSessionWithGroup(sessions, 2, 5)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/invites", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCreateInvite_NoInvitesRemaining(t *testing.T) {
	router, sessions, userRepo := setupInviteRouter()

	// Create user with 0 invites
	_ = userRepo.Create(context.Background(), &model.User{
		Username: "noinvites",
		Email:    "noinvites@test.com",
		Invites:  0,
		Enabled:  true,
		GroupID:  1,
	})
	token := createSessionWithGroup(sessions, 1, 1)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/invites", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleListInvites_Success(t *testing.T) {
	router, sessions, userRepo := setupInviteRouter()

	// Create user with invites
	_ = userRepo.Create(context.Background(), &model.User{
		Username: "lister",
		Email:    "lister@test.com",
		Invites:  5,
		Enabled:  true,
		GroupID:  1,
	})
	token := createSessionWithGroup(sessions, 1, 1)

	// Create an invite first
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/invites", nil)
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+token)
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create invite failed: %d %s", createRec.Code, createRec.Body.String())
	}

	// List invites
	req := httptest.NewRequest(http.MethodGet, "/api/v1/invites", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	invites := resp["invites"].([]interface{})
	if len(invites) != 1 {
		t.Errorf("expected 1 invite, got %d", len(invites))
	}
	total := resp["total"].(float64)
	if total != 1 {
		t.Errorf("expected total 1, got %v", total)
	}
}

func TestHandleValidateInvite_NotFound(t *testing.T) {
	router, _, _ := setupInviteRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/invites/no-such-token", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}
