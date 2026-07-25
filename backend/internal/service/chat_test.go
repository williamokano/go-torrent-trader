package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// --- mock chat message repository ---

type mockChatMessageRepo struct {
	messages []model.ChatMessage
	nextID   int64
}

func newMockChatMessageRepo() *mockChatMessageRepo {
	return &mockChatMessageRepo{nextID: 1}
}

func (r *mockChatMessageRepo) Create(_ context.Context, msg *model.ChatMessage) error {
	msg.ID = r.nextID
	r.nextID++
	msg.CreatedAt = time.Now()
	r.messages = append(r.messages, *msg)
	return nil
}

func (r *mockChatMessageRepo) ListRecent(_ context.Context, limit int) ([]model.ChatMessage, error) {
	if limit > len(r.messages) {
		limit = len(r.messages)
	}
	start := len(r.messages) - limit
	if start < 0 {
		start = 0
	}
	result := make([]model.ChatMessage, len(r.messages[start:]))
	copy(result, r.messages[start:])
	return result, nil
}

func (r *mockChatMessageRepo) ListBefore(_ context.Context, beforeID int64, limit int) ([]model.ChatMessage, error) {
	var filtered []model.ChatMessage
	for _, m := range r.messages {
		if m.ID < beforeID {
			filtered = append(filtered, m)
		}
	}
	if limit > len(filtered) {
		limit = len(filtered)
	}
	start := len(filtered) - limit
	if start < 0 {
		start = 0
	}
	result := make([]model.ChatMessage, len(filtered[start:]))
	copy(result, filtered[start:])
	return result, nil
}

func (r *mockChatMessageRepo) Delete(_ context.Context, id int64) error {
	for i, m := range r.messages {
		if m.ID == id {
			r.messages = append(r.messages[:i], r.messages[i+1:]...)
			return nil
		}
	}
	return sql.ErrNoRows
}

func (r *mockChatMessageRepo) DeleteByUserID(_ context.Context, userID int64) (int64, error) {
	var kept []model.ChatMessage
	var count int64
	for _, m := range r.messages {
		if m.UserID == userID {
			count++
		} else {
			kept = append(kept, m)
		}
	}
	r.messages = kept
	return count, nil
}

// --- mock chat mute repository ---

type mockChatMuteRepo struct {
	mutes  []model.ChatMute
	nextID int64
}

func newMockChatMuteRepo() *mockChatMuteRepo {
	return &mockChatMuteRepo{nextID: 1}
}

func (r *mockChatMuteRepo) Create(_ context.Context, mute *model.ChatMute) error {
	mute.ID = r.nextID
	r.nextID++
	mute.CreatedAt = time.Now()
	r.mutes = append(r.mutes, *mute)
	return nil
}

func (r *mockChatMuteRepo) GetActiveMute(_ context.Context, userID int64) (*model.ChatMute, error) {
	for _, m := range r.mutes {
		if m.UserID == userID && m.ExpiresAt.After(time.Now()) {
			return &m, nil
		}
	}
	return nil, nil
}

func (r *mockChatMuteRepo) Delete(_ context.Context, userID int64) error {
	var kept []model.ChatMute
	for _, m := range r.mutes {
		if m.UserID != userID {
			kept = append(kept, m)
		}
	}
	r.mutes = kept
	return nil
}

func (r *mockChatMuteRepo) ListActive(_ context.Context, _, _ int) ([]repository.ChatMuteWithNames, int64, error) {
	return nil, 0, nil
}

func (r *mockChatMuteRepo) DeleteExpired(_ context.Context) ([]int64, error) {
	var kept []model.ChatMute
	var userIDs []int64
	now := time.Now()
	for _, m := range r.mutes {
		if m.ExpiresAt.Before(now) || m.ExpiresAt.Equal(now) {
			userIDs = append(userIDs, m.UserID)
		} else {
			kept = append(kept, m)
		}
	}
	r.mutes = kept
	return userIDs, nil
}

// --- mock user repo for chat tests ---

type mockChatUserRepo struct {
	users map[int64]*model.User
}

func newMockChatUserRepo() *mockChatUserRepo {
	return &mockChatUserRepo{users: make(map[int64]*model.User)}
}

func (r *mockChatUserRepo) GetByID(_ context.Context, id int64) (*model.User, error) {
	u, ok := r.users[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return u, nil
}

func (r *mockChatUserRepo) GetByUsername(context.Context, string) (*model.User, error) {
	return nil, sql.ErrNoRows
}
func (r *mockChatUserRepo) GetByUsernames(context.Context, []string) ([]model.User, error) {
	return nil, nil
}
func (r *mockChatUserRepo) GetByEmail(context.Context, string) (*model.User, error) {
	return nil, sql.ErrNoRows
}
func (r *mockChatUserRepo) GetByPasskey(context.Context, string) (*model.User, error) {
	return nil, sql.ErrNoRows
}
func (r *mockChatUserRepo) Count(context.Context) (int64, error) { return 0, nil }
func (r *mockChatUserRepo) Create(context.Context, *model.User) error {
	return nil
}
func (r *mockChatUserRepo) Update(context.Context, *model.User) error {
	return nil
}
func (r *mockChatUserRepo) IncrementStats(context.Context, int64, int64, int64) error {
	return nil
}
func (r *mockChatUserRepo) List(context.Context, repository.ListUsersOptions) ([]model.User, int64, error) {
	return nil, 0, nil
}
func (r *mockChatUserRepo) ListStaff(context.Context) ([]model.User, error) {
	return nil, nil
}
func (r *mockChatUserRepo) UpdateLastAccess(context.Context, int64) error {
	return nil
}

// --- helper ---

func newTestChatService() (*ChatService, *mockChatMessageRepo, *mockChatMuteRepo) {
	bus := event.NewInMemoryBus()
	chatRepo := newMockChatMessageRepo()
	muteRepo := newMockChatMuteRepo()
	userRepo := newMockChatUserRepo()
	userRepo.users[1] = &model.User{ID: 1, Username: "alice", CanChat: true, CanDownload: true, CanUpload: true}
	userRepo.users[2] = &model.User{ID: 2, Username: "bob", CanChat: true, CanDownload: true, CanUpload: true}

	svc := NewChatService(chatRepo, muteRepo, userRepo, bus, nil)
	return svc, chatRepo, muteRepo
}

func TestChatService_SendMessage(t *testing.T) {
	svc, _, _ := newTestChatService()
	ctx := context.Background()

	t.Run("valid message", func(t *testing.T) {
		msg, err := svc.SendMessage(ctx, 1, "hello world")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if msg.Message != "hello world" {
			t.Errorf("expected message 'hello world', got %q", msg.Message)
		}
		if msg.Username != "alice" {
			t.Errorf("expected username 'alice', got %q", msg.Username)
		}
		if msg.ID == 0 {
			t.Error("expected non-zero ID")
		}
	})

	t.Run("empty message", func(t *testing.T) {
		_, err := svc.SendMessage(ctx, 1, "   ")
		if !errors.Is(err, ErrInvalidChatMessage) {
			t.Errorf("expected ErrInvalidChatMessage, got %v", err)
		}
	})

	t.Run("message too long", func(t *testing.T) {
		long := strings.Repeat("a", 501)
		_, err := svc.SendMessage(ctx, 1, long)
		if !errors.Is(err, ErrInvalidChatMessage) {
			t.Errorf("expected ErrInvalidChatMessage, got %v", err)
		}
	})

	t.Run("trims whitespace", func(t *testing.T) {
		msg, err := svc.SendMessage(ctx, 1, "  trimmed  ")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if msg.Message != "trimmed" {
			t.Errorf("expected 'trimmed', got %q", msg.Message)
		}
	})
}

func TestChatService_SendMessage_Muted(t *testing.T) {
	svc, _, muteRepo := newTestChatService()
	ctx := context.Background()

	// Mute user 1
	muteRepo.mutes = append(muteRepo.mutes, model.ChatMute{
		ID:        1,
		UserID:    1,
		MutedBy:   ptrInt64(2),
		ExpiresAt: time.Now().Add(10 * time.Minute),
		CreatedAt: time.Now(),
	})

	_, err := svc.SendMessage(ctx, 1, "hello")
	if !errors.Is(err, ErrChatMuted) {
		t.Errorf("expected ErrChatMuted, got %v", err)
	}
}

func TestChatService_ListRecent(t *testing.T) {
	svc, _, _ := newTestChatService()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if _, err := svc.SendMessage(ctx, 1, "msg"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	t.Run("default limit", func(t *testing.T) {
		msgs, err := svc.ListRecent(ctx, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(msgs) != 5 {
			t.Errorf("expected 5 messages, got %d", len(msgs))
		}
	})

	t.Run("custom limit", func(t *testing.T) {
		msgs, err := svc.ListRecent(ctx, 3)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(msgs) != 3 {
			t.Errorf("expected 3 messages, got %d", len(msgs))
		}
	})

	t.Run("caps at max", func(t *testing.T) {
		msgs, err := svc.ListRecent(ctx, 200)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(msgs) != 5 {
			t.Errorf("expected 5 messages, got %d", len(msgs))
		}
	})
}

func TestChatService_ListHistory(t *testing.T) {
	svc, _, _ := newTestChatService()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if _, err := svc.SendMessage(ctx, 1, "msg"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	msgs, err := svc.ListHistory(ctx, 4, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 3 {
		t.Errorf("expected 3 messages before id=4, got %d", len(msgs))
	}
}

func TestChatService_DeleteMessage(t *testing.T) {
	svc, _, _ := newTestChatService()
	ctx := context.Background()

	msg, err := svc.SendMessage(ctx, 1, "to be deleted")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	staffPerms := model.Permissions{IsAdmin: true}
	regularPerms := model.Permissions{}

	t.Run("non-staff cannot delete", func(t *testing.T) {
		err := svc.DeleteMessage(ctx, msg.ID, 2, regularPerms)
		if !errors.Is(err, ErrForbidden) {
			t.Errorf("expected ErrForbidden, got %v", err)
		}
	})

	t.Run("staff can delete", func(t *testing.T) {
		err := svc.DeleteMessage(ctx, msg.ID, 1, staffPerms)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("delete non-existent", func(t *testing.T) {
		err := svc.DeleteMessage(ctx, 999, 1, staffPerms)
		if !errors.Is(err, ErrChatMessageNotFound) {
			t.Errorf("expected ErrChatMessageNotFound, got %v", err)
		}
	})
}

func TestChatService_DeleteUserMessages(t *testing.T) {
	svc, _, _ := newTestChatService()
	ctx := context.Background()
	staffPerms := model.Permissions{IsAdmin: true}
	regularPerms := model.Permissions{}

	// Create messages from two users
	for i := 0; i < 3; i++ {
		if _, err := svc.SendMessage(ctx, 1, "alice msg"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	for i := 0; i < 2; i++ {
		if _, err := svc.SendMessage(ctx, 2, "bob msg"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	t.Run("non-staff cannot delete", func(t *testing.T) {
		_, err := svc.DeleteUserMessages(ctx, 1, 2, regularPerms)
		if !errors.Is(err, ErrForbidden) {
			t.Errorf("expected ErrForbidden, got %v", err)
		}
	})

	t.Run("staff can delete all user messages", func(t *testing.T) {
		count, err := svc.DeleteUserMessages(ctx, 1, 2, staffPerms)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 3 {
			t.Errorf("expected 3 deleted, got %d", count)
		}

		// Bob's messages should still be there
		msgs, err := svc.ListRecent(ctx, 50)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(msgs) != 2 {
			t.Errorf("expected 2 remaining messages, got %d", len(msgs))
		}
	})
}

func TestChatService_MuteUnmute(t *testing.T) {
	svc, _, _ := newTestChatService()
	ctx := context.Background()
	staffPerms := model.Permissions{IsAdmin: true}
	regularPerms := model.Permissions{}

	t.Run("non-staff cannot mute", func(t *testing.T) {
		_, err := svc.MuteUser(ctx, 1, 2, 10, "spam", regularPerms)
		if !errors.Is(err, ErrForbidden) {
			t.Errorf("expected ErrForbidden, got %v", err)
		}
	})

	t.Run("staff can mute", func(t *testing.T) {
		mute, err := svc.MuteUser(ctx, 1, 2, 10, "spam", staffPerms)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mute.UserID != 1 {
			t.Errorf("expected user_id 1, got %d", mute.UserID)
		}
		if mute.Reason != "spam" {
			t.Errorf("expected reason 'spam', got %q", mute.Reason)
		}
	})

	t.Run("muted user cannot send", func(t *testing.T) {
		_, err := svc.SendMessage(ctx, 1, "hello")
		if !errors.Is(err, ErrChatMuted) {
			t.Errorf("expected ErrChatMuted, got %v", err)
		}
	})

	t.Run("non-staff cannot unmute", func(t *testing.T) {
		err := svc.UnmuteUser(ctx, 1, 2, regularPerms)
		if !errors.Is(err, ErrForbidden) {
			t.Errorf("expected ErrForbidden, got %v", err)
		}
	})

	t.Run("staff can unmute", func(t *testing.T) {
		err := svc.UnmuteUser(ctx, 1, 2, staffPerms)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// User can send again
		_, err = svc.SendMessage(ctx, 1, "hello again")
		if err != nil {
			t.Fatalf("expected user to send after unmute, got: %v", err)
		}
	})
}

func TestChatService_SystemMuteUser(t *testing.T) {
	svc, _, _ := newTestChatService()
	ctx := context.Background()

	t.Run("auto-mutes user without staff permissions", func(t *testing.T) {
		mute, err := svc.SystemMuteUser(ctx, 1, 5, "Automatic mute: chat spam/flooding")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mute.UserID != 1 {
			t.Errorf("expected user_id 1, got %d", mute.UserID)
		}
		if mute.MutedBy != nil {
			t.Errorf("expected muted_by nil (system), got %v", mute.MutedBy)
		}
		if mute.Reason != "Automatic mute: chat spam/flooding" {
			t.Errorf("unexpected reason: %q", mute.Reason)
		}

		// Verify user is actually muted
		_, err = svc.SendMessage(ctx, 1, "should fail")
		if !errors.Is(err, ErrChatMuted) {
			t.Errorf("expected ErrChatMuted after system mute, got %v", err)
		}
	})

	t.Run("rejects zero duration", func(t *testing.T) {
		_, err := svc.SystemMuteUser(ctx, 2, 0, "test")
		if err == nil {
			t.Error("expected error for zero duration")
		}
	})

	t.Run("rejects negative duration", func(t *testing.T) {
		_, err := svc.SystemMuteUser(ctx, 2, -1, "test")
		if err == nil {
			t.Error("expected error for negative duration")
		}
	})
}

func TestChatService_CleanupExpiredMutes(t *testing.T) {
	svc, _, muteRepo := newTestChatService()
	ctx := context.Background()

	// Add an expired mute
	muteRepo.mutes = append(muteRepo.mutes, model.ChatMute{
		ID:        1,
		UserID:    1,
		MutedBy:   ptrInt64(2),
		ExpiresAt: time.Now().Add(-1 * time.Hour),
		CreatedAt: time.Now().Add(-2 * time.Hour),
	})
	// Add an active mute
	muteRepo.mutes = append(muteRepo.mutes, model.ChatMute{
		ID:        2,
		UserID:    2,
		MutedBy:   ptrInt64(1),
		ExpiresAt: time.Now().Add(1 * time.Hour),
		CreatedAt: time.Now(),
	})

	unmutedUsers, err := svc.CleanupExpiredMutes(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(unmutedUsers) != 1 {
		t.Errorf("expected 1 cleaned, got %d", len(unmutedUsers))
	}
	if len(muteRepo.mutes) != 1 {
		t.Errorf("expected 1 remaining mute, got %d", len(muteRepo.mutes))
	}
}

func ptrInt64(v int64) *int64 { return &v }

// --- system messages (BE-10.1) ---

func TestSendSystemMessageStoresAuthorlessRow(t *testing.T) {
	messages := newMockChatMessageRepo()
	svc := NewChatService(messages, newMockChatMuteRepo(), newMockChatUserRepo(), event.NewInMemoryBus(), nil)

	msg, err := svc.SendSystemMessage(context.Background(), "  New torrent: Thing  ")
	if err != nil {
		t.Fatalf("SendSystemMessage: %v", err)
	}

	if !msg.System {
		t.Fatal("system flag not set")
	}
	if msg.UserID != 0 {
		t.Fatalf("user_id = %d, want 0: a system message has no author", msg.UserID)
	}
	if msg.Username != model.SystemChatUsername {
		t.Fatalf("username = %q, want %q", msg.Username, model.SystemChatUsername)
	}
	if msg.Message != "New torrent: Thing" {
		t.Fatalf("message = %q, want it trimmed", msg.Message)
	}
	if len(messages.messages) != 1 {
		t.Fatalf("stored %d messages, want 1", len(messages.messages))
	}
}

// Mutes and chat-privilege checks police users; there is no user behind a
// system announcement, so they must not apply to it.
func TestSendSystemMessageIgnoresMutesAndPrivileges(t *testing.T) {
	// Both repos fail on every call, so the message can only get through if
	// SendSystemMessage never consults them at all.
	messages := newMockChatMessageRepo()
	svc := NewChatService(messages, &explodingMuteRepo{}, &explodingUserRepo{}, event.NewInMemoryBus(), nil)

	msg, err := svc.SendSystemMessage(context.Background(), "announcement")
	if err != nil {
		t.Fatalf("SendSystemMessage consulted the mute/privilege checks: %v", err)
	}
	if !msg.System {
		t.Fatal("system flag not set")
	}
}

// The exploding repositories fail the one lookup SendMessage makes and
// SendSystemMessage must not, so an accidental check is impossible to miss.

type explodingMuteRepo struct{ mockChatMuteRepo }

func (e *explodingMuteRepo) GetActiveMute(context.Context, int64) (*model.ChatMute, error) {
	return nil, errors.New("mute repository must not be consulted for a system message")
}

type explodingUserRepo struct{ mockChatUserRepo }

func (e *explodingUserRepo) GetByID(context.Context, int64) (*model.User, error) {
	return nil, errors.New("user repository must not be consulted for a system message")
}

func TestSendSystemMessageRejectsEmptyAndOversized(t *testing.T) {
	svc := NewChatService(newMockChatMessageRepo(), newMockChatMuteRepo(), newMockChatUserRepo(), event.NewInMemoryBus(), nil)

	if _, err := svc.SendSystemMessage(context.Background(), "   "); !errors.Is(err, ErrInvalidChatMessage) {
		t.Fatalf("err = %v, want ErrInvalidChatMessage for an empty message", err)
	}
	if _, err := svc.SendSystemMessage(context.Background(), strings.Repeat("x", maxChatMessageLength+1)); !errors.Is(err, ErrInvalidChatMessage) {
		t.Fatalf("err = %v, want ErrInvalidChatMessage for an oversized message", err)
	}
}

func TestSendSystemMessagePropagatesStorageFailure(t *testing.T) {
	messages := &failingChatMessageRepo{}
	svc := NewChatService(messages, newMockChatMuteRepo(), newMockChatUserRepo(), event.NewInMemoryBus(), nil)

	if _, err := svc.SendSystemMessage(context.Background(), "announcement"); err == nil {
		t.Fatal("expected a storage failure to surface so the delivery retries")
	}
}

// failingChatMessageRepo fails every write, standing in for a database outage.
type failingChatMessageRepo struct{ mockChatMessageRepo }

func (r *failingChatMessageRepo) Create(context.Context, *model.ChatMessage) error {
	return errors.New("database down")
}

// --- system message labelling ---

// newLabelledChatService wires a chat service whose system label is name.
func newLabelledChatService(t *testing.T, messages repository.ChatMessageRepository, name string) *ChatService {
	t.Helper()

	settingsRepo := newMockSiteSettingsRepo()
	if err := settingsRepo.Set(context.Background(), SettingChatSystemDisplayName, name); err != nil {
		t.Fatalf("seed setting: %v", err)
	}
	return NewChatService(messages, newMockChatMuteRepo(), newMockChatUserRepo(), event.NewInMemoryBus(),
		NewSiteSettingsService(settingsRepo, event.NewInMemoryBus()))
}

func TestSendSystemMessageUsesTheConfiguredName(t *testing.T) {
	svc := newLabelledChatService(t, newMockChatMessageRepo(), "Tracker Bot")

	msg, err := svc.SendSystemMessage(context.Background(), "New torrent: Thing")
	if err != nil {
		t.Fatalf("SendSystemMessage: %v", err)
	}
	if msg.Username != "Tracker Bot" {
		t.Fatalf("username = %q, want %q", msg.Username, "Tracker Bot")
	}
}

// The repository hands system rows back unlabelled, so a page that is not
// relabelled here would render an announcement with a blank author.
func TestListRecentLabelsSystemRows(t *testing.T) {
	messages := newMockChatMessageRepo()
	svc := newLabelledChatService(t, messages, "Tracker Bot")
	ctx := context.Background()

	// Written straight to the repository, the way a row comes back from
	// Postgres: system rows carry no username.
	if err := messages.Create(ctx, &model.ChatMessage{Message: "New torrent: Thing", System: true}); err != nil {
		t.Fatalf("create system message: %v", err)
	}
	if err := messages.Create(ctx, &model.ChatMessage{UserID: 2, Username: "bob", Message: "hi"}); err != nil {
		t.Fatalf("create user message: %v", err)
	}

	got, err := svc.ListRecent(ctx, 10)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d messages, want 2", len(got))
	}
	for _, m := range got {
		if m.System && m.Username != "Tracker Bot" {
			t.Fatalf("system row username = %q, want %q", m.Username, "Tracker Bot")
		}
		// An authored row keeps the name the JOIN resolved.
		if !m.System && m.Username != "bob" {
			t.Fatalf("user row username = %q, want %q", m.Username, "bob")
		}
	}
}

func TestListHistoryLabelsSystemRows(t *testing.T) {
	messages := newMockChatMessageRepo()
	svc := newLabelledChatService(t, messages, "Tracker Bot")
	ctx := context.Background()

	if err := messages.Create(ctx, &model.ChatMessage{Message: "older announcement", System: true}); err != nil {
		t.Fatalf("create system message: %v", err)
	}
	if err := messages.Create(ctx, &model.ChatMessage{UserID: 2, Username: "bob", Message: "newer"}); err != nil {
		t.Fatalf("create user message: %v", err)
	}

	got, err := svc.ListHistory(ctx, 2, 10)
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d messages, want 1", len(got))
	}
	if got[0].Username != "Tracker Bot" {
		t.Fatalf("system row username = %q, want %q", got[0].Username, "Tracker Bot")
	}
}

// Site settings are injected after construction, so every path has to survive
// the dependency being absent. For a display label the safe answer is the
// built-in default — unlike an authorization check, where absent must mean "no".
func TestSystemLabelFallsBackWithoutSiteSettings(t *testing.T) {
	messages := newMockChatMessageRepo()
	svc := NewChatService(messages, newMockChatMuteRepo(), newMockChatUserRepo(), event.NewInMemoryBus(), nil)
	ctx := context.Background()

	msg, err := svc.SendSystemMessage(ctx, "announcement")
	if err != nil {
		t.Fatalf("SendSystemMessage: %v", err)
	}
	if msg.Username != model.SystemChatUsername {
		t.Fatalf("username = %q, want %q", msg.Username, model.SystemChatUsername)
	}

	got, err := svc.ListRecent(ctx, 10)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if got[0].Username != model.SystemChatUsername {
		t.Fatalf("listed username = %q, want %q", got[0].Username, model.SystemChatUsername)
	}
}

// A page of ordinary chat must not pay for a settings read at all.
func TestListRecentSkipsTheSettingsReadWithoutSystemRows(t *testing.T) {
	messages := newMockChatMessageRepo()
	settingsRepo := newMockSiteSettingsRepo()
	svc := NewChatService(messages, newMockChatMuteRepo(), newMockChatUserRepo(), event.NewInMemoryBus(),
		NewSiteSettingsService(settingsRepo, event.NewInMemoryBus()))
	ctx := context.Background()

	if err := messages.Create(ctx, &model.ChatMessage{UserID: 2, Username: "bob", Message: "hi"}); err != nil {
		t.Fatalf("create user message: %v", err)
	}
	if _, err := svc.ListRecent(ctx, 10); err != nil {
		t.Fatalf("ListRecent: %v", err)
	}

	if got := settingsRepo.getCount(); got != 0 {
		t.Fatalf("read settings %d times for a page with no announcements, want 0", got)
	}
}

// The frontend guard that used to hide the delete button is gone, so the
// service is now the only thing standing between a member and an announcement.
// It must let staff through and keep everyone else out.
func TestDeleteMessageAppliesToSystemRows(t *testing.T) {
	messages := newMockChatMessageRepo()
	svc := newLabelledChatService(t, messages, "Tracker Bot")
	ctx := context.Background()

	if err := messages.Create(ctx, &model.ChatMessage{Message: "New torrent: Oops", System: true}); err != nil {
		t.Fatalf("create system message: %v", err)
	}
	id := messages.messages[0].ID

	member := model.Permissions{}
	if err := svc.DeleteMessage(ctx, id, 5, member); !errors.Is(err, ErrForbidden) {
		t.Fatalf("member delete = %v, want ErrForbidden", err)
	}
	if len(messages.messages) != 1 {
		t.Fatal("a member's refused delete removed the row anyway")
	}

	if err := svc.DeleteMessage(ctx, id, 1, model.Permissions{IsAdmin: true}); err != nil {
		t.Fatalf("staff delete of a system message: %v", err)
	}
	if len(messages.messages) != 0 {
		t.Fatal("staff delete left the announcement in place")
	}
}
