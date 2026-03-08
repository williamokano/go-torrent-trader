package event

import (
	"context"
	"time"
)

// Type identifies the kind of domain event.
type Type string

const (
	UserRegistered Type = "user_registered"
	UserLogin      Type = "user_login"
	UserBanned     Type = "user_banned"
	UserWarned      Type = "user_warned"
	UserDeleted     Type = "user_deleted"
	TorrentUploaded Type = "torrent_uploaded"
	TorrentEdited   Type = "torrent_edited"
	TorrentDeleted  Type = "torrent_deleted"
	TorrentReported Type = "torrent_reported"
	ReportResolved  Type = "report_resolved"
	CommentCreated  Type = "comment_created"
	CommentDeleted  Type = "comment_deleted"
)

// Event is the base interface for all domain events.
type Event interface {
	EventType() Type
	OccurredAt() time.Time
}

// Handler processes a domain event. Returning an error is logged but does not
// prevent other handlers from running.
type Handler func(ctx context.Context, evt Event) error

// Bus is the interface for publishing and subscribing to domain events.
// The in-memory implementation dispatches synchronously. A future SQS or
// message-queue implementation could dispatch asynchronously.
type Bus interface {
	Publish(ctx context.Context, evt Event)
	Subscribe(eventType Type, handler Handler)
}

// Actor identifies who triggered the event. Carries enough context so handlers
// don't need to look up the user again.
type Actor struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

// Base holds common fields shared by all events.
type Base struct {
	Type      Type      `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Actor     Actor     `json:"actor"`
}

func (b Base) EventType() Type      { return b.Type }
func (b Base) OccurredAt() time.Time { return b.Timestamp }

// NewBase creates a Base with the given type and actor.
func NewBase(t Type, actor Actor) Base {
	return Base{Type: t, Timestamp: time.Now(), Actor: actor}
}

// --- Concrete event types ---

type UserRegisteredEvent struct {
	Base
	UserID int64 `json:"user_id"`
}

type UserLoginEvent struct {
	Base
	UserID int64  `json:"user_id"`
	IP     string `json:"ip"`
}

type UserBannedEvent struct {
	Base
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
}

type UserWarnedEvent struct {
	Base
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
}

type UserDeletedEvent struct {
	Base
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
}

type TorrentUploadedEvent struct {
	Base
	TorrentID   int64  `json:"torrent_id"`
	TorrentName string `json:"torrent_name"`
}

type TorrentEditedEvent struct {
	Base
	TorrentID   int64  `json:"torrent_id"`
	TorrentName string `json:"torrent_name"`
}

type TorrentDeletedEvent struct {
	Base
	TorrentID   int64  `json:"torrent_id"`
	TorrentName string `json:"torrent_name"`
}

type TorrentReportedEvent struct {
	Base
	TorrentID int64  `json:"torrent_id"`
	Reason    string `json:"reason"`
}

type ReportResolvedEvent struct {
	Base
	ReportID int64 `json:"report_id"`
}

type CommentCreatedEvent struct {
	Base
	CommentID int64 `json:"comment_id"`
	TorrentID int64 `json:"torrent_id"`
}

type CommentDeletedEvent struct {
	Base
	CommentID int64 `json:"comment_id"`
	TorrentID int64 `json:"torrent_id"`
}
