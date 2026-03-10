package listener

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

type mockMessageRepo struct {
	unreadCount int
	unreadErr   error
}

func (m *mockMessageRepo) Create(_ context.Context, _ *model.Message) error { return nil }
func (m *mockMessageRepo) GetByID(_ context.Context, _ int64) (*model.Message, error) {
	return nil, nil
}
func (m *mockMessageRepo) ListInbox(_ context.Context, _ int64, _, _ int) ([]model.Message, int64, error) {
	return nil, 0, nil
}
func (m *mockMessageRepo) ListOutbox(_ context.Context, _ int64, _, _ int) ([]model.Message, int64, error) {
	return nil, 0, nil
}
func (m *mockMessageRepo) MarkAsRead(_ context.Context, _, _ int64) error { return nil }
func (m *mockMessageRepo) DeleteForUser(_ context.Context, _, _ int64) error {
	return nil
}
func (m *mockMessageRepo) CountUnread(_ context.Context, _ int64) (int, error) {
	return m.unreadCount, m.unreadErr
}

type sentPayload struct {
	UserID  int64
	Payload []byte
}

func TestPMNotificationListener_SendsUnreadCount(t *testing.T) {
	bus := event.NewInMemoryBus()
	msgRepo := &mockMessageRepo{unreadCount: 3}

	var mu sync.Mutex
	var sent []sentPayload

	sendToUser := func(userID int64, payload []byte) {
		mu.Lock()
		defer mu.Unlock()
		sent = append(sent, sentPayload{UserID: userID, Payload: payload})
	}

	RegisterPMNotificationListener(bus, msgRepo, sendToUser)

	bus.Publish(context.Background(), &event.MessageSentEvent{
		Base:       event.NewBase(event.MessageSent, event.Actor{ID: 1, Username: "alice"}),
		MessageID:  10,
		ReceiverID: 42,
		Subject:    "Hello",
	})

	mu.Lock()
	defer mu.Unlock()

	if len(sent) != 1 {
		t.Fatalf("expected 1 payload sent, got %d", len(sent))
	}
	if sent[0].UserID != 42 {
		t.Errorf("expected userID 42, got %d", sent[0].UserID)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(sent[0].Payload, &parsed); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
	if parsed["type"] != "pm_notification" {
		t.Errorf("expected type pm_notification, got %v", parsed["type"])
	}
	if parsed["unread_count"] != float64(3) {
		t.Errorf("expected unread_count 3, got %v", parsed["unread_count"])
	}
}

func TestPMNotificationListener_CountUnreadError_DoesNotSend(t *testing.T) {
	bus := event.NewInMemoryBus()
	msgRepo := &mockMessageRepo{unreadErr: errors.New("db connection lost")}

	var mu sync.Mutex
	var sent []sentPayload

	sendToUser := func(userID int64, payload []byte) {
		mu.Lock()
		defer mu.Unlock()
		sent = append(sent, sentPayload{UserID: userID, Payload: payload})
	}

	RegisterPMNotificationListener(bus, msgRepo, sendToUser)

	bus.Publish(context.Background(), &event.MessageSentEvent{
		Base:       event.NewBase(event.MessageSent, event.Actor{ID: 1, Username: "alice"}),
		MessageID:  10,
		ReceiverID: 42,
		Subject:    "Hello",
	})

	mu.Lock()
	defer mu.Unlock()

	if len(sent) != 0 {
		t.Fatalf("expected no payloads sent on CountUnread error, got %d", len(sent))
	}
}

// Verify the listener satisfies the MessageRepository interface at compile time.
var _ repository.MessageRepository = (*mockMessageRepo)(nil)
