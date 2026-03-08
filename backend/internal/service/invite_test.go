package service

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// --- mock invite repo ---

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

// --- mock user repo for invite tests ---

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

func (m *mockInviteUserRepo) GetByUsername(_ context.Context, _ string) (*model.User, error) {
	return nil, errors.New("not found")
}

func (m *mockInviteUserRepo) GetByEmail(_ context.Context, _ string) (*model.User, error) {
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
	return nil, 0, nil
}

func (m *mockInviteUserRepo) ListStaff(_ context.Context) ([]model.User, error) {
	return nil, nil
}

func (m *mockInviteUserRepo) UpdateLastAccess(_ context.Context, _ int64) error { return nil }

// --- tests ---

func newTestInviteService() (*InviteService, *mockInviteRepo, *mockInviteUserRepo) {
	inviteRepo := newMockInviteRepo()
	userRepo := newMockInviteUserRepo()
	bus := event.NewInMemoryBus()
	svc := NewInviteService(inviteRepo, userRepo, bus)

	// Create a test user with invites
	_ = userRepo.Create(context.Background(), &model.User{
		Username: "inviter",
		Email:    "inviter@example.com",
		Invites:  3,
		Enabled:  true,
		GroupID:  5,
	})

	return svc, inviteRepo, userRepo
}

func TestInviteService_CreateInvite_Success(t *testing.T) {
	svc, _, userRepo := newTestInviteService()

	invite, err := svc.CreateInvite(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if invite.ID != 1 {
		t.Errorf("expected ID 1, got %d", invite.ID)
	}
	if invite.Token == "" {
		t.Error("expected non-empty token")
	}

	// Check invite count decremented
	user, _ := userRepo.GetByID(context.Background(), 1)
	if user.Invites != 2 {
		t.Errorf("expected 2 invites remaining, got %d", user.Invites)
	}
}

func TestInviteService_CreateInvite_NoInvitesRemaining(t *testing.T) {
	svc, _, userRepo := newTestInviteService()

	// Set invites to 0
	user, _ := userRepo.GetByID(context.Background(), 1)
	user.Invites = 0
	_ = userRepo.Update(context.Background(), user)

	_, err := svc.CreateInvite(context.Background(), 1)
	if !errors.Is(err, ErrNoInvitesRemaining) {
		t.Errorf("expected ErrNoInvitesRemaining, got %v", err)
	}
}

func TestInviteService_ValidateInvite_Success(t *testing.T) {
	svc, _, _ := newTestInviteService()

	// Create an invite first
	invite, err := svc.CreateInvite(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error creating invite: %v", err)
	}

	// Validate the token
	result, err := svc.ValidateInvite(context.Background(), invite.Token)
	if err != nil {
		t.Fatalf("unexpected error validating invite: %v", err)
	}
	if result.ID != invite.ID {
		t.Errorf("expected invite ID %d, got %d", invite.ID, result.ID)
	}
}

func TestInviteService_ValidateInvite_NotFound(t *testing.T) {
	svc, _, _ := newTestInviteService()

	_, err := svc.ValidateInvite(context.Background(), "nonexistent-token")
	if !errors.Is(err, ErrInviteNotFound) {
		t.Errorf("expected ErrInviteNotFound, got %v", err)
	}
}

func TestInviteService_ValidateInvite_Expired(t *testing.T) {
	svc, inviteRepo, _ := newTestInviteService()

	invite, err := svc.CreateInvite(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Manually expire it
	inviteRepo.mu.Lock()
	for _, inv := range inviteRepo.invites {
		if inv.Token == invite.Token {
			inv.ExpiresAt = time.Now().Add(-1 * time.Hour)
		}
	}
	inviteRepo.mu.Unlock()

	_, err = svc.ValidateInvite(context.Background(), invite.Token)
	if !errors.Is(err, ErrInviteExpired) {
		t.Errorf("expected ErrInviteExpired, got %v", err)
	}
}

func TestInviteService_ValidateInvite_AlreadyRedeemed(t *testing.T) {
	svc, inviteRepo, _ := newTestInviteService()

	invite, err := svc.CreateInvite(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Mark as redeemed
	inviteRepo.mu.Lock()
	for _, inv := range inviteRepo.invites {
		if inv.Token == invite.Token {
			inviteeID := int64(99)
			inv.InviteeID = &inviteeID
			inv.Redeemed = true
		}
	}
	inviteRepo.mu.Unlock()

	_, err = svc.ValidateInvite(context.Background(), invite.Token)
	if !errors.Is(err, ErrInviteRedeemed) {
		t.Errorf("expected ErrInviteRedeemed, got %v", err)
	}
}

func TestInviteService_RedeemInvite_Success(t *testing.T) {
	svc, _, _ := newTestInviteService()

	invite, err := svc.CreateInvite(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error creating invite: %v", err)
	}

	result, err := svc.RedeemInvite(context.Background(), invite.Token, 42)
	if err != nil {
		t.Fatalf("unexpected error redeeming invite: %v", err)
	}
	if result.ID != invite.ID {
		t.Errorf("expected invite ID %d, got %d", invite.ID, result.ID)
	}
}

func TestInviteService_ListMyInvites(t *testing.T) {
	svc, _, _ := newTestInviteService()

	// Create some invites
	_, _ = svc.CreateInvite(context.Background(), 1)
	_, _ = svc.CreateInvite(context.Background(), 1)

	invites, total, err := svc.ListMyInvites(context.Background(), 1, 1, 25)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Errorf("expected total 2, got %d", total)
	}
	if len(invites) != 2 {
		t.Errorf("expected 2 invites, got %d", len(invites))
	}
}

func TestInviteService_ListMyInvites_Pagination(t *testing.T) {
	svc, _, _ := newTestInviteService()

	// Create 3 invites (user has 3 invites)
	_, _ = svc.CreateInvite(context.Background(), 1)
	_, _ = svc.CreateInvite(context.Background(), 1)
	_, _ = svc.CreateInvite(context.Background(), 1)

	invites, total, err := svc.ListMyInvites(context.Background(), 1, 1, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}
	if len(invites) != 2 {
		t.Errorf("expected 2 invites on page 1, got %d", len(invites))
	}
}

func TestInviteService_CreateInvite_DecrementsInviteCount(t *testing.T) {
	svc, _, userRepo := newTestInviteService()

	user, _ := userRepo.GetByID(context.Background(), 1)
	initialInvites := user.Invites

	_, _ = svc.CreateInvite(context.Background(), 1)
	_, _ = svc.CreateInvite(context.Background(), 1)

	user, _ = userRepo.GetByID(context.Background(), 1)
	if user.Invites != initialInvites-2 {
		t.Errorf("expected %d invites, got %d", initialInvites-2, user.Invites)
	}
}
