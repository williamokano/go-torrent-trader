package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// --- mock site settings repo ---

type mockSiteSettingsRepo struct {
	mu       sync.Mutex
	settings map[string]*model.SiteSetting
	// gets counts reads, so a cache test can prove the second call never
	// reached storage rather than merely returning the right value.
	gets int
	// getErr stands in for a storage failure, which must be told apart from
	// "no such row".
	getErr error
}

func newMockSiteSettingsRepo() *mockSiteSettingsRepo {
	return &mockSiteSettingsRepo{
		settings: make(map[string]*model.SiteSetting),
	}
}

func (m *mockSiteSettingsRepo) Get(_ context.Context, key string) (*model.SiteSetting, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gets++
	if m.getErr != nil {
		return nil, m.getErr
	}
	s, ok := m.settings[key]
	if !ok {
		// Wrapped exactly the way postgres.SiteSettingsRepo wraps it: a bare
		// errors.New here would be indistinguishable from a connection failure
		// and would hide the branch that tells them apart.
		return nil, fmt.Errorf("get site setting %q: %w", key, sql.ErrNoRows)
	}
	return s, nil
}

func (m *mockSiteSettingsRepo) getCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.gets
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

func TestSiteSettingsService_GetBool(t *testing.T) {
	repo := newMockSiteSettingsRepo()
	bus := event.NewInMemoryBus()
	svc := NewSiteSettingsService(repo, bus)
	ctx := context.Background()

	t.Run("returns fallback when key missing", func(t *testing.T) {
		got := svc.GetBool(ctx, "nonexistent", true)
		if !got {
			t.Error("expected true (fallback)")
		}
	})

	t.Run("returns true for true", func(t *testing.T) {
		_ = repo.Set(ctx, "test_bool", "true")
		got := svc.GetBool(ctx, "test_bool", false)
		if !got {
			t.Error("expected true")
		}
	})

	t.Run("returns true for 1", func(t *testing.T) {
		_ = repo.Set(ctx, "test_bool_one", "1")
		got := svc.GetBool(ctx, "test_bool_one", false)
		if !got {
			t.Error("expected true for '1'")
		}
	})

	t.Run("returns false for false", func(t *testing.T) {
		_ = repo.Set(ctx, "test_bool_false", "false")
		got := svc.GetBool(ctx, "test_bool_false", true)
		if got {
			t.Error("expected false")
		}
	})

	t.Run("returns fallback for unrecognized value", func(t *testing.T) {
		_ = repo.Set(ctx, "test_bool_bad", "maybe")
		got := svc.GetBool(ctx, "test_bool_bad", true)
		if !got {
			t.Error("expected fallback true for unrecognized value")
		}
	})
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

// --- system chat name: the one cached setting ---

func TestSystemChatName_DefaultsWhenUnset(t *testing.T) {
	svc := NewSiteSettingsService(newMockSiteSettingsRepo(), event.NewInMemoryBus())

	if got := svc.SystemChatName(context.Background()); got != model.SystemChatUsername {
		t.Fatalf("SystemChatName = %q, want %q", got, model.SystemChatUsername)
	}
}

func TestSystemChatName_ReadsTheSetting(t *testing.T) {
	repo := newMockSiteSettingsRepo()
	svc := NewSiteSettingsService(repo, event.NewInMemoryBus())
	ctx := context.Background()

	_ = repo.Set(ctx, SettingChatSystemDisplayName, "Tracker Bot")

	if got := svc.SystemChatName(ctx); got != "Tracker Bot" {
		t.Fatalf("SystemChatName = %q, want %q", got, "Tracker Bot")
	}
}

// The shoutbox reads this on every backfill and every announcement, so the
// point of the cache is that repeated reads do not each become a query.
func TestSystemChatName_SecondReadIsCached(t *testing.T) {
	repo := newMockSiteSettingsRepo()
	svc := NewSiteSettingsService(repo, event.NewInMemoryBus())
	ctx := context.Background()
	_ = repo.Set(ctx, SettingChatSystemDisplayName, "Tracker Bot")

	for range 5 {
		if got := svc.SystemChatName(ctx); got != "Tracker Bot" {
			t.Fatalf("SystemChatName = %q, want %q", got, "Tracker Bot")
		}
	}
	if got := repo.getCount(); got != 1 {
		t.Fatalf("hit storage %d times, want 1 — the value is not being cached", got)
	}
}

// A missing row is a stable answer and worth caching; if it were treated like a
// failure the fallback path would query on every single message.
func TestSystemChatName_MissingRowIsCached(t *testing.T) {
	repo := newMockSiteSettingsRepo()
	svc := NewSiteSettingsService(repo, event.NewInMemoryBus())

	for range 3 {
		if got := svc.SystemChatName(context.Background()); got != model.SystemChatUsername {
			t.Fatalf("SystemChatName = %q, want the default", got)
		}
	}
	if got := repo.getCount(); got != 1 {
		t.Fatalf("hit storage %d times, want 1", got)
	}
}

// A connection blip is not an answer. Caching the fallback would pin the wrong
// name for the whole TTL, long after the database came back.
func TestSystemChatName_StorageFailureIsNotCached(t *testing.T) {
	repo := newMockSiteSettingsRepo()
	repo.getErr = errors.New("connection refused")
	svc := NewSiteSettingsService(repo, event.NewInMemoryBus())
	ctx := context.Background()

	if got := svc.SystemChatName(ctx); got != model.SystemChatUsername {
		t.Fatalf("SystemChatName = %q, want the default while storage is down", got)
	}

	repo.mu.Lock()
	repo.getErr = nil
	repo.settings[SettingChatSystemDisplayName] = &model.SiteSetting{
		Key:   SettingChatSystemDisplayName,
		Value: "Tracker Bot",
	}
	repo.mu.Unlock()

	if got := svc.SystemChatName(ctx); got != "Tracker Bot" {
		t.Fatalf("SystemChatName = %q, want the real value once storage recovers", got)
	}
}

// An operator who renames the label expects the next announcement to use it,
// not to wait out the TTL.
func TestSystemChatName_SetEvictsImmediately(t *testing.T) {
	repo := newMockSiteSettingsRepo()
	svc := NewSiteSettingsService(repo, event.NewInMemoryBus())
	ctx := context.Background()

	_ = repo.Set(ctx, SettingChatSystemDisplayName, "Tracker Bot")
	if got := svc.SystemChatName(ctx); got != "Tracker Bot" {
		t.Fatalf("SystemChatName = %q, want %q", got, "Tracker Bot")
	}

	if err := svc.Set(ctx, SettingChatSystemDisplayName, "Announcer", event.Actor{ID: 1}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := svc.SystemChatName(ctx); got != "Announcer" {
		t.Fatalf("SystemChatName = %q, want %q right after Set", got, "Announcer")
	}
}

// The safety net for a replica that did not serve the write and so never
// evicted anything.
func TestSystemChatName_ExpiresAfterTTL(t *testing.T) {
	repo := newMockSiteSettingsRepo()
	svc := NewSiteSettingsService(repo, event.NewInMemoryBus())
	ctx := context.Background()

	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	_ = repo.Set(ctx, SettingChatSystemDisplayName, "Tracker Bot")

	if got := svc.SystemChatName(ctx); got != "Tracker Bot" {
		t.Fatalf("SystemChatName = %q, want %q", got, "Tracker Bot")
	}

	// Written straight to the repo: this stands in for another process's write,
	// which cannot evict this process's cache.
	_ = repo.Set(ctx, SettingChatSystemDisplayName, "Announcer")
	if got := svc.SystemChatName(ctx); got != "Tracker Bot" {
		t.Fatalf("SystemChatName = %q, want the cached value before the TTL is up", got)
	}

	now = now.Add(cachedSettingTTL + time.Second)
	if got := svc.SystemChatName(ctx); got != "Announcer" {
		t.Fatalf("SystemChatName = %q, want %q once the entry expired", got, "Announcer")
	}
}

func TestSet_ChatSystemDisplayNameValidation(t *testing.T) {
	repo := newMockSiteSettingsRepo()
	svc := NewSiteSettingsService(repo, event.NewInMemoryBus())
	ctx := context.Background()

	t.Run("rejects blank", func(t *testing.T) {
		err := svc.Set(ctx, SettingChatSystemDisplayName, "   ", event.Actor{ID: 1})
		if !errors.Is(err, ErrInvalidSetting) {
			t.Fatalf("Set = %v, want ErrInvalidSetting — a blank label renders as a broken row", err)
		}
	})

	t.Run("rejects an over-long label", func(t *testing.T) {
		err := svc.Set(ctx, SettingChatSystemDisplayName,
			strings.Repeat("a", maxSystemChatNameLength+1), event.Actor{ID: 1})
		if !errors.Is(err, ErrInvalidSetting) {
			t.Fatalf("Set = %v, want ErrInvalidSetting", err)
		}
	})

	t.Run("counts runes, not bytes", func(t *testing.T) {
		// Each of these is three bytes, so a byte-length check would reject a
		// label that renders at a third of the limit.
		if err := svc.Set(ctx, SettingChatSystemDisplayName,
			strings.Repeat("運", maxSystemChatNameLength), event.Actor{ID: 1}); err != nil {
			t.Fatalf("Set: %v", err)
		}
	})

	t.Run("stores the trimmed value", func(t *testing.T) {
		if err := svc.Set(ctx, SettingChatSystemDisplayName, "  Announcer  ", event.Actor{ID: 1}); err != nil {
			t.Fatalf("Set: %v", err)
		}
		if got := svc.SystemChatName(ctx); got != "Announcer" {
			t.Fatalf("SystemChatName = %q, want %q", got, "Announcer")
		}
	})
}

// This setting decides how much of the announce log survives, and therefore how
// far back a ratio dispute can be checked. Accepting a value the worker then reads
// as something else — via
// GetInt's silent fallback, or via a duration overflow — means the admin page shows
// one policy while the prune enforces another.
func TestSet_AnnounceLogRetentionValidation(t *testing.T) {
	repo := newMockSiteSettingsRepo()
	svc := NewSiteSettingsService(repo, event.NewInMemoryBus())
	ctx := context.Background()

	t.Run("rejects non-numeric input", func(t *testing.T) {
		err := svc.Set(ctx, SettingAnnounceLogRetentionDays, "ninety", event.Actor{ID: 1})
		if !errors.Is(err, ErrInvalidSetting) {
			t.Fatalf("Set = %v, want ErrInvalidSetting — GetInt would silently fall back to 90", err)
		}
	})

	t.Run("rejects a value that overflows the duration", func(t *testing.T) {
		// days * 24h overflows int64 nanoseconds above roughly 106,751 days and
		// wraps negative, which the prune reads as "disabled".
		err := svc.Set(ctx, SettingAnnounceLogRetentionDays, "99999999", event.Actor{ID: 1})
		if !errors.Is(err, ErrInvalidSetting) {
			t.Fatalf("Set = %v, want ErrInvalidSetting", err)
		}
	})

	t.Run("accepts zero, which disables pruning", func(t *testing.T) {
		if err := svc.Set(ctx, SettingAnnounceLogRetentionDays, "0", event.Actor{ID: 1}); err != nil {
			t.Fatalf("Set: %v", err)
		}
		if got := svc.GetInt(ctx, SettingAnnounceLogRetentionDays, 90); got != 0 {
			t.Fatalf("GetInt = %d, want 0", got)
		}
	})

	t.Run("accepts an ordinary window", func(t *testing.T) {
		if err := svc.Set(ctx, SettingAnnounceLogRetentionDays, "30", event.Actor{ID: 1}); err != nil {
			t.Fatalf("Set: %v", err)
		}
		if got := svc.GetInt(ctx, SettingAnnounceLogRetentionDays, 90); got != 30 {
			t.Fatalf("GetInt = %d, want 30", got)
		}
	})

	t.Run("accepts the upper bound", func(t *testing.T) {
		if err := svc.Set(ctx, SettingAnnounceLogRetentionDays,
			strconv.Itoa(maxAnnounceLogRetentionDays), event.Actor{ID: 1}); err != nil {
			t.Fatalf("Set: %v", err)
		}
	})
}

// hookedSettingsRepo runs a callback once, from inside Get, after the value has
// been read. That makes a read/write interleaving deterministic without any
// goroutines: the hook stands exactly where a concurrent Set would land.
type hookedSettingsRepo struct {
	*mockSiteSettingsRepo
	onGet func()
}

func (r *hookedSettingsRepo) Get(ctx context.Context, key string) (*model.SiteSetting, error) {
	setting, err := r.mockSiteSettingsRepo.Get(ctx, key)
	if r.onGet != nil {
		hook := r.onGet
		r.onGet = nil
		hook()
	}
	return setting, err
}

// A read that missed the cache holds a value from before a concurrent Set. It
// must not write that value back: the eviction already happened, so the stale
// name would be resurrected with a fresh TTL and stick for five minutes.
func TestSystemChatName_InFlightReadDoesNotResurrectAnEvictedValue(t *testing.T) {
	base := newMockSiteSettingsRepo()
	repo := &hookedSettingsRepo{mockSiteSettingsRepo: base}
	svc := NewSiteSettingsService(repo, event.NewInMemoryBus())
	ctx := context.Background()

	_ = base.Set(ctx, SettingChatSystemDisplayName, "Tracker Bot")

	// Fires while the first SystemChatName call is between "read storage" and
	// "write to cache".
	repo.onGet = func() {
		if err := svc.Set(ctx, SettingChatSystemDisplayName, "Announcer", event.Actor{ID: 1}); err != nil {
			t.Errorf("Set during read: %v", err)
		}
	}

	// This call legitimately returns what it read before the write landed.
	if got := svc.SystemChatName(ctx); got != "Tracker Bot" {
		t.Fatalf("SystemChatName = %q, want the value read before the write", got)
	}

	// The next one must see the new name. It only can if the in-flight read
	// declined to cache itself over the eviction.
	if got := svc.SystemChatName(ctx); got != "Announcer" {
		t.Fatalf("SystemChatName = %q, want %q — a stale read was written back over the eviction", got, "Announcer")
	}
}
