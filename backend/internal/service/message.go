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
	ErrMessageNotFound  = errors.New("message not found")
	ErrInvalidMessage   = errors.New("invalid message")
	ErrCannotMessageSelf = errors.New("cannot send message to yourself")
)

// SendMessageRequest holds the data needed to send a private message.
type SendMessageRequest struct {
	ReceiverID int64  `json:"receiver_id"`
	Subject    string `json:"subject"`
	Body       string `json:"body"`
	ParentID   *int64 `json:"parent_id,omitempty"`
}

// MessageService handles private message business logic.
type MessageService struct {
	messages repository.MessageRepository
	users    repository.UserRepository
	eventBus event.Bus
}

// NewMessageService creates a new MessageService.
func NewMessageService(
	messages repository.MessageRepository,
	users repository.UserRepository,
	bus event.Bus,
) *MessageService {
	return &MessageService{
		messages: messages,
		users:    users,
		eventBus: bus,
	}
}

// SendMessage creates and sends a private message.
func (s *MessageService) SendMessage(ctx context.Context, senderID int64, req SendMessageRequest) (*model.Message, error) {
	subject := strings.TrimSpace(req.Subject)
	body := strings.TrimSpace(req.Body)

	if subject == "" {
		return nil, fmt.Errorf("%w: subject cannot be empty", ErrInvalidMessage)
	}
	if body == "" {
		return nil, fmt.Errorf("%w: body cannot be empty", ErrInvalidMessage)
	}
	if senderID == req.ReceiverID {
		return nil, ErrCannotMessageSelf
	}

	// Verify receiver exists.
	if _, err := s.users.GetByID(ctx, req.ReceiverID); err != nil {
		return nil, fmt.Errorf("%w: receiver not found", ErrInvalidMessage)
	}

	// Validate parent_id if provided: must exist and sender must be a participant.
	if req.ParentID != nil {
		parentMsg, err := s.messages.GetByID(ctx, *req.ParentID)
		if err != nil {
			return nil, fmt.Errorf("%w: parent message not found", ErrInvalidMessage)
		}
		if parentMsg.SenderID != senderID && parentMsg.ReceiverID != senderID {
			return nil, fmt.Errorf("%w: you are not a participant in the parent conversation", ErrInvalidMessage)
		}
	}

	msg := &model.Message{
		SenderID:   senderID,
		ReceiverID: req.ReceiverID,
		Subject:    subject,
		Body:       body,
		ParentID:   req.ParentID,
	}

	if err := s.messages.Create(ctx, msg); err != nil {
		return nil, fmt.Errorf("create message: %w", err)
	}

	// Re-fetch to get usernames from JOINs.
	created, err := s.messages.GetByID(ctx, msg.ID)
	if err != nil {
		return nil, fmt.Errorf("get created message: %w", err)
	}

	s.eventBus.Publish(ctx, &event.MessageSentEvent{
		Base:       event.NewBase(event.MessageSent, event.Actor{ID: senderID, Username: created.SenderUsername}),
		MessageID:  created.ID,
		ReceiverID: created.ReceiverID,
		Subject:    created.Subject,
	})

	return created, nil
}

// GetMessage retrieves a single message. The caller must be the sender or receiver.
func (s *MessageService) GetMessage(ctx context.Context, id, userID int64) (*model.Message, error) {
	msg, err := s.messages.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrMessageNotFound
		}
		return nil, ErrMessageNotFound
	}

	// Enforce visibility: user must be sender or receiver, and not have deleted it.
	if msg.SenderID == userID && msg.SenderDeleted {
		return nil, ErrMessageNotFound
	}
	if msg.ReceiverID == userID && msg.ReceiverDeleted {
		return nil, ErrMessageNotFound
	}
	if msg.SenderID != userID && msg.ReceiverID != userID {
		return nil, ErrMessageNotFound
	}

	// Auto-mark as read if the receiver is viewing.
	if msg.ReceiverID == userID && !msg.IsRead {
		if err := s.messages.MarkAsRead(ctx, id, userID); err != nil {
			return nil, fmt.Errorf("mark as read: %w", err)
		}
		msg.IsRead = true
	}

	return msg, nil
}

// ListInbox returns paginated inbox messages for a user.
func (s *MessageService) ListInbox(ctx context.Context, userID int64, page, perPage int) ([]model.Message, int64, error) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}
	return s.messages.ListInbox(ctx, userID, page, perPage)
}

// ListOutbox returns paginated outbox messages for a user.
func (s *MessageService) ListOutbox(ctx context.Context, userID int64, page, perPage int) ([]model.Message, int64, error) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}
	return s.messages.ListOutbox(ctx, userID, page, perPage)
}

// MarkAsRead marks a message as read. Only the receiver can do this.
func (s *MessageService) MarkAsRead(ctx context.Context, id, userID int64) error {
	return s.messages.MarkAsRead(ctx, id, userID)
}

// DeleteMessage soft-deletes a message for the current user.
func (s *MessageService) DeleteMessage(ctx context.Context, id, userID int64) error {
	err := s.messages.DeleteForUser(ctx, id, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrMessageNotFound
		}
		return fmt.Errorf("delete message: %w", err)
	}
	return nil
}

// CountUnread returns the number of unread messages for a user.
func (s *MessageService) CountUnread(ctx context.Context, userID int64) (int, error) {
	return s.messages.CountUnread(ctx, userID)
}
