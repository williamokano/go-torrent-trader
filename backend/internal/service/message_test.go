package service_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// --- mock message repo ---

type mockMessageRepo struct {
	mu       sync.Mutex
	messages []*model.Message
	nextID   int64
}

func newMockMessageRepo() *mockMessageRepo {
	return &mockMessageRepo{nextID: 1}
}

func (m *mockMessageRepo) Create(_ context.Context, msg *model.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	msg.ID = m.nextID
	m.nextID++
	copy := *msg
	m.messages = append(m.messages, &copy)
	return nil
}

func (m *mockMessageRepo) GetByID(_ context.Context, id int64) (*model.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, msg := range m.messages {
		if msg.ID == id {
			copy := *msg
			if copy.SenderUsername == "" {
				copy.SenderUsername = "sender"
			}
			if copy.ReceiverUsername == "" {
				copy.ReceiverUsername = "receiver"
			}
			return &copy, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (m *mockMessageRepo) ListInbox(_ context.Context, userID int64, page, perPage int) ([]model.Message, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []model.Message
	for _, msg := range m.messages {
		if msg.ReceiverID == userID && !msg.ReceiverDeleted {
			copy := *msg
			if copy.SenderUsername == "" {
				copy.SenderUsername = "sender"
			}
			if copy.ReceiverUsername == "" {
				copy.ReceiverUsername = "receiver"
			}
			result = append(result, copy)
		}
	}
	total := int64(len(result))
	start := (page - 1) * perPage
	if start >= len(result) {
		return nil, total, nil
	}
	end := start + perPage
	if end > len(result) {
		end = len(result)
	}
	return result[start:end], total, nil
}

func (m *mockMessageRepo) ListOutbox(_ context.Context, userID int64, page, perPage int) ([]model.Message, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []model.Message
	for _, msg := range m.messages {
		if msg.SenderID == userID && !msg.SenderDeleted {
			copy := *msg
			if copy.SenderUsername == "" {
				copy.SenderUsername = "sender"
			}
			if copy.ReceiverUsername == "" {
				copy.ReceiverUsername = "receiver"
			}
			result = append(result, copy)
		}
	}
	total := int64(len(result))
	start := (page - 1) * perPage
	if start >= len(result) {
		return nil, total, nil
	}
	end := start + perPage
	if end > len(result) {
		end = len(result)
	}
	return result[start:end], total, nil
}

func (m *mockMessageRepo) MarkAsRead(_ context.Context, id, userID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, msg := range m.messages {
		if msg.ID == id && msg.ReceiverID == userID {
			msg.IsRead = true
			return nil
		}
	}
	return nil
}

func (m *mockMessageRepo) DeleteForUser(_ context.Context, id, userID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, msg := range m.messages {
		if msg.ID == id {
			if msg.SenderID == userID {
				msg.SenderDeleted = true
				return nil
			}
			if msg.ReceiverID == userID {
				msg.ReceiverDeleted = true
				return nil
			}
		}
	}
	return sql.ErrNoRows
}

func (m *mockMessageRepo) CountUnread(_ context.Context, userID int64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, msg := range m.messages {
		if msg.ReceiverID == userID && !msg.IsRead && !msg.ReceiverDeleted {
			count++
		}
	}
	return count, nil
}

// --- mock user repo for message tests ---

type mockUserRepoForMessage struct {
	mu    sync.Mutex
	users []*model.User
}

func newMockUserRepoForMessage() *mockUserRepoForMessage {
	return &mockUserRepoForMessage{
		users: []*model.User{
			{ID: 1, Username: "sender"},
			{ID: 2, Username: "receiver"},
		},
	}
}

func (m *mockUserRepoForMessage) GetByID(_ context.Context, id int64) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, errors.New("user not found")
}

func (m *mockUserRepoForMessage) GetByUsername(context.Context, string) (*model.User, error) {
	return nil, errors.New("not implemented")
}
func (m *mockUserRepoForMessage) GetByEmail(context.Context, string) (*model.User, error) {
	return nil, errors.New("not implemented")
}
func (m *mockUserRepoForMessage) GetByPasskey(context.Context, string) (*model.User, error) {
	return nil, errors.New("not implemented")
}
func (m *mockUserRepoForMessage) Count(context.Context) (int64, error) { return 0, nil }
func (m *mockUserRepoForMessage) Create(context.Context, *model.User) error {
	return nil
}
func (m *mockUserRepoForMessage) Update(context.Context, *model.User) error {
	return nil
}
func (m *mockUserRepoForMessage) IncrementStats(context.Context, int64, int64, int64) error {
	return nil
}
func (m *mockUserRepoForMessage) List(context.Context, repository.ListUsersOptions) ([]model.User, int64, error) {
	return nil, 0, nil
}
func (m *mockUserRepoForMessage) ListStaff(context.Context) ([]model.User, error) {
	return nil, nil
}
func (m *mockUserRepoForMessage) UpdateLastAccess(context.Context, int64) error {
	return nil
}

// --- helpers ---

func setupMessageService() *service.MessageService {
	return service.NewMessageService(
		newMockMessageRepo(),
		newMockUserRepoForMessage(),
		event.NewInMemoryBus(),
	)
}

// --- tests ---

func TestSendMessage_Success(t *testing.T) {
	svc := setupMessageService()
	msg, err := svc.SendMessage(context.Background(), 1, service.SendMessageRequest{
		ReceiverID: 2,
		Subject:    "Hello",
		Body:       "How are you?",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if msg.Subject != "Hello" {
		t.Errorf("expected subject 'Hello', got %q", msg.Subject)
	}
}

func TestSendMessage_EmptySubject(t *testing.T) {
	svc := setupMessageService()
	_, err := svc.SendMessage(context.Background(), 1, service.SendMessageRequest{
		ReceiverID: 2,
		Subject:    "",
		Body:       "Hello",
	})
	if !errors.Is(err, service.ErrInvalidMessage) {
		t.Errorf("expected ErrInvalidMessage, got %v", err)
	}
}

func TestSendMessage_EmptyBody(t *testing.T) {
	svc := setupMessageService()
	_, err := svc.SendMessage(context.Background(), 1, service.SendMessageRequest{
		ReceiverID: 2,
		Subject:    "Hello",
		Body:       "",
	})
	if !errors.Is(err, service.ErrInvalidMessage) {
		t.Errorf("expected ErrInvalidMessage, got %v", err)
	}
}

func TestSendMessage_CannotMessageSelf(t *testing.T) {
	svc := setupMessageService()
	_, err := svc.SendMessage(context.Background(), 1, service.SendMessageRequest{
		ReceiverID: 1,
		Subject:    "Hello",
		Body:       "Talking to myself",
	})
	if !errors.Is(err, service.ErrCannotMessageSelf) {
		t.Errorf("expected ErrCannotMessageSelf, got %v", err)
	}
}

func TestSendMessage_ReceiverNotFound(t *testing.T) {
	svc := setupMessageService()
	_, err := svc.SendMessage(context.Background(), 1, service.SendMessageRequest{
		ReceiverID: 999,
		Subject:    "Hello",
		Body:       "Who are you?",
	})
	if !errors.Is(err, service.ErrInvalidMessage) {
		t.Errorf("expected ErrInvalidMessage, got %v", err)
	}
}

func TestGetMessage_AsReceiver(t *testing.T) {
	svc := setupMessageService()
	sent, _ := svc.SendMessage(context.Background(), 1, service.SendMessageRequest{
		ReceiverID: 2,
		Subject:    "Hello",
		Body:       "Test",
	})

	msg, err := svc.GetMessage(context.Background(), sent.ID, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !msg.IsRead {
		t.Error("expected message to be auto-marked as read")
	}
}

func TestGetMessage_AsSender(t *testing.T) {
	svc := setupMessageService()
	sent, _ := svc.SendMessage(context.Background(), 1, service.SendMessageRequest{
		ReceiverID: 2,
		Subject:    "Hello",
		Body:       "Test",
	})

	msg, err := svc.GetMessage(context.Background(), sent.ID, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.IsRead {
		t.Error("expected message to not be marked as read when sender views")
	}
}

func TestGetMessage_NotFound(t *testing.T) {
	svc := setupMessageService()
	_, err := svc.GetMessage(context.Background(), 999, 1)
	if !errors.Is(err, service.ErrMessageNotFound) {
		t.Errorf("expected ErrMessageNotFound, got %v", err)
	}
}

func TestGetMessage_Unauthorized(t *testing.T) {
	svc := setupMessageService()
	sent, _ := svc.SendMessage(context.Background(), 1, service.SendMessageRequest{
		ReceiverID: 2,
		Subject:    "Hello",
		Body:       "Test",
	})

	// User 3 is neither sender nor receiver
	_, err := svc.GetMessage(context.Background(), sent.ID, 3)
	if !errors.Is(err, service.ErrMessageNotFound) {
		t.Errorf("expected ErrMessageNotFound, got %v", err)
	}
}

func TestListInbox_Pagination(t *testing.T) {
	svc := setupMessageService()

	for i := 0; i < 5; i++ {
		if _, err := svc.SendMessage(context.Background(), 1, service.SendMessageRequest{
			ReceiverID: 2,
			Subject:    "Hello",
			Body:       "Test",
		}); err != nil {
			t.Fatalf("send message: %v", err)
		}
	}

	messages, total, err := svc.ListInbox(context.Background(), 2, 1, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(messages) != 2 {
		t.Errorf("expected 2 messages on page, got %d", len(messages))
	}
}

func TestListOutbox(t *testing.T) {
	svc := setupMessageService()

	if _, err := svc.SendMessage(context.Background(), 1, service.SendMessageRequest{
		ReceiverID: 2,
		Subject:    "Hello",
		Body:       "Test",
	}); err != nil {
		t.Fatalf("send message: %v", err)
	}

	messages, total, err := svc.ListOutbox(context.Background(), 1, 1, 25)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 {
		t.Errorf("expected total 1, got %d", total)
	}
	if len(messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(messages))
	}
}

func TestDeleteMessage_AsReceiver(t *testing.T) {
	svc := setupMessageService()
	sent, _ := svc.SendMessage(context.Background(), 1, service.SendMessageRequest{
		ReceiverID: 2,
		Subject:    "Hello",
		Body:       "Test",
	})

	err := svc.DeleteMessage(context.Background(), sent.ID, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should not appear in receiver's inbox
	messages, total, _ := svc.ListInbox(context.Background(), 2, 1, 25)
	if total != 0 {
		t.Errorf("expected total 0, got %d", total)
	}
	if len(messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(messages))
	}

	// Should still appear in sender's outbox
	messages, total, _ = svc.ListOutbox(context.Background(), 1, 1, 25)
	if total != 1 {
		t.Errorf("expected total 1, got %d", total)
	}
	if len(messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(messages))
	}
}

func TestDeleteMessage_NotFound(t *testing.T) {
	svc := setupMessageService()
	err := svc.DeleteMessage(context.Background(), 999, 1)
	if !errors.Is(err, service.ErrMessageNotFound) {
		t.Errorf("expected ErrMessageNotFound, got %v", err)
	}
}

func TestCountUnread(t *testing.T) {
	svc := setupMessageService()

	for i := 0; i < 3; i++ {
		if _, err := svc.SendMessage(context.Background(), 1, service.SendMessageRequest{
			ReceiverID: 2,
			Subject:    "Hello",
			Body:       "Test",
		}); err != nil {
			t.Fatalf("send message: %v", err)
		}
	}

	count, err := svc.CountUnread(context.Background(), 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 unread, got %d", count)
	}

	// Read one message
	messages, _, _ := svc.ListInbox(context.Background(), 2, 1, 25)
	if len(messages) > 0 {
		_, _ = svc.GetMessage(context.Background(), messages[0].ID, 2)
	}

	count, err = svc.CountUnread(context.Background(), 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 unread, got %d", count)
	}
}

func TestListInbox_DefaultPagination(t *testing.T) {
	svc := setupMessageService()
	messages, total, err := svc.ListInbox(context.Background(), 2, 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 0 {
		t.Errorf("expected total 0, got %d", total)
	}
	if len(messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(messages))
	}
}
