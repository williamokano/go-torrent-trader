package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

var (
	ErrNotificationNotFound = errors.New("notification not found")
	ErrInvalidNotification  = errors.New("invalid notification")
)

// NotificationService handles notification business logic.
type NotificationService struct {
	notifications repository.NotificationRepository
	preferences   repository.NotificationPreferenceRepository
	subscriptions repository.TopicSubscriptionRepository
	topics        repository.ForumTopicRepository
	forums        repository.ForumRepository
	sendToUser    func(userID int64, payload []byte)
}

// NewNotificationService creates a new NotificationService.
func NewNotificationService(
	notifications repository.NotificationRepository,
	preferences repository.NotificationPreferenceRepository,
	subscriptions repository.TopicSubscriptionRepository,
	topics repository.ForumTopicRepository,
	forums repository.ForumRepository,
	sendToUser func(userID int64, payload []byte),
) *NotificationService {
	return &NotificationService{
		notifications: notifications,
		preferences:   preferences,
		subscriptions: subscriptions,
		topics:        topics,
		forums:        forums,
		sendToUser:    sendToUser,
	}
}

// Create creates a notification for a user, checking preferences and self-notification.
// Returns nil without error if the notification was skipped (disabled or self-notify).
func (s *NotificationService) Create(ctx context.Context, userID int64, actorID int64, notifType string, data json.RawMessage) (*model.Notification, error) {
	// Don't notify yourself
	if userID == actorID {
		return nil, nil
	}

	// Check user preferences
	enabled, err := s.preferences.IsEnabled(ctx, userID, notifType)
	if err != nil {
		return nil, fmt.Errorf("check preference: %w", err)
	}
	if !enabled {
		return nil, nil
	}

	notif := &model.Notification{
		UserID: userID,
		Type:   notifType,
		Data:   data,
	}
	if err := s.notifications.Create(ctx, notif); err != nil {
		return nil, fmt.Errorf("create notification: %w", err)
	}

	// Push via WebSocket
	s.pushNotification(ctx, notif)

	return notif, nil
}

// List returns paginated notifications for a user.
func (s *NotificationService) List(ctx context.Context, userID int64, page, perPage int, unreadOnly bool) ([]model.Notification, int64, error) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}
	return s.notifications.List(ctx, userID, repository.ListNotificationsOptions{
		UnreadOnly: unreadOnly,
		Page:       page,
		PerPage:    perPage,
	})
}

// MarkRead marks a single notification as read.
func (s *NotificationService) MarkRead(ctx context.Context, userID, notifID int64) error {
	return s.notifications.MarkRead(ctx, userID, notifID)
}

// MarkAllRead marks all notifications as read for a user.
func (s *NotificationService) MarkAllRead(ctx context.Context, userID int64) error {
	return s.notifications.MarkAllRead(ctx, userID)
}

// UnreadCount returns the number of unread notifications for a user.
func (s *NotificationService) UnreadCount(ctx context.Context, userID int64) (int, error) {
	return s.notifications.CountUnread(ctx, userID)
}

// GetPreferences returns all notification preferences for a user.
// Includes defaults for types without explicit preferences.
func (s *NotificationService) GetPreferences(ctx context.Context, userID int64) ([]model.NotificationPreference, error) {
	stored, err := s.preferences.GetAll(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get preferences: %w", err)
	}

	// Build map of stored preferences
	storedMap := make(map[string]bool)
	for _, p := range stored {
		storedMap[p.NotificationType] = p.Enabled
	}

	// Return all types with defaults (true) for unset types
	result := make([]model.NotificationPreference, 0, len(model.AllNotificationTypes))
	for _, t := range model.AllNotificationTypes {
		enabled := true
		if v, ok := storedMap[t]; ok {
			enabled = v
		}
		result = append(result, model.NotificationPreference{
			UserID:           userID,
			NotificationType: t,
			Enabled:          enabled,
		})
	}
	return result, nil
}

// SetPreference sets a notification preference for a user.
func (s *NotificationService) SetPreference(ctx context.Context, userID int64, notifType string, enabled bool) error {
	// Validate notification type
	valid := false
	for _, t := range model.AllNotificationTypes {
		if t == notifType {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("%w: unknown notification type %q", ErrInvalidNotification, notifType)
	}
	return s.preferences.Set(ctx, userID, notifType, enabled)
}

// Subscribe subscribes a user to topic notifications.
// Validates that the topic exists. Forum access is not checked here because
// internal callers (auto-subscribe) have already passed access checks.
func (s *NotificationService) Subscribe(ctx context.Context, userID, topicID int64) error {
	if s.topics != nil {
		if _, err := s.topics.GetByID(ctx, topicID); err != nil {
			return ErrTopicNotFound
		}
	}
	return s.subscriptions.Subscribe(ctx, userID, topicID)
}

// SubscribeWithAccessCheck subscribes after verifying forum access level.
func (s *NotificationService) SubscribeWithAccessCheck(ctx context.Context, userID, topicID int64, perms model.Permissions) error {
	if s.topics != nil {
		topic, err := s.topics.GetByID(ctx, topicID)
		if err != nil {
			return ErrTopicNotFound
		}
		if s.forums != nil {
			forum, err := s.forums.GetByID(ctx, topic.ForumID)
			if err != nil {
				return ErrForumNotFound
			}
			if forum.MinGroupLevel > perms.Level {
				return ErrForbidden
			}
		}
	}
	return s.subscriptions.Subscribe(ctx, userID, topicID)
}

// Unsubscribe removes a user's topic subscription.
func (s *NotificationService) Unsubscribe(ctx context.Context, userID, topicID int64) error {
	return s.subscriptions.Unsubscribe(ctx, userID, topicID)
}

// IsSubscribed checks if a user is subscribed to a topic.
func (s *NotificationService) IsSubscribed(ctx context.Context, userID, topicID int64) (bool, error) {
	return s.subscriptions.IsSubscribed(ctx, userID, topicID)
}

// pushNotification sends a real-time notification to the user via WebSocket.
func (s *NotificationService) pushNotification(ctx context.Context, notif *model.Notification) {
	if s.sendToUser == nil {
		return
	}

	unread, err := s.notifications.CountUnread(ctx, notif.UserID)
	if err != nil {
		slog.Error("notification: failed to count unread", "user_id", notif.UserID, "error", err)
		return
	}

	payload, err := json.Marshal(map[string]interface{}{
		"type": "notification",
		"notification": map[string]interface{}{
			"id":         notif.ID,
			"type":       notif.Type,
			"data":       notif.Data,
			"created_at": notif.CreatedAt,
		},
		"unread_count": unread,
	})
	if err != nil {
		slog.Error("notification: failed to marshal payload", "error", err)
		return
	}

	s.sendToUser(notif.UserID, payload)
}
