package model

import (
	"encoding/json"
	"time"
)

// NotificationGroup is a read-time collapse of one or more Notification rows
// that share a grouping key. Today only topic_reply notifications for the
// same topic are folded together; every other type is represented as its
// own singleton group (Count == 1) so callers have a single shape to render
// regardless of whether an entry was collapsed.
//
// This type is purely a query-time view: it is never persisted, and nothing
// about the underlying Notification rows changes when they are grouped. See
// NotificationService.ListGrouped (backend/internal/service/notification_group.go)
// for how it's built and why grouping happens at read time.
type NotificationGroup struct {
	// Key identifies the group within a single listing response. It is
	// derived, not persisted: "topic_reply:<topic_id>" for collapsed
	// entries, "single:<notification_id>" otherwise.
	Key             string
	Type            string
	Count           int
	Unread          bool // true if any underlying notification is unread
	LastActors      []string
	LatestCreatedAt time.Time
	Data            json.RawMessage // most recent underlying notification's data
	Notifications   []Notification  // underlying notifications, newest first
}
