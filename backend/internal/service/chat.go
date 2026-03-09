package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

var (
	ErrInvalidChatMessage  = errors.New("invalid chat message")
	ErrChatMessageNotFound = errors.New("chat message not found")
	ErrChatMuted           = errors.New("you are muted")
)

const (
	maxChatMessageLength  = 500
	defaultChatLimit      = 50
	maxChatLimit          = 100
	maxMuteDurationMinutes = 43200 // 30 days
)

// ChatService handles chat/shoutbox business logic.
type ChatService struct {
	messages repository.ChatMessageRepository
	mutes    repository.ChatMuteRepository
	users    repository.UserRepository
	eventBus event.Bus
}

// NewChatService creates a new ChatService.
func NewChatService(
	messages repository.ChatMessageRepository,
	mutes repository.ChatMuteRepository,
	users repository.UserRepository,
	bus event.Bus,
) *ChatService {
	return &ChatService{
		messages:  messages,
		mutes:     mutes,
		users:     users,
		eventBus:  bus,
	}
}

// SendMessage creates a new chat message. Returns ErrChatMuted if the user is muted.
func (s *ChatService) SendMessage(ctx context.Context, userID int64, message string) (*model.ChatMessage, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return nil, fmt.Errorf("%w: message cannot be empty", ErrInvalidChatMessage)
	}
	if len(message) > maxChatMessageLength {
		return nil, fmt.Errorf("%w: message exceeds %d characters", ErrInvalidChatMessage, maxChatMessageLength)
	}

	// Check mute status.
	mute, err := s.mutes.GetActiveMute(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("check mute: %w", err)
	}
	if mute != nil {
		remaining := time.Until(mute.ExpiresAt).Minutes()
		if remaining < 1 {
			remaining = 1
		}
		return nil, fmt.Errorf("%w for %.0f more minutes", ErrChatMuted, remaining)
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

// DeleteUserMessages removes all chat messages from a specific user. Staff only.
func (s *ChatService) DeleteUserMessages(ctx context.Context, userID, actorID int64, perms model.Permissions) (int64, error) {
	if !perms.IsStaff() {
		return 0, ErrForbidden
	}

	count, err := s.messages.DeleteByUserID(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("delete user chat messages: %w", err)
	}

	s.eventBus.Publish(ctx, &event.ChatUserMessagesDeletedEvent{
		Base:         event.NewBase(event.ChatUserMessagesDeleted, event.Actor{ID: actorID}),
		TargetUserID: userID,
		Count:        count,
	})

	return count, nil
}

// MuteUser mutes a user in chat for the given duration. Staff only.
func (s *ChatService) MuteUser(ctx context.Context, userID, actorID int64, durationMinutes int, reason string, perms model.Permissions) (*model.ChatMute, error) {
	if !perms.IsStaff() {
		return nil, ErrForbidden
	}

	if durationMinutes <= 0 {
		return nil, fmt.Errorf("%w: duration must be positive", ErrInvalidChatMessage)
	}
	if durationMinutes > maxMuteDurationMinutes {
		return nil, fmt.Errorf("%w: duration cannot exceed %d minutes (30 days)", ErrInvalidChatMessage, maxMuteDurationMinutes)
	}

	mute := &model.ChatMute{
		UserID:    userID,
		MutedBy:   actorID,
		Reason:    strings.TrimSpace(reason),
		ExpiresAt: time.Now().Add(time.Duration(durationMinutes) * time.Minute),
	}

	if err := s.mutes.Create(ctx, mute); err != nil {
		return nil, fmt.Errorf("create chat mute: %w", err)
	}

	return mute, nil
}

// UnmuteUser removes all mutes for a user. Staff only.
func (s *ChatService) UnmuteUser(ctx context.Context, userID, actorID int64, perms model.Permissions) error {
	if !perms.IsStaff() {
		return ErrForbidden
	}

	if err := s.mutes.Delete(ctx, userID); err != nil {
		return fmt.Errorf("unmute user: %w", err)
	}

	return nil
}

// CleanupExpiredMutes deletes expired mute records. Returns count deleted.
func (s *ChatService) CleanupExpiredMutes(ctx context.Context) (int64, error) {
	return s.mutes.DeleteExpired(ctx)
}
