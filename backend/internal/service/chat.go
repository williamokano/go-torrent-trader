package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

var (
	ErrInvalidChatMessage  = errors.New("invalid chat message")
	ErrChatMessageNotFound = errors.New("chat message not found")
)

const (
	maxChatMessageLength = 500
	defaultChatLimit     = 50
	maxChatLimit         = 100
)

// ChatService handles chat/shoutbox business logic.
type ChatService struct {
	messages repository.ChatMessageRepository
	users    repository.UserRepository
	eventBus event.Bus
}

// NewChatService creates a new ChatService.
func NewChatService(
	messages repository.ChatMessageRepository,
	users repository.UserRepository,
	bus event.Bus,
) *ChatService {
	return &ChatService{
		messages: messages,
		users:    users,
		eventBus: bus,
	}
}

// SendMessage creates a new chat message.
func (s *ChatService) SendMessage(ctx context.Context, userID int64, message string) (*model.ChatMessage, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return nil, fmt.Errorf("%w: message cannot be empty", ErrInvalidChatMessage)
	}
	if len(message) > maxChatMessageLength {
		return nil, fmt.Errorf("%w: message exceeds %d characters", ErrInvalidChatMessage, maxChatMessageLength)
	}

	// Look up the username for the response.
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	msg := &model.ChatMessage{
		UserID:  userID,
		Message: message,
	}

	if err := s.messages.Create(ctx, msg); err != nil {
		return nil, fmt.Errorf("create chat message: %w", err)
	}

	msg.Username = user.Username

	return msg, nil
}

// ListRecent returns the most recent chat messages in chronological order.
func (s *ChatService) ListRecent(ctx context.Context, limit int) ([]model.ChatMessage, error) {
	if limit <= 0 {
		limit = defaultChatLimit
	}
	if limit > maxChatLimit {
		limit = maxChatLimit
	}
	return s.messages.ListRecent(ctx, limit)
}

// ListHistory returns chat messages older than the given ID for pagination.
func (s *ChatService) ListHistory(ctx context.Context, beforeID int64, limit int) ([]model.ChatMessage, error) {
	if limit <= 0 {
		limit = defaultChatLimit
	}
	if limit > maxChatLimit {
		limit = maxChatLimit
	}
	return s.messages.ListBefore(ctx, beforeID, limit)
}

// DeleteMessage removes a chat message. Only staff may delete.
func (s *ChatService) DeleteMessage(ctx context.Context, id, actorID int64, perms model.Permissions) error {
	if !perms.IsStaff() {
		return ErrForbidden
	}

	if err := s.messages.Delete(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrChatMessageNotFound
		}
		return fmt.Errorf("delete chat message: %w", err)
	}

	s.eventBus.Publish(ctx, &event.ChatMessageDeletedEvent{
		Base:      event.NewBase(event.ChatMessageDeleted, event.Actor{ID: actorID}),
		MessageID: id,
	})

	return nil
}
