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

// --- mock warning repo ---

type mockWarningRepo struct {
	mu       sync.Mutex
	warnings []*model.Warning
	nextID   int64
}

func newMockWarningRepo() *mockWarningRepo {
	return &mockWarningRepo{nextID: 1}
}

func (m *mockWarningRepo) Create(_ context.Context, w *model.Warning) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	w.ID = m.nextID
	m.nextID++
	w.CreatedAt = time.Now()
	w.UpdatedAt = time.Now()
	cp := *w
	m.warnings = append(m.warnings, &cp)
	return nil
}

func (m *mockWarningRepo) GetByID(_ context.Context, id int64) (*model.Warning, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, w := range m.warnings {
		if w.ID == id {
			cp := *w
			return &cp, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (m *mockWarningRepo) ListByUser(_ context.Context, userID int64, includeInactive bool) ([]model.Warning, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []model.Warning
	for _, w := range m.warnings {
		if w.UserID == userID {
			if !includeInactive && w.Status != model.WarningStatusActive {
				continue
			}
			result = append(result, *w)
		}
	}
	return result, nil
}

func (m *mockWarningRepo) ListAll(_ context.Context, opts repository.ListWarningsOptions) ([]model.Warning, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var filtered []model.Warning
	for _, w := range m.warnings {
		if opts.UserID != nil && w.UserID != *opts.UserID {
			continue
		}
		if opts.Status != nil && *opts.Status != "all" && w.Status != *opts.Status {
			continue
		}
		filtered = append(filtered, *w)
	}
	return filtered, int64(len(filtered)), nil
}

func (m *mockWarningRepo) Update(_ context.Context, w *model.Warning) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, existing := range m.warnings {
		if existing.ID == w.ID {
			cp := *w
			cp.UpdatedAt = time.Now()
			m.warnings[i] = &cp
			return nil
		}
	}
	return sql.ErrNoRows
}

func (m *mockWarningRepo) CountActiveByUser(_ context.Context, userID int64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, w := range m.warnings {
		if w.UserID == userID && w.Status == model.WarningStatusActive {
			count++
		}
	}
	return count, nil
}

func (m *mockWarningRepo) GetActiveRatioWarning(_ context.Context, userID int64) (*model.Warning, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, w := range m.warnings {
		if w.UserID == userID && w.Status == model.WarningStatusActive && w.Type == model.WarningTypeRatioSoft {
			cp := *w
			return &cp, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (m *mockWarningRepo) GetUsersWithLowRatio(_ context.Context, _ float64, _ int64) ([]model.User, error) {
	return nil, nil
}

// --- mock user repo (minimal) ---

type mockUserRepoForWarnings struct {
	mu    sync.Mutex
	users map[int64]*model.User
}

func newMockUserRepoForWarnings() *mockUserRepoForWarnings {
	return &mockUserRepoForWarnings{users: make(map[int64]*model.User)}
}

func (m *mockUserRepoForWarnings) addUser(u *model.User) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[u.ID] = u
}

func (m *mockUserRepoForWarnings) GetByID(_ context.Context, id int64) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	cp := *u
	return &cp, nil
}

func (m *mockUserRepoForWarnings) GetByUsername(_ context.Context, _ string) (*model.User, error) {
	return nil, sql.ErrNoRows
}
func (m *mockUserRepoForWarnings) GetByEmail(_ context.Context, _ string) (*model.User, error) {
	return nil, sql.ErrNoRows
}
func (m *mockUserRepoForWarnings) GetByPasskey(_ context.Context, _ string) (*model.User, error) {
	return nil, sql.ErrNoRows
}
func (m *mockUserRepoForWarnings) Count(_ context.Context) (int64, error) { return 0, nil }
func (m *mockUserRepoForWarnings) Create(_ context.Context, _ *model.User) error { return nil }
func (m *mockUserRepoForWarnings) Update(_ context.Context, u *model.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[u.ID] = u
	return nil
}
func (m *mockUserRepoForWarnings) IncrementStats(_ context.Context, _ int64, _, _ int64) error {
	return nil
}
func (m *mockUserRepoForWarnings) List(_ context.Context, _ repository.ListUsersOptions) ([]model.User, int64, error) {
	return nil, 0, nil
}
func (m *mockUserRepoForWarnings) ListStaff(_ context.Context) ([]model.User, error) {
	return nil, nil
}
func (m *mockUserRepoForWarnings) UpdateLastAccess(_ context.Context, _ int64) error { return nil }

// --- mock message repo (minimal) ---

type mockMessageRepoForWarnings struct {
	mu       sync.Mutex
	messages []*model.Message
	nextID   int64
}

func newMockMessageRepoForWarnings() *mockMessageRepoForWarnings {
	return &mockMessageRepoForWarnings{nextID: 1}
}

func (m *mockMessageRepoForWarnings) Create(_ context.Context, msg *model.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	msg.ID = m.nextID
	m.nextID++
	msg.CreatedAt = time.Now()
	m.messages = append(m.messages, msg)
	return nil
}

func (m *mockMessageRepoForWarnings) GetByID(_ context.Context, _ int64) (*model.Message, error) {
	return nil, sql.ErrNoRows
}
func (m *mockMessageRepoForWarnings) ListInbox(_ context.Context, _ int64, _, _ int) ([]model.Message, int64, error) {
	return nil, 0, nil
}
func (m *mockMessageRepoForWarnings) ListOutbox(_ context.Context, _ int64, _, _ int) ([]model.Message, int64, error) {
	return nil, 0, nil
}
func (m *mockMessageRepoForWarnings) MarkAsRead(_ context.Context, _, _ int64) error { return nil }
func (m *mockMessageRepoForWarnings) DeleteForUser(_ context.Context, _, _ int64) error {
	return nil
}
func (m *mockMessageRepoForWarnings) CountUnread(_ context.Context, _ int64) (int, error) {
	return 0, nil
}

// --- tests ---

func TestIssueManualWarning(t *testing.T) {
	warnRepo := newMockWarningRepo()
	userRepo := newMockUserRepoForWarnings()
	msgRepo := newMockMessageRepoForWarnings()
	bus := event.NewInMemoryBus()

	admin := &model.User{ID: 1, Username: "admin", Enabled: true}
	target := &model.User{ID: 2, Username: "baduser", Enabled: true}
	userRepo.addUser(admin)
	userRepo.addUser(target)

	svc := NewWarningService(warnRepo, userRepo, msgRepo, bus)

	// Issue a manual warning
	w, err := svc.IssueManualWarning(context.Background(), 2, "bad behavior", nil, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.ID == 0 {
		t.Fatal("expected warning ID to be set")
	}
	if w.Type != model.WarningTypeManual {
		t.Errorf("expected type %q, got %q", model.WarningTypeManual, w.Type)
	}
	if w.Status != model.WarningStatusActive {
		t.Errorf("expected status %q, got %q", model.WarningStatusActive, w.Status)
	}

	// User should be warned
	u, _ := userRepo.GetByID(context.Background(), 2)
	if !u.Warned {
		t.Error("expected user to be warned")
	}

	// PM should have been sent
	if len(msgRepo.messages) != 1 {
		t.Fatalf("expected 1 PM, got %d", len(msgRepo.messages))
	}
	if msgRepo.messages[0].ReceiverID != 2 {
		t.Errorf("expected PM to user 2, got %d", msgRepo.messages[0].ReceiverID)
	}
}

func TestIssueManualWarning_EmptyReason(t *testing.T) {
	warnRepo := newMockWarningRepo()
	userRepo := newMockUserRepoForWarnings()
	msgRepo := newMockMessageRepoForWarnings()
	bus := event.NewInMemoryBus()

	svc := NewWarningService(warnRepo, userRepo, msgRepo, bus)

	_, err := svc.IssueManualWarning(context.Background(), 1, "", nil, 2)
	if err == nil {
		t.Fatal("expected error for empty reason")
	}
}

func TestLiftWarning(t *testing.T) {
	warnRepo := newMockWarningRepo()
	userRepo := newMockUserRepoForWarnings()
	msgRepo := newMockMessageRepoForWarnings()
	bus := event.NewInMemoryBus()

	admin := &model.User{ID: 1, Username: "admin", Enabled: true}
	target := &model.User{ID: 2, Username: "user", Enabled: true, Warned: true}
	userRepo.addUser(admin)
	userRepo.addUser(target)

	svc := NewWarningService(warnRepo, userRepo, msgRepo, bus)

	// Create a warning
	w, err := svc.IssueManualWarning(context.Background(), 2, "test reason", nil, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Lift it
	err = svc.LiftWarning(context.Background(), w.ID, 1, "fixed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// User should no longer be warned
	u, _ := userRepo.GetByID(context.Background(), 2)
	if u.Warned {
		t.Error("expected user warned flag to be cleared")
	}

	// Warning should be lifted
	lifted, _ := warnRepo.GetByID(context.Background(), w.ID)
	if lifted.Status != model.WarningStatusLifted {
		t.Errorf("expected status %q, got %q", model.WarningStatusLifted, lifted.Status)
	}
}

func TestLiftWarning_NotActive(t *testing.T) {
	warnRepo := newMockWarningRepo()
	userRepo := newMockUserRepoForWarnings()
	msgRepo := newMockMessageRepoForWarnings()
	bus := event.NewInMemoryBus()

	admin := &model.User{ID: 1, Username: "admin", Enabled: true}
	target := &model.User{ID: 2, Username: "user", Enabled: true}
	userRepo.addUser(admin)
	userRepo.addUser(target)

	svc := NewWarningService(warnRepo, userRepo, msgRepo, bus)

	w, _ := svc.IssueManualWarning(context.Background(), 2, "test", nil, 1)
	_ = svc.LiftWarning(context.Background(), w.ID, 1, "fixed")

	// Try to lift again
	err := svc.LiftWarning(context.Background(), w.ID, 1, "again")
	if err == nil {
		t.Fatal("expected error lifting non-active warning")
	}
}

func TestIssueRatioWarning(t *testing.T) {
	warnRepo := newMockWarningRepo()
	userRepo := newMockUserRepoForWarnings()
	msgRepo := newMockMessageRepoForWarnings()
	bus := event.NewInMemoryBus()

	user := &model.User{ID: 1, Username: "lowratio", Enabled: true}
	userRepo.addUser(user)

	svc := NewWarningService(warnRepo, userRepo, msgRepo, bus)

	w, err := svc.IssueRatioWarning(context.Background(), 1, "Your ratio is too low")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w.Type != model.WarningTypeRatioSoft {
		t.Errorf("expected type %q, got %q", model.WarningTypeRatioSoft, w.Type)
	}

	// User should be warned
	u, _ := userRepo.GetByID(context.Background(), 1)
	if !u.Warned {
		t.Error("expected user to be warned")
	}
}

func TestEscalateRatioWarning(t *testing.T) {
	warnRepo := newMockWarningRepo()
	userRepo := newMockUserRepoForWarnings()
	msgRepo := newMockMessageRepoForWarnings()
	bus := event.NewInMemoryBus()

	user := &model.User{ID: 1, Username: "lowratio", Enabled: true}
	userRepo.addUser(user)

	svc := NewWarningService(warnRepo, userRepo, msgRepo, bus)

	w, _ := svc.IssueRatioWarning(context.Background(), 1, "low ratio")

	err := svc.EscalateRatioWarning(context.Background(), w.ID, "Account disabled")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// User should be disabled
	u, _ := userRepo.GetByID(context.Background(), 1)
	if u.Enabled {
		t.Error("expected user to be disabled after escalation")
	}

	// Original warning should be escalated
	escalated, _ := warnRepo.GetByID(context.Background(), w.ID)
	if escalated.Status != model.WarningStatusEscalated {
		t.Errorf("expected status %q, got %q", model.WarningStatusEscalated, escalated.Status)
	}
}

func TestResolveWarning(t *testing.T) {
	warnRepo := newMockWarningRepo()
	userRepo := newMockUserRepoForWarnings()
	msgRepo := newMockMessageRepoForWarnings()
	bus := event.NewInMemoryBus()

	user := &model.User{ID: 1, Username: "user", Enabled: true}
	userRepo.addUser(user)

	svc := NewWarningService(warnRepo, userRepo, msgRepo, bus)

	w, _ := svc.IssueRatioWarning(context.Background(), 1, "low ratio")

	err := svc.ResolveWarning(context.Background(), w.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Warning should be resolved
	resolved, _ := warnRepo.GetByID(context.Background(), w.ID)
	if resolved.Status != model.WarningStatusResolved {
		t.Errorf("expected status %q, got %q", model.WarningStatusResolved, resolved.Status)
	}

	// User should no longer be warned
	u, _ := userRepo.GetByID(context.Background(), 1)
	if u.Warned {
		t.Error("expected user warned flag to be cleared")
	}
}

func TestReplaceTemplateVars(t *testing.T) {
	msg := "Hello {{username}}, your ratio is {{ratio}}."
	result := ReplaceTemplateVars(msg, map[string]string{
		"username": "testuser",
		"ratio":    "0.150",
	})
	expected := "Hello testuser, your ratio is 0.150."
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestGetActiveRatioWarning_None(t *testing.T) {
	warnRepo := newMockWarningRepo()
	userRepo := newMockUserRepoForWarnings()
	msgRepo := newMockMessageRepoForWarnings()
	bus := event.NewInMemoryBus()

	svc := NewWarningService(warnRepo, userRepo, msgRepo, bus)

	w, err := svc.GetActiveRatioWarning(context.Background(), 999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w != nil {
		t.Error("expected nil warning for user with no ratio warnings")
	}
}

func TestListWarnings(t *testing.T) {
	warnRepo := newMockWarningRepo()
	userRepo := newMockUserRepoForWarnings()
	msgRepo := newMockMessageRepoForWarnings()
	bus := event.NewInMemoryBus()

	user := &model.User{ID: 1, Username: "user", Enabled: true}
	admin := &model.User{ID: 2, Username: "admin", Enabled: true}
	userRepo.addUser(user)
	userRepo.addUser(admin)

	svc := NewWarningService(warnRepo, userRepo, msgRepo, bus)

	_, _ = svc.IssueManualWarning(context.Background(), 1, "reason1", nil, 2)
	_, _ = svc.IssueManualWarning(context.Background(), 1, "reason2", nil, 2)

	// List all
	warnings, total, err := svc.ListWarnings(context.Background(), repository.ListWarningsOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Errorf("expected 2 warnings, got %d", total)
	}
	if len(warnings) != 2 {
		t.Errorf("expected 2 warnings, got %d", len(warnings))
	}

	// List by user
	uid := int64(1)
	userWarnings, userTotal, err := svc.ListWarnings(context.Background(), repository.ListWarningsOptions{UserID: &uid})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if userTotal != 2 {
		t.Errorf("expected 2 warnings for user 1, got %d", userTotal)
	}
	if len(userWarnings) != 2 {
		t.Errorf("expected 2 warnings for user 1, got %d", len(userWarnings))
	}
}
