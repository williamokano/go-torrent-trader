package model

import (
	"encoding/json"
	"time"
)

// Notification type constants.
const (
	NotifForumReply     = "forum_reply"
	NotifMention        = "mention"
	NotifTopicReply     = "topic_reply"
	NotifTorrentComment = "torrent_comment"
	NotifPMReceived     = "pm_received"
	NotifSystem         = "system"
	// NotifModerationMessage notifies the uploader + assigned moderator of a new
	// message in a torrent's moderation thread (BE-8.22b).
	NotifModerationMessage = "moderation_message"
	// NotifModerationDecision notifies the uploader that their torrent was
	// approved or rejected.
	NotifModerationDecision = "moderation_decision"
)

// AllNotificationTypes lists all valid notification types for preference management.
var AllNotificationTypes = []string{
	NotifForumReply,
	NotifMention,
	NotifTopicReply,
	NotifTorrentComment,
	NotifPMReceived,
	NotifSystem,
	NotifModerationMessage,
	NotifModerationDecision,
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

// Digest frequency constants.
const (
	DigestOff    = "off"
	DigestDaily  = "daily"
	DigestWeekly = "weekly"
)

// AllDigestFrequencies lists every valid digest frequency for preference validation.
var AllDigestFrequencies = []string{DigestOff, DigestDaily, DigestWeekly}

// NotificationDigestPreference stores a user's email digest cadence and the
// cursor (LastDigestSentAt) used to compute "unread since last digest".
type NotificationDigestPreference struct {
	UserID           int64
	Frequency        string
	LastDigestSentAt *time.Time
}

// DigestRecipient is a user due for a digest run, with the fields needed to
// build and send their digest email.
type DigestRecipient struct {
	UserID           int64
	Email            string
	LastDigestSentAt *time.Time
}

// TopicSubscription represents a user's subscription to a forum topic.
type TopicSubscription struct {
	UserID    int64
	TopicID   int64
	CreatedAt time.Time
}
