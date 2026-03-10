package service

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// --- mock restriction repo ---

type mockRestrictionRepo struct {
	mu           sync.Mutex
	restrictions []*model.Restriction
	nextID       int64
}

func newMockRestrictionRepo() *mockRestrictionRepo {
	return &mockRestrictionRepo{nextID: 1}
}

func (m *mockRestrictionRepo) Create(_ context.Context, r *model.Restriction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r.ID = m.nextID
	m.nextID++
	r.CreatedAt = time.Now()
	cp := *r
	m.restrictions = append(m.restrictions, &cp)
	return nil
}

func (m *mockRestrictionRepo) GetByID(_ context.Context, id int64) (*model.Restriction, error) {
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

func (m *mockRestrictionRepo) ListByUser(_ context.Context, userID int64) ([]model.Restriction, error) {
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

func (m *mockRestrictionRepo) ListActive(_ context.Context) ([]model.Restriction, error) {
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

func (m *mockRestrictionRepo) Lift(_ context.Context, id int64, liftedBy *int64) error {
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

func (m *mockRestrictionRepo) LiftExpired(_ context.Context) ([]model.Restriction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	var lifted []model.Restriction
	for _, r := range m.restrictions {
		if r.LiftedAt == nil && r.ExpiresAt != nil && r.ExpiresAt.Before(now) {
			r.LiftedAt = &now
			lifted = append(lifted, *r)
		}
	}
	return lifted, nil
}

func (m *mockRestrictionRepo) HasActiveByType(_ context.Context, userID int64, restrictionType string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.restrictions {
		if r.UserID == userID && r.RestrictionType == restrictionType && r.LiftedAt == nil {
			return true, nil
		}
	}
	return false, nil
}

// --- mock user repo for restriction tests ---

type mockUserRepoForRestrictions struct {
	mu    sync.Mutex
	users map[int64]*model.User
}

func newMockUserRepoForRestrictions() *mockUserRepoForRestrictions {
	return &mockUserRepoForRestrictions{users: make(map[int64]*model.User)}
}

func (m *mockUserRepoForRestrictions) addUser(u *model.User) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[u.ID] = u
}

func (m *mockUserRepoForRestrictions) GetByID(_ context.Context, id int64) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	cp := *u
	return &cp, nil
}

func (m *mockUserRepoForRestrictions) GetByUsername(_ context.Context, _ string) (*model.User, error) {
	return nil, sql.ErrNoRows
}
func (m *mockUserRepoForRestrictions) GetByEmail(_ context.Context, _ string) (*model.User, error) {
	return nil, sql.ErrNoRows
}
func (m *mockUserRepoForRestrictions) GetByPasskey(_ context.Context, _ string) (*model.User, error) {
	return nil, sql.ErrNoRows
}
func (m *mockUserRepoForRestrictions) Count(_ context.Context) (int64, error) { return 0, nil }
func (m *mockUserRepoForRestrictions) Create(_ context.Context, _ *model.User) error { return nil }
func (m *mockUserRepoForRestrictions) Update(_ context.Context, u *model.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *u
	m.users[u.ID] = &cp
	return nil
}
func (m *mockUserRepoForRestrictions) IncrementStats(_ context.Context, _ int64, _, _ int64) error {
	return nil
}
func (m *mockUserRepoForRestrictions) List(_ context.Context, _ repository.ListUsersOptions) ([]model.User, int64, error) {
	return nil, 0, nil
}
func (m *mockUserRepoForRestrictions) ListStaff(_ context.Context) ([]model.User, error) {
	return nil, nil
}
func (m *mockUserRepoForRestrictions) UpdateLastAccess(_ context.Context, _ int64) error {
	return nil
}

// --- helpers ---

func setupRestrictionService() (*RestrictionService, *mockRestrictionRepo, *mockUserRepoForRestrictions) {
	restrictionRepo := newMockRestrictionRepo()
	userRepo := newMockUserRepoForRestrictions()
	bus := event.NewInMemoryBus()
	svc := NewRestrictionService(restrictionRepo, userRepo, bus)
	return svc, restrictionRepo, userRepo
}

// --- tests ---

func TestApplyRestriction_HappyPath(t *testing.T) {
	svc, _, userRepo := setupRestrictionService()
	userRepo.addUser(&model.User{ID: 1, Username: "testuser", CanDownload: true, CanUpload: true, CanChat: true})

	adminID := int64(99)
	restriction, err := svc.ApplyRestriction(context.Background(), 1, model.RestrictionTypeDownload, "bad ratio", nil, &adminID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if restriction.ID == 0 {
		t.Error("restriction ID should be assigned")
	}
	if restriction.RestrictionType != model.RestrictionTypeDownload {
		t.Errorf("expected download restriction, got %s", restriction.RestrictionType)
	}

	// Verify user flag was updated.
	user, _ := userRepo.GetByID(context.Background(), 1)
	if user.CanDownload {
		t.Error("user.CanDownload should be false after restriction")
	}
}

func TestApplyRestriction_EmptyReason(t *testing.T) {
	svc, _, userRepo := setupRestrictionService()
	userRepo.addUser(&model.User{ID: 1, Username: "testuser", CanDownload: true})

	adminID := int64(99)
	_, err := svc.ApplyRestriction(context.Background(), 1, model.RestrictionTypeDownload, "", nil, &adminID)
	if err == nil {
		t.Fatal("expected error for empty reason")
	}
}

func TestApplyRestriction_InvalidType(t *testing.T) {
	svc, _, userRepo := setupRestrictionService()
	userRepo.addUser(&model.User{ID: 1, Username: "testuser"})

	adminID := int64(99)
	_, err := svc.ApplyRestriction(context.Background(), 1, "invalid_type", "reason", nil, &adminID)
	if err == nil {
		t.Fatal("expected error for invalid restriction type")
	}
}

func TestLiftRestriction_HappyPath(t *testing.T) {
	svc, _, userRepo := setupRestrictionService()
	userRepo.addUser(&model.User{ID: 1, Username: "testuser", CanDownload: true, CanUpload: true, CanChat: true})
	userRepo.addUser(&model.User{ID: 99, Username: "admin"})

	adminID := int64(99)
	restriction, err := svc.ApplyRestriction(context.Background(), 1, model.RestrictionTypeDownload, "bad ratio", nil, &adminID)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	// Verify flag is false.
	user, _ := userRepo.GetByID(context.Background(), 1)
	if user.CanDownload {
		t.Error("should be false after apply")
	}

	// Lift it.
	err = svc.LiftRestriction(context.Background(), restriction.ID, &adminID)
	if err != nil {
		t.Fatalf("lift: %v", err)
	}

	// Verify flag is restored.
	user, _ = userRepo.GetByID(context.Background(), 1)
	if !user.CanDownload {
		t.Error("user.CanDownload should be true after lift")
	}
}

func TestLiftRestriction_RestoreFlagOnlyWhenNoOtherActive(t *testing.T) {
	svc, _, userRepo := setupRestrictionService()
	userRepo.addUser(&model.User{ID: 1, Username: "testuser", CanDownload: true, CanUpload: true, CanChat: true})
	userRepo.addUser(&model.User{ID: 99, Username: "admin"})

	adminID := int64(99)

	// Apply two download restrictions.
	r1, err := svc.ApplyRestriction(context.Background(), 1, model.RestrictionTypeDownload, "reason 1", nil, &adminID)
	if err != nil {
		t.Fatalf("apply r1: %v", err)
	}
	_, err = svc.ApplyRestriction(context.Background(), 1, model.RestrictionTypeDownload, "reason 2", nil, &adminID)
	if err != nil {
		t.Fatalf("apply r2: %v", err)
	}

	// Lift only the first one.
	err = svc.LiftRestriction(context.Background(), r1.ID, &adminID)
	if err != nil {
		t.Fatalf("lift r1: %v", err)
	}

	// Flag should still be false since r2 is still active.
	user, _ := userRepo.GetByID(context.Background(), 1)
	if user.CanDownload {
		t.Error("user.CanDownload should still be false with another active restriction")
	}
}

func TestLiftRestriction_AlreadyLifted(t *testing.T) {
	svc, _, userRepo := setupRestrictionService()
	userRepo.addUser(&model.User{ID: 1, Username: "testuser", CanDownload: true, CanUpload: true, CanChat: true})
	userRepo.addUser(&model.User{ID: 99, Username: "admin"})

	adminID := int64(99)
	r, _ := svc.ApplyRestriction(context.Background(), 1, model.RestrictionTypeDownload, "reason", nil, &adminID)
	_ = svc.LiftRestriction(context.Background(), r.ID, &adminID)

	err := svc.LiftRestriction(context.Background(), r.ID, &adminID)
	if err == nil {
		t.Fatal("expected error for already-lifted restriction")
	}
}

func TestResolveExpired(t *testing.T) {
	svc, _, userRepo := setupRestrictionService()
	userRepo.addUser(&model.User{ID: 1, Username: "testuser", CanDownload: true, CanUpload: true, CanChat: true})

	adminID := int64(99)
	past := time.Now().Add(-1 * time.Hour)
	_, err := svc.ApplyRestriction(context.Background(), 1, model.RestrictionTypeDownload, "expired", &past, &adminID)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	// Verify flag is false.
	user, _ := userRepo.GetByID(context.Background(), 1)
	if user.CanDownload {
		t.Error("should be false after apply")
	}

	// Resolve expired.
	count, err := svc.ResolveExpired(context.Background())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 resolved, got %d", count)
	}

	// Verify flag is restored.
	user, _ = userRepo.GetByID(context.Background(), 1)
	if !user.CanDownload {
		t.Error("user.CanDownload should be true after expired restriction resolved")
	}
}

func TestListByUser(t *testing.T) {
	svc, _, userRepo := setupRestrictionService()
	userRepo.addUser(&model.User{ID: 1, Username: "testuser", CanDownload: true, CanUpload: true, CanChat: true})

	adminID := int64(99)
	_, _ = svc.ApplyRestriction(context.Background(), 1, model.RestrictionTypeDownload, "reason1", nil, &adminID)
	_, _ = svc.ApplyRestriction(context.Background(), 1, model.RestrictionTypeUpload, "reason2", nil, &adminID)

	list, err := svc.ListByUser(context.Background(), 1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 restrictions, got %d", len(list))
	}
}
