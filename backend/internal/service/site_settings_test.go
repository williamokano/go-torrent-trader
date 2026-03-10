package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// --- mock site settings repo ---

type mockSiteSettingsRepo struct {
	mu       sync.Mutex
	settings map[string]*model.SiteSetting
}

func newMockSiteSettingsRepo() *mockSiteSettingsRepo {
	return &mockSiteSettingsRepo{
		settings: make(map[string]*model.SiteSetting),
	}
}

func (m *mockSiteSettingsRepo) Get(_ context.Context, key string) (*model.SiteSetting, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.settings[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return s, nil
}

func (m *mockSiteSettingsRepo) Set(_ context.Context, key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings[key] = &model.SiteSetting{Key: key, Value: value}
	return nil
}

func (m *mockSiteSettingsRepo) GetAll(_ context.Context) ([]model.SiteSetting, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []model.SiteSetting
	for _, s := range m.settings {
		result = append(result, *s)
	}
	return result, nil
}

// --- tests ---

func TestSiteSettingsService_GetRegistrationMode_Default(t *testing.T) {
	repo := newMockSiteSettingsRepo()
	bus := event.NewInMemoryBus()
	svc := NewSiteSettingsService(repo, bus)

	mode := svc.GetRegistrationMode(context.Background())
	if mode != RegistrationModeInviteOnly {
		t.Errorf("expected %q, got %q", RegistrationModeInviteOnly, mode)
	}
}

func TestSiteSettingsService_GetRegistrationMode_Open(t *testing.T) {
	repo := newMockSiteSettingsRepo()
	bus := event.NewInMemoryBus()
	svc := NewSiteSettingsService(repo, bus)

	_ = repo.Set(context.Background(), SettingRegistrationMode, RegistrationModeOpen)

	mode := svc.GetRegistrationMode(context.Background())
	if mode != RegistrationModeOpen {
		t.Errorf("expected %q, got %q", RegistrationModeOpen, mode)
	}
}

func TestSiteSettingsService_Set_Valid(t *testing.T) {
	repo := newMockSiteSettingsRepo()
	bus := event.NewInMemoryBus()
	svc := NewSiteSettingsService(repo, bus)

	actor := event.Actor{ID: 1, Username: "admin"}
	err := svc.Set(context.Background(), SettingRegistrationMode, RegistrationModeOpen, actor)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mode := svc.GetRegistrationMode(context.Background())
	if mode != RegistrationModeOpen {
		t.Errorf("expected %q, got %q", RegistrationModeOpen, mode)
	}
}

func TestSiteSettingsService_Set_InvalidValue(t *testing.T) {
	repo := newMockSiteSettingsRepo()
	bus := event.NewInMemoryBus()
	svc := NewSiteSettingsService(repo, bus)

	actor := event.Actor{ID: 1, Username: "admin"}
	err := svc.Set(context.Background(), SettingRegistrationMode, "bogus", actor)
	if err == nil {
		t.Error("expected error for invalid registration mode")
	}
}

func TestSiteSettingsService_Set_PublishesEvent(t *testing.T) {
	repo := newMockSiteSettingsRepo()
	bus := event.NewInMemoryBus()
	svc := NewSiteSettingsService(repo, bus)

	// Seed initial value
	_ = repo.Set(context.Background(), SettingRegistrationMode, RegistrationModeInviteOnly)

	var published bool
	bus.Subscribe(event.RegistrationModeChanged, func(_ context.Context, evt event.Event) error {
		e := evt.(*event.RegistrationModeChangedEvent)
		if e.OldMode != RegistrationModeInviteOnly || e.NewMode != RegistrationModeOpen {
			t.Errorf("unexpected event values: old=%s new=%s", e.OldMode, e.NewMode)
		}
		published = true
		return nil
	})

	actor := event.Actor{ID: 1, Username: "admin"}
	_ = svc.Set(context.Background(), SettingRegistrationMode, RegistrationModeOpen, actor)

	if !published {
		t.Error("expected RegistrationModeChanged event to be published")
	}
}

func TestSiteSettingsService_GetAll(t *testing.T) {
	repo := newMockSiteSettingsRepo()
	bus := event.NewInMemoryBus()
	svc := NewSiteSettingsService(repo, bus)

	_ = repo.Set(context.Background(), SettingRegistrationMode, RegistrationModeOpen)

	settings, err := svc.GetAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(settings) != 1 {
		t.Errorf("expected 1 setting, got %d", len(settings))
	}
}

func TestSiteSettingsService_GetInt(t *testing.T) {
	repo := newMockSiteSettingsRepo()
	bus := event.NewInMemoryBus()
	svc := NewSiteSettingsService(repo, bus)
	ctx := context.Background()

	t.Run("returns fallback when key missing", func(t *testing.T) {
		got := svc.GetInt(ctx, "nonexistent", 42)
		if got != 42 {
			t.Errorf("expected 42, got %d", got)
		}
	})

	t.Run("returns parsed int", func(t *testing.T) {
		_ = repo.Set(ctx, "chat_rate_limit_window", "15")
		got := svc.GetInt(ctx, "chat_rate_limit_window", 10)
		if got != 15 {
			t.Errorf("expected 15, got %d", got)
		}
	})

	t.Run("returns fallback for non-integer value", func(t *testing.T) {
		_ = repo.Set(ctx, "bad_int", "notanumber")
		got := svc.GetInt(ctx, "bad_int", 99)
		if got != 99 {
			t.Errorf("expected 99, got %d", got)
		}
	})
}
