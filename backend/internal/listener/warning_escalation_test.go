package listener

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// --- mock site settings repo ---

type mockSiteSettingsRepo struct {
	mu       sync.Mutex
	settings map[string]string
}

func newMockSiteSettingsRepo(settings map[string]string) *mockSiteSettingsRepo {
	return &mockSiteSettingsRepo{settings: settings}
}

func (m *mockSiteSettingsRepo) Get(_ context.Context, key string) (*model.SiteSetting, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.settings[key]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return &model.SiteSetting{Key: key, Value: v}, nil
}

func (m *mockSiteSettingsRepo) Set(_ context.Context, key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings[key] = value
	return nil
}

func (m *mockSiteSettingsRepo) GetAll(_ context.Context) ([]model.SiteSetting, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []model.SiteSetting
	for k, v := range m.settings {
		result = append(result, model.SiteSetting{Key: k, Value: v})
	}
	return result, nil
}

// --- mock warning repo ---

type mockEscalationWarningRepo struct {
	mu       sync.Mutex
	warnings []*model.Warning
	nextID   int64
}

func (m *mockEscalationWarningRepo) Create(_ context.Context, w *model.Warning) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	w.ID = m.nextID
	m.warnings = append(m.warnings, w)
	return nil
}

func (m *mockEscalationWarningRepo) GetByID(_ context.Context, id int64) (*model.Warning, error) {
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

func (m *mockEscalationWarningRepo) ListByUser(_ context.Context, _ int64, _ bool) ([]model.Warning, error) {
	return nil, nil
}

func (m *mockEscalationWarningRepo) ListAll(_ context.Context, _ repository.ListWarningsOptions) ([]model.Warning, int64, error) {
	return nil, 0, nil
}

func (m *mockEscalationWarningRepo) Update(_ context.Context, _ *model.Warning) error {
	return nil
}

func (m *mockEscalationWarningRepo) CountActiveByUser(_ context.Context, userID int64) (int, error) {
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

func (m *mockEscalationWarningRepo) CountActiveManualByUser(_ context.Context, userID int64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, w := range m.warnings {
		if w.UserID == userID && w.Status == model.WarningStatusActive && w.Type == model.WarningTypeManual {
			count++
		}
	}
	return count, nil
}

func (m *mockEscalationWarningRepo) GetActiveRatioWarning(_ context.Context, _ int64) (*model.Warning, error) {
	return nil, sql.ErrNoRows
}

func (m *mockEscalationWarningRepo) GetUsersWithLowRatio(_ context.Context, _ float64, _ int64) ([]model.User, error) {
	return nil, nil
}

func (m *mockEscalationWarningRepo) ResolveExpiredManualWarnings(_ context.Context) ([]int64, error) {
	return nil, nil
}

// --- mock restriction repo ---

type mockEscalationRestrictionRepo struct {
	mu           sync.Mutex
	restrictions []*model.Restriction
	nextID       int64
}

func (m *mockEscalationRestrictionRepo) Create(_ context.Context, r *model.Restriction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	r.ID = m.nextID
	now := time.Now()
	r.CreatedAt = now
	m.restrictions = append(m.restrictions, r)
	return nil
}

func (m *mockEscalationRestrictionRepo) GetByID(_ context.Context, id int64) (*model.Restriction, error) {
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

func (m *mockEscalationRestrictionRepo) ListByUser(_ context.Context, _ int64) ([]model.Restriction, error) {
	return nil, nil
}

func (m *mockEscalationRestrictionRepo) ListActive(_ context.Context) ([]model.Restriction, error) {
	return nil, nil
}

func (m *mockEscalationRestrictionRepo) Lift(_ context.Context, _ int64, _ *int64) error {
	return nil
}

func (m *mockEscalationRestrictionRepo) LiftExpired(_ context.Context) ([]model.Restriction, error) {
	return nil, nil
}

func (m *mockEscalationRestrictionRepo) HasActiveByType(_ context.Context, _ int64, _ string) (bool, error) {
	return false, nil
}

// --- mock user repo for escalation ---

type mockEscalationUserRepo struct {
	mu    sync.Mutex
	users map[int64]*model.User
}

func newMockEscalationUserRepo() *mockEscalationUserRepo {
	return &mockEscalationUserRepo{
		users: map[int64]*model.User{
			10: {ID: 10, Username: "testuser", Enabled: true, CanDownload: true, CanUpload: true, CanChat: true},
		},
	}
}

func (m *mockEscalationUserRepo) GetByID(_ context.Context, id int64) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	cp := *u
	return &cp, nil
}

func (m *mockEscalationUserRepo) GetByUsername(_ context.Context, _ string) (*model.User, error) {
	return nil, nil
}
func (m *mockEscalationUserRepo) GetByEmail(_ context.Context, _ string) (*model.User, error) {
	return nil, nil
}
func (m *mockEscalationUserRepo) GetByPasskey(_ context.Context, _ string) (*model.User, error) {
	return nil, nil
}
func (m *mockEscalationUserRepo) Count(_ context.Context) (int64, error)    { return 0, nil }
func (m *mockEscalationUserRepo) Create(_ context.Context, _ *model.User) error { return nil }
func (m *mockEscalationUserRepo) Update(_ context.Context, u *model.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *u
	m.users[u.ID] = &cp
	return nil
}
func (m *mockEscalationUserRepo) IncrementStats(_ context.Context, _ int64, _, _ int64) error {
	return nil
}
func (m *mockEscalationUserRepo) List(_ context.Context, _ repository.ListUsersOptions) ([]model.User, int64, error) {
	return nil, 0, nil
}
func (m *mockEscalationUserRepo) ListStaff(_ context.Context) ([]model.User, error) {
	return nil, nil
}
func (m *mockEscalationUserRepo) UpdateLastAccess(_ context.Context, _ int64) error { return nil }

// --- helper ---

func setupEscalation(settings map[string]string, warnings []*model.Warning) (
	event.Bus,
	*mockEscalationWarningRepo,
	*mockEscalationRestrictionRepo,
	*mockEscalationUserRepo,
	*mockActivityLogRepo,
) {
	bus := event.NewInMemoryBus()

	settingsRepo := newMockSiteSettingsRepo(settings)
	siteSettingsSvc := service.NewSiteSettingsService(settingsRepo, bus)

	warningRepo := &mockEscalationWarningRepo{warnings: warnings}
	restrictionRepo := &mockEscalationRestrictionRepo{}
	userRepo := newMockEscalationUserRepo()
	restrictionSvc := service.NewRestrictionService(restrictionRepo, userRepo, bus)

	activityLogRepo := &mockActivityLogRepo{}
	activityLogSvc := service.NewActivityLogService(activityLogRepo)

	RegisterWarningEscalationListener(bus, siteSettingsSvc, warningRepo, restrictionSvc, userRepo, activityLogSvc)

	return bus, warningRepo, restrictionRepo, userRepo, activityLogRepo
}

// --- tests ---

func TestWarningEscalation_DisabledByDefault(t *testing.T) {
	// Escalation disabled (default) — no action even with many warnings.
	warnings := []*model.Warning{
		{ID: 1, UserID: 10, Type: model.WarningTypeManual, Status: model.WarningStatusActive},
		{ID: 2, UserID: 10, Type: model.WarningTypeManual, Status: model.WarningStatusActive},
		{ID: 3, UserID: 10, Type: model.WarningTypeManual, Status: model.WarningStatusActive},
	}

	bus, _, restrictionRepo, userRepo, _ := setupEscalation(
		map[string]string{"warning_escalation_enabled": "false"},
		warnings,
	)

	bus.Publish(context.Background(), &event.WarningIssuedEvent{
		Base:        event.NewBase(event.WarningIssued, event.Actor{ID: 1, Username: "admin"}),
		WarningID:   3,
		UserID:      10,
		Username:    "testuser",
		WarningType: model.WarningTypeManual,
	})

	// User should remain enabled, no restrictions.
	u, _ := userRepo.GetByID(context.Background(), 10)
	if !u.Enabled {
		t.Error("expected user to remain enabled when escalation is disabled")
	}
	if len(restrictionRepo.restrictions) != 0 {
		t.Errorf("expected 0 restrictions, got %d", len(restrictionRepo.restrictions))
	}
}

func TestWarningEscalation_SkipsRatioWarnings(t *testing.T) {
	// Escalation enabled but ratio warning should not trigger it.
	warnings := []*model.Warning{
		{ID: 1, UserID: 10, Type: model.WarningTypeRatioSoft, Status: model.WarningStatusActive},
		{ID: 2, UserID: 10, Type: model.WarningTypeRatioSoft, Status: model.WarningStatusActive},
		{ID: 3, UserID: 10, Type: model.WarningTypeRatioSoft, Status: model.WarningStatusActive},
	}

	bus, _, restrictionRepo, userRepo, _ := setupEscalation(
		map[string]string{
			"warning_escalation_enabled": "true",
			"warning_count_restrict":     "2",
			"warning_count_ban":          "3",
		},
		warnings,
	)

	bus.Publish(context.Background(), &event.WarningIssuedEvent{
		Base:        event.NewBase(event.WarningIssued, event.Actor{ID: 0, Username: "System"}),
		WarningID:   3,
		UserID:      10,
		Username:    "testuser",
		WarningType: model.WarningTypeRatioSoft,
	})

	u, _ := userRepo.GetByID(context.Background(), 10)
	if !u.Enabled {
		t.Error("expected user to remain enabled for ratio warnings")
	}
	if len(restrictionRepo.restrictions) != 0 {
		t.Errorf("expected 0 restrictions, got %d", len(restrictionRepo.restrictions))
	}
}

func TestWarningEscalation_AppliesRestriction(t *testing.T) {
	// 2 active manual warnings should trigger restriction (threshold default 2).
	warnings := []*model.Warning{
		{ID: 1, UserID: 10, Type: model.WarningTypeManual, Status: model.WarningStatusActive},
		{ID: 2, UserID: 10, Type: model.WarningTypeManual, Status: model.WarningStatusActive},
	}

	bus, _, restrictionRepo, userRepo, activityLogRepo := setupEscalation(
		map[string]string{
			"warning_escalation_enabled": "true",
			"warning_count_restrict":     "2",
			"warning_count_ban":          "3",
			"warning_restrict_type":      "download",
			"warning_restrict_days":      "7",
		},
		warnings,
	)

	bus.Publish(context.Background(), &event.WarningIssuedEvent{
		Base:        event.NewBase(event.WarningIssued, event.Actor{ID: 1, Username: "admin"}),
		WarningID:   2,
		UserID:      10,
		Username:    "testuser",
		WarningType: model.WarningTypeManual,
	})

	// User should still be enabled but should have a restriction.
	u, _ := userRepo.GetByID(context.Background(), 10)
	if !u.Enabled {
		t.Error("expected user to remain enabled at restriction threshold")
	}

	restrictionRepo.mu.Lock()
	defer restrictionRepo.mu.Unlock()
	if len(restrictionRepo.restrictions) != 1 {
		t.Fatalf("expected 1 restriction, got %d", len(restrictionRepo.restrictions))
	}
	r := restrictionRepo.restrictions[0]
	if r.RestrictionType != "download" {
		t.Errorf("expected download restriction, got %s", r.RestrictionType)
	}
	if r.ExpiresAt == nil {
		t.Error("expected restriction to have an expiry")
	}

	// Activity log should have an entry.
	activityLogRepo.mu.Lock()
	defer activityLogRepo.mu.Unlock()
	found := false
	for _, log := range activityLogRepo.logs {
		if log.EventType == "warning_escalation_restrict" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected warning_escalation_restrict activity log entry")
	}
}

func TestWarningEscalation_BansUser(t *testing.T) {
	// 3 active manual warnings should trigger ban (threshold default 3).
	warnings := []*model.Warning{
		{ID: 1, UserID: 10, Type: model.WarningTypeManual, Status: model.WarningStatusActive},
		{ID: 2, UserID: 10, Type: model.WarningTypeManual, Status: model.WarningStatusActive},
		{ID: 3, UserID: 10, Type: model.WarningTypeManual, Status: model.WarningStatusActive},
	}

	bus, _, _, userRepo, activityLogRepo := setupEscalation(
		map[string]string{
			"warning_escalation_enabled": "true",
			"warning_count_restrict":     "2",
			"warning_count_ban":          "3",
		},
		warnings,
	)

	bus.Publish(context.Background(), &event.WarningIssuedEvent{
		Base:        event.NewBase(event.WarningIssued, event.Actor{ID: 1, Username: "admin"}),
		WarningID:   3,
		UserID:      10,
		Username:    "testuser",
		WarningType: model.WarningTypeManual,
	})

	// User should be disabled.
	u, _ := userRepo.GetByID(context.Background(), 10)
	if u.Enabled {
		t.Error("expected user to be disabled after reaching ban threshold")
	}

	// Activity log should have a ban entry.
	activityLogRepo.mu.Lock()
	defer activityLogRepo.mu.Unlock()
	found := false
	for _, log := range activityLogRepo.logs {
		if log.EventType == "warning_escalation_ban" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected warning_escalation_ban activity log entry")
	}
}

func TestWarningEscalation_AllRestrictionTypes(t *testing.T) {
	// restriction type "all" should apply download, upload, and chat restrictions.
	warnings := []*model.Warning{
		{ID: 1, UserID: 10, Type: model.WarningTypeManual, Status: model.WarningStatusActive},
		{ID: 2, UserID: 10, Type: model.WarningTypeManual, Status: model.WarningStatusActive},
	}

	bus, _, restrictionRepo, _, _ := setupEscalation(
		map[string]string{
			"warning_escalation_enabled": "true",
			"warning_count_restrict":     "2",
			"warning_count_ban":          "5",
			"warning_restrict_type":      "all",
			"warning_restrict_days":      "3",
		},
		warnings,
	)

	bus.Publish(context.Background(), &event.WarningIssuedEvent{
		Base:        event.NewBase(event.WarningIssued, event.Actor{ID: 1, Username: "admin"}),
		WarningID:   2,
		UserID:      10,
		Username:    "testuser",
		WarningType: model.WarningTypeManual,
	})

	restrictionRepo.mu.Lock()
	defer restrictionRepo.mu.Unlock()
	if len(restrictionRepo.restrictions) != 3 {
		t.Fatalf("expected 3 restrictions (download+upload+chat), got %d", len(restrictionRepo.restrictions))
	}

	types := make(map[string]bool)
	for _, r := range restrictionRepo.restrictions {
		types[r.RestrictionType] = true
	}
	for _, expected := range []string{"download", "upload", "chat"} {
		if !types[expected] {
			t.Errorf("expected %s restriction to be applied", expected)
		}
	}
}

func TestWarningEscalation_BelowThreshold(t *testing.T) {
	// 1 active manual warning — below both thresholds, no action.
	warnings := []*model.Warning{
		{ID: 1, UserID: 10, Type: model.WarningTypeManual, Status: model.WarningStatusActive},
	}

	bus, _, restrictionRepo, userRepo, _ := setupEscalation(
		map[string]string{
			"warning_escalation_enabled": "true",
			"warning_count_restrict":     "2",
			"warning_count_ban":          "3",
		},
		warnings,
	)

	bus.Publish(context.Background(), &event.WarningIssuedEvent{
		Base:        event.NewBase(event.WarningIssued, event.Actor{ID: 1, Username: "admin"}),
		WarningID:   1,
		UserID:      10,
		Username:    "testuser",
		WarningType: model.WarningTypeManual,
	})

	u, _ := userRepo.GetByID(context.Background(), 10)
	if !u.Enabled {
		t.Error("expected user to remain enabled below threshold")
	}
	if len(restrictionRepo.restrictions) != 0 {
		t.Errorf("expected 0 restrictions, got %d", len(restrictionRepo.restrictions))
	}
}
