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
	UserBanned       Type = "user_banned"
	UserUnbanned     Type = "user_unbanned"
	UserWarned       Type = "user_warned"
	UserUnwarned     Type = "user_unwarned"
	UserGroupChanged Type = "user_group_changed"
	UserDeleted      Type = "user_deleted"
	TorrentUploaded Type = "torrent_uploaded"
	TorrentEdited   Type = "torrent_edited"
	TorrentDeleted  Type = "torrent_deleted"
	TorrentReported Type = "torrent_reported"
	ReportResolved  Type = "report_resolved"
	CommentCreated    Type = "comment_created"
	CommentDeleted    Type = "comment_deleted"
	ReseedRequested         Type = "reseed_requested"
	InviteCreated           Type = "invite_created"
	InviteRedeemed          Type = "invite_redeemed"
	RegistrationModeChanged Type = "registration_mode_changed"
	EmailBanned             Type = "email_banned"
	EmailUnbanned           Type = "email_unbanned"
	IPBanned                Type = "ip_banned"
	IPUnbanned              Type = "ip_unbanned"
	MessageSent             Type = "message_sent"
	ChatMessageDeleted      Type = "chat_message_deleted"
	ChatUserMessagesDeleted Type = "chat_user_messages_deleted"
	ChatUserMuted           Type = "chat_user_muted"
	ChatUserUnmuted         Type = "chat_user_unmuted"
	WarningIssued           Type = "warning_issued"
	WarningLifted           Type = "warning_lifted"
	NewsPublished           Type = "news_published"
	SiteSettingChanged      Type = "site_setting_changed"
	PasswordReset           Type = "password_reset"
	PasskeyReset            Type = "passkey_reset"
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

type UserUnbannedEvent struct {
	Base
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
}

type UserUnwarnedEvent struct {
	Base
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
}

type UserGroupChangedEvent struct {
	Base
	UserID       int64  `json:"user_id"`
	Username     string `json:"username"`
	OldGroupName string `json:"old_group_name"`
	NewGroupName string `json:"new_group_name"`
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
	TorrentID   int64  `json:"torrent_id"`
	TorrentName string `json:"torrent_name"`
	Reason      string `json:"reason"`
}

type ReportResolvedEvent struct {
	Base
	ReportID int64 `json:"report_id"`
}

type CommentCreatedEvent struct {
	Base
	CommentID   int64  `json:"comment_id"`
	TorrentID   int64  `json:"torrent_id"`
	TorrentName string `json:"torrent_name"`
}

type CommentDeletedEvent struct {
	Base
	CommentID   int64  `json:"comment_id"`
	TorrentID   int64  `json:"torrent_id"`
	TorrentName string `json:"torrent_name"`
}

type ReseedRequestedEvent struct {
	Base
	TorrentID     int64  `json:"torrent_id"`
	TorrentName   string `json:"torrent_name"`
	UploaderID    int64  `json:"uploader_id"`
	UploaderEmail string `json:"uploader_email"`
}

type InviteCreatedEvent struct {
	Base
	InviteID int64 `json:"invite_id"`
}

type InviteRedeemedEvent struct {
	Base
	InviteID        int64  `json:"invite_id"`
	InviteeID       int64  `json:"invitee_id"`
	InviteeUsername string `json:"invitee_username"`
	Token           string `json:"token"`
}

type RegistrationModeChangedEvent struct {
	Base
	OldMode string `json:"old_mode"`
	NewMode string `json:"new_mode"`
}

type EmailBannedEvent struct {
	Base
	Pattern string `json:"pattern"`
}

type EmailUnbannedEvent struct {
	Base
	Pattern string `json:"pattern"`
}

type IPBannedEvent struct {
	Base
	IPRange string `json:"ip_range"`
}

type IPUnbannedEvent struct {
	Base
	IPRange string `json:"ip_range"`
}

type MessageSentEvent struct {
	Base
	MessageID  int64  `json:"message_id"`
	ReceiverID int64  `json:"receiver_id"`
	Subject    string `json:"subject"`
}

type ChatMessageDeletedEvent struct {
	Base
	MessageID int64 `json:"message_id"`
}

type ChatUserMessagesDeletedEvent struct {
	Base
	TargetUserID int64 `json:"target_user_id"`
	Count        int64 `json:"count"`
}

type ChatUserMutedEvent struct {
	Base
	TargetUserID    int64  `json:"target_user_id"`
	DurationMinutes int    `json:"duration_minutes"`
	Reason          string `json:"reason"`
}

type ChatUserUnmutedEvent struct {
	Base
	TargetUserID int64 `json:"target_user_id"`
}

type WarningIssuedEvent struct {
	Base
	WarningID   int64  `json:"warning_id"`
	UserID      int64  `json:"user_id"`
	Username    string `json:"username"`
	WarningType string `json:"warning_type"`
}

type WarningLiftedEvent struct {
	Base
	WarningID int64  `json:"warning_id"`
	UserID    int64  `json:"user_id"`
	Username  string `json:"username"`
}

type NewsPublishedEvent struct {
	Base
	ArticleID int64  `json:"article_id"`
	Title     string `json:"title"`
}

type SiteSettingChangedEvent struct {
	Base
	Key      string `json:"key"`
	OldValue string `json:"old_value"`
	NewValue string `json:"new_value"`
}

type PasswordResetEvent struct {
	Base
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
}

type PasskeyResetEvent struct {
	Base
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
}
