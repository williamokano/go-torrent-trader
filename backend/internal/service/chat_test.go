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

func TestChatService_SendMessage(t *testing.T) {
	bus := event.NewInMemoryBus()
	chatRepo := newMockChatMessageRepo()
	userRepo := newMockChatUserRepo()
	userRepo.users[1] = &model.User{ID: 1, Username: "alice"}

	svc := NewChatService(chatRepo, userRepo, bus)
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

func TestChatService_ListRecent(t *testing.T) {
	bus := event.NewInMemoryBus()
	chatRepo := newMockChatMessageRepo()
	userRepo := newMockChatUserRepo()
	userRepo.users[1] = &model.User{ID: 1, Username: "alice"}

	svc := NewChatService(chatRepo, userRepo, bus)
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
	bus := event.NewInMemoryBus()
	chatRepo := newMockChatMessageRepo()
	userRepo := newMockChatUserRepo()
	userRepo.users[1] = &model.User{ID: 1, Username: "alice"}

	svc := NewChatService(chatRepo, userRepo, bus)
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
	bus := event.NewInMemoryBus()
	chatRepo := newMockChatMessageRepo()
	userRepo := newMockChatUserRepo()
	userRepo.users[1] = &model.User{ID: 1, Username: "alice"}

	svc := NewChatService(chatRepo, userRepo, bus)
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
