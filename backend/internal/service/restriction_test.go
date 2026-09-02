package service

import (
	"context"
	"database/sql"
	"fmt"
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

func (m *mockRestrictionRepo) LiftActiveBySource(_ context.Context, userID int64, restrictionType, source string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	var n int
	for _, r := range m.restrictions {
		if r.UserID == userID && r.RestrictionType == restrictionType && r.Source == source && r.LiftedAt == nil {
			r.LiftedAt = &now
			n++
		}
	}
	return n, nil
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
func (m *mockUserRepoForRestrictions) GetByUsernames(_ context.Context, _ []string) ([]model.User, error) {
	return nil, nil
}
func (m *mockUserRepoForRestrictions) GetByEmail(_ context.Context, _ string) (*model.User, error) {
	return nil, sql.ErrNoRows
}
func (m *mockUserRepoForRestrictions) GetByPasskey(_ context.Context, _ string) (*model.User, error) {
	return nil, sql.ErrNoRows
}
func (m *mockUserRepoForRestrictions) Count(_ context.Context) (int64, error)        { return 0, nil }
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
func (m *mockUserRepoForRestrictions) SetPrivilegeFlag(_ context.Context, userID int64, restrictionType string, value bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[userID]
	if !ok {
		return sql.ErrNoRows
	}
	switch restrictionType {
	case model.RestrictionTypeDownload:
		u.CanDownload = value
	case model.RestrictionTypeUpload:
		u.CanUpload = value
	case model.RestrictionTypeChat:
		u.CanChat = value
	case model.RestrictionTypeInvite:
		u.CanInvite = value
	case model.RestrictionTypeFeed:
		u.CanFeed = value
	case model.RestrictionTypeForum:
		u.CanForum = value
	default:
		// Mirror privilegeFlagColumn's production behavior for an unmapped
		// type exactly (tasks/lessons.md #5): silently no-op here is how a
		// missing "forum" case went untested until this fake was corrected.
		return fmt.Errorf("unknown privilege flag: %s", restrictionType)
	}
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
	restriction, err := svc.ApplyRestriction(context.Background(), 1, model.RestrictionTypeDownload, "bad ratio", model.RestrictionSourceManual, nil, &adminID)
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

func TestApplyRestriction_Invite(t *testing.T) {
	svc, _, userRepo := setupRestrictionService()
	userRepo.addUser(&model.User{ID: 1, Username: "testuser", CanInvite: true})
	userRepo.addUser(&model.User{ID: 99, Username: "admin"})

	adminID := int64(99)
	restriction, err := svc.ApplyRestriction(context.Background(), 1, model.RestrictionTypeInvite, "invite abuse", model.RestrictionSourceManual, nil, &adminID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	user, _ := userRepo.GetByID(context.Background(), 1)
	if user.CanInvite {
		t.Error("user.CanInvite should be false after restriction")
	}

	if err := svc.LiftRestriction(context.Background(), restriction.ID, &adminID); err != nil {
		t.Fatalf("lift: %v", err)
	}
	user, _ = userRepo.GetByID(context.Background(), 1)
	if !user.CanInvite {
		t.Error("user.CanInvite should be true after lift")
	}
}

// TestApplyRestriction_Forum guards the gap docs/OPEN_QUESTIONS.md's HnR plan
// flagged: RestrictionTypeForum was accepted as a valid type (and already
// wired into the staff restriction handler) but privilegeFlagColumn had no
// case for it, so SetPrivilegeFlag — and therefore ApplyRestriction and
// LiftRestriction — errored the moment anything actually tried to restrict
// forum access. The test fake used to silently no-op instead of erroring the
// way production did, which is exactly how this went untested.
func TestApplyRestriction_Forum(t *testing.T) {
	svc, _, userRepo := setupRestrictionService()
	userRepo.addUser(&model.User{ID: 1, Username: "testuser", CanForum: true})
	userRepo.addUser(&model.User{ID: 99, Username: "admin"})

	adminID := int64(99)
	restriction, err := svc.ApplyRestriction(context.Background(), 1, model.RestrictionTypeForum, "spam", model.RestrictionSourceManual, nil, &adminID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	user, _ := userRepo.GetByID(context.Background(), 1)
	if user.CanForum {
		t.Error("user.CanForum should be false after restriction")
	}

	if err := svc.LiftRestriction(context.Background(), restriction.ID, &adminID); err != nil {
		t.Fatalf("lift: %v", err)
	}
	user, _ = userRepo.GetByID(context.Background(), 1)
	if !user.CanForum {
		t.Error("user.CanForum should be true after lift")
	}
}

func TestSyncUserFlag_HealsDriftedFlag(t *testing.T) {
	svc, _, userRepo := setupRestrictionService()
	// Flag reads suspended but there is no active restriction row.
	userRepo.addUser(&model.User{ID: 1, Username: "testuser", CanInvite: false})

	if err := svc.SyncUserFlag(context.Background(), 1, model.RestrictionTypeInvite); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	user, _ := userRepo.GetByID(context.Background(), 1)
	if !user.CanInvite {
		t.Error("user.CanInvite should be restored when no active restriction exists")
	}
}

func TestSyncUserFlag_KeepsFlagWhileRestrictionActive(t *testing.T) {
	svc, _, userRepo := setupRestrictionService()
	userRepo.addUser(&model.User{ID: 1, Username: "testuser", CanInvite: true})

	adminID := int64(99)
	if _, err := svc.ApplyRestriction(context.Background(), 1, model.RestrictionTypeInvite, "abuse", model.RestrictionSourceManual, nil, &adminID); err != nil {
		t.Fatalf("apply: %v", err)
	}

	if err := svc.SyncUserFlag(context.Background(), 1, model.RestrictionTypeInvite); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	user, _ := userRepo.GetByID(context.Background(), 1)
	if user.CanInvite {
		t.Error("user.CanInvite must stay false while a restriction is active")
	}
}

func TestApplyRestriction_EmptyReason(t *testing.T) {
	svc, _, userRepo := setupRestrictionService()
	userRepo.addUser(&model.User{ID: 1, Username: "testuser", CanDownload: true})

	adminID := int64(99)
	_, err := svc.ApplyRestriction(context.Background(), 1, model.RestrictionTypeDownload, "", model.RestrictionSourceManual, nil, &adminID)
	if err == nil {
		t.Fatal("expected error for empty reason")
	}
}

func TestApplyRestriction_InvalidType(t *testing.T) {
	svc, _, userRepo := setupRestrictionService()
	userRepo.addUser(&model.User{ID: 1, Username: "testuser"})

	adminID := int64(99)
	_, err := svc.ApplyRestriction(context.Background(), 1, "invalid_type", "reason", model.RestrictionSourceManual, nil, &adminID)
	if err == nil {
		t.Fatal("expected error for invalid restriction type")
	}
}

func TestLiftRestriction_HappyPath(t *testing.T) {
	svc, _, userRepo := setupRestrictionService()
	userRepo.addUser(&model.User{ID: 1, Username: "testuser", CanDownload: true, CanUpload: true, CanChat: true})
	userRepo.addUser(&model.User{ID: 99, Username: "admin"})

	adminID := int64(99)
	restriction, err := svc.ApplyRestriction(context.Background(), 1, model.RestrictionTypeDownload, "bad ratio", model.RestrictionSourceManual, nil, &adminID)
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
	r1, err := svc.ApplyRestriction(context.Background(), 1, model.RestrictionTypeDownload, "reason 1", model.RestrictionSourceManual, nil, &adminID)
	if err != nil {
		t.Fatalf("apply r1: %v", err)
	}
	_, err = svc.ApplyRestriction(context.Background(), 1, model.RestrictionTypeDownload, "reason 2", model.RestrictionSourceManual, nil, &adminID)
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
	r, _ := svc.ApplyRestriction(context.Background(), 1, model.RestrictionTypeDownload, "reason", model.RestrictionSourceManual, nil, &adminID)
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
	_, err := svc.ApplyRestriction(context.Background(), 1, model.RestrictionTypeDownload, "expired", model.RestrictionSourceManual, &past, &adminID)
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
	_, _ = svc.ApplyRestriction(context.Background(), 1, model.RestrictionTypeDownload, "reason1", model.RestrictionSourceManual, nil, &adminID)
	_, _ = svc.ApplyRestriction(context.Background(), 1, model.RestrictionTypeUpload, "reason2", model.RestrictionSourceManual, nil, &adminID)

	list, err := svc.ListByUser(context.Background(), 1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 restrictions, got %d", len(list))
	}
}

// TestLiftActiveBySource_OnlyLiftsOwnSource is the exact scenario the
// user_restrictions.source migration exists for: a manually-issued
// restriction and an automated (HnR-sourced) one, both of the same type on
// the same user. Lifting the automated one must not touch the manual one,
// and the privilege flag — which reflects "no active restriction from
// anywhere" — must stay false because the manual restriction is still open.
func TestLiftActiveBySource_OnlyLiftsOwnSource(t *testing.T) {
	svc, _, userRepo := setupRestrictionService()
	userRepo.addUser(&model.User{ID: 1, Username: "testuser", CanDownload: true, CanUpload: true, CanChat: true})
	userRepo.addUser(&model.User{ID: 99, Username: "admin"})

	adminID := int64(99)
	manual, err := svc.ApplyRestriction(context.Background(), 1, model.RestrictionTypeDownload, "abuse", model.RestrictionSourceManual, nil, &adminID)
	if err != nil {
		t.Fatalf("apply manual: %v", err)
	}
	_, err = svc.ApplyRestriction(context.Background(), 1, model.RestrictionTypeDownload, "hit and run", model.RestrictionSourceHnR, nil, nil)
	if err != nil {
		t.Fatalf("apply hnr: %v", err)
	}

	n, err := svc.LiftActiveBySource(context.Background(), 1, model.RestrictionTypeDownload, model.RestrictionSourceHnR, nil)
	if err != nil {
		t.Fatalf("lift by source: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 restriction lifted, got %d", n)
	}

	// The manual restriction must still be active.
	manualNow, err := svc.restrictions.GetByID(context.Background(), manual.ID)
	if err != nil {
		t.Fatalf("get manual: %v", err)
	}
	if manualNow.LiftedAt != nil {
		t.Error("manual restriction should not have been lifted by an hnr-source lift")
	}

	// The flag must stay false — another source's restriction is still open.
	user, _ := userRepo.GetByID(context.Background(), 1)
	if user.CanDownload {
		t.Error("user.CanDownload should still be false while the manual restriction is active")
	}
}

// TestLiftActiveBySource_NothingToLiftSucceeds is the idempotency contract:
// attempting to lift a source that has nothing active for this user/type must
// succeed with zero lifted, not return an error. HnR's compliance/clear path
// relies on this — a repeated lift attempt (already lifted, or never applied)
// is a safe no-op because the desired end state already holds.
func TestLiftActiveBySource_NothingToLiftSucceeds(t *testing.T) {
	svc, _, userRepo := setupRestrictionService()
	userRepo.addUser(&model.User{ID: 1, Username: "testuser", CanDownload: true, CanUpload: true, CanChat: true})

	n, err := svc.LiftActiveBySource(context.Background(), 1, model.RestrictionTypeDownload, model.RestrictionSourceHnR, nil)
	if err != nil {
		t.Fatalf("expected no error lifting a source with nothing active, got: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 lifted, got %d", n)
	}
}

// TestLiftActiveBySource_RestoresFlagWhenLastOne mirrors
// TestLiftRestriction_RestoreFlagOnlyWhenNoOtherActive for the source-scoped
// lift: once the only active restriction of a type is lifted, the flag comes
// back regardless of which lift method removed it.
func TestLiftActiveBySource_RestoresFlagWhenLastOne(t *testing.T) {
	svc, _, userRepo := setupRestrictionService()
	userRepo.addUser(&model.User{ID: 1, Username: "testuser", CanDownload: true, CanUpload: true, CanChat: true})

	_, err := svc.ApplyRestriction(context.Background(), 1, model.RestrictionTypeDownload, "hit and run", model.RestrictionSourceHnR, nil, nil)
	if err != nil {
		t.Fatalf("apply hnr: %v", err)
	}

	n, err := svc.LiftActiveBySource(context.Background(), 1, model.RestrictionTypeDownload, model.RestrictionSourceHnR, nil)
	if err != nil {
		t.Fatalf("lift by source: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 restriction lifted, got %d", n)
	}

	user, _ := userRepo.GetByID(context.Background(), 1)
	if !user.CanDownload {
		t.Error("user.CanDownload should be true once the only active restriction is lifted")
	}
}
