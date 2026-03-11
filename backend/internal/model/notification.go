package model

import (
	"encoding/json"
	"time"
)

// Notification type constants.
const (
	NotifForumReply     = "forum_reply"
	NotifForumMention   = "forum_mention"
	NotifTopicReply     = "topic_reply"
	NotifTorrentComment = "torrent_comment"
	NotifPMReceived     = "pm_received"
	NotifSystem         = "system"
)

// AllNotificationTypes lists all valid notification types for preference management.
var AllNotificationTypes = []string{
	NotifForumReply,
	NotifForumMention,
	NotifTopicReply,
	NotifTorrentComment,
	NotifPMReceived,
	NotifSystem,
}

// Notification represents an in-app notification for a user.
type Notification struct {
	ID        int64
	UserID    int64
	Type      string
	Data      json.RawMessage
	Read      bool
	CreatedAt time.Time
}

// NotificationPreference stores a user's per-type notification toggle.
type NotificationPreference struct {
	UserID           int64
	NotificationType string
	Enabled          bool
}

// TopicSubscription represents a user's subscription to a forum topic.
type TopicSubscription struct {
	UserID    int64
	TopicID   int64
	CreatedAt time.Time
}
