package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// UserRepository defines persistence operations for users.
type UserRepository interface {
	GetByID(ctx context.Context, id int64) (*model.User, error)
	GetByUsername(ctx context.Context, username string) (*model.User, error)
	// GetByUsernames resolves multiple usernames in one round trip. Unknown
	// usernames are simply absent from the result — not an error.
	GetByUsernames(ctx context.Context, usernames []string) ([]model.User, error)
	GetByEmail(ctx context.Context, email string) (*model.User, error)
	GetByPasskey(ctx context.Context, passkey string) (*model.User, error)
	Count(ctx context.Context) (int64, error)
	Create(ctx context.Context, user *model.User) error
	Update(ctx context.Context, user *model.User) error
	IncrementStats(ctx context.Context, id int64, uploadedDelta, downloadedDelta int64) error
	List(ctx context.Context, opts ListUsersOptions) ([]model.User, int64, error)
	ListStaff(ctx context.Context) ([]model.User, error)
	UpdateLastAccess(ctx context.Context, id int64) error
}

// ListUsersOptions holds filtering and pagination options for listing users.
type ListUsersOptions struct {
	Search              string
	GroupID             *int64
	Enabled             *bool
	DisabledUntilBefore *time.Time // Filter users whose disabled_until IS NOT NULL AND < this time
	SortBy              string     // username, created_at, uploaded, downloaded
	SortOrder           string     // asc, desc
	Page                int
	PerPage             int
}

// TorrentRepository defines persistence operations for torrents.
type TorrentRepository interface {
	GetByID(ctx context.Context, id int64) (*model.Torrent, error)
	GetByInfoHash(ctx context.Context, infoHash []byte) (*model.Torrent, error)
	List(ctx context.Context, opts ListTorrentsOptions) ([]model.Torrent, int64, error)
	ListByUploader(ctx context.Context, uploaderID int64, limit int) ([]model.Torrent, error)
	Create(ctx context.Context, torrent *model.Torrent) error
	Update(ctx context.Context, torrent *model.Torrent) error
	Delete(ctx context.Context, id int64) error
	IncrementSeeders(ctx context.Context, id int64, delta int) error
	IncrementLeechers(ctx context.Context, id int64, delta int) error
	IncrementTimesCompleted(ctx context.Context, id int64) error
}

// PeerRepository defines persistence operations for peers.
type PeerRepository interface {
	GetByTorrentAndUser(ctx context.Context, torrentID, userID int64) (*model.Peer, error)
	GetByTorrentUserAndPeerID(ctx context.Context, torrentID, userID int64, peerID []byte) (*model.Peer, error)
	ListByTorrent(ctx context.Context, torrentID int64, limit int) ([]model.Peer, error)
	ListByUserSeeding(ctx context.Context, userID int64, page, perPage int) ([]PeerWithTorrent, int64, error)
	ListByUserLeeching(ctx context.Context, userID int64, page, perPage int) ([]PeerWithTorrent, int64, error)
	CountByUser(ctx context.Context, userID int64) (seeding int, leeching int, err error)
	CountByTorrent(ctx context.Context, torrentID int64) (int, error)
	CountTotalByUser(ctx context.Context, userID int64) (int, error)
	Upsert(ctx context.Context, peer *model.Peer) error
	Delete(ctx context.Context, torrentID, userID int64, peerID []byte) error
	DeleteStale(ctx context.Context, before time.Time) (int64, error)
}

// PeerWithTorrent is a peer joined with torrent name for activity views.
type PeerWithTorrent struct {
	model.Peer
	TorrentName string
}

// TransferHistoryRepository defines persistence operations for transfer history.
type TransferHistoryRepository interface {
	Upsert(ctx context.Context, th *model.TransferHistory) error
	ListByUser(ctx context.Context, userID int64, page, perPage int) ([]TransferHistoryWithTorrent, int64, error)
}

// TransferHistoryWithTorrent is a transfer history entry with torrent name.
type TransferHistoryWithTorrent struct {
	model.TransferHistory
	TorrentName string
}

// AnnounceEventRepository defines persistence operations for the append-only
// announce event log.
type AnnounceEventRepository interface {
	Create(ctx context.Context, event *model.AnnounceEvent) error
	ListByUser(ctx context.Context, userID int64, page, perPage int) ([]AnnounceEventWithTorrent, int64, error)
}

// AnnounceEventWithTorrent is an announce event joined with the torrent name.
type AnnounceEventWithTorrent struct {
	model.AnnounceEvent
	TorrentName string
}

// Bonus purchase sentinels. They originate inside the purchase transaction
// (only the repo can observe them), so they live here rather than in service.
var (
	ErrInsufficientBonusPoints = errors.New("insufficient bonus points")
	ErrBonusKindNotAvailable   = errors.New("bonus item kind not available")
)

// BonusRepository defines persistence operations for the bonus point economy.
// All bonus_points writes happen here via atomic statements — never through
// UserRepository.Update — so awards, purchases, and admin adjustments cannot
// clobber each other.
type BonusRepository interface {
	ListEnabledItems(ctx context.Context) ([]model.BonusStoreItem, error)
	GetItem(ctx context.Context, id int64) (*model.BonusStoreItem, error)
	// AwardPoints applies one cycle's awards: per user an atomic balance
	// increment plus a ledger row, in a single transaction. Non-positive
	// deltas are skipped.
	AwardPoints(ctx context.Context, awards map[int64]int64, reason string) error
	// SetPoints sets an absolute balance and records the delta as an
	// admin_adjust ledger row referencing the acting admin, plus the given
	// user_edit_history entries, all in one transaction.
	SetPoints(ctx context.Context, userID, newBalance, actorID int64, entries []model.UserEditHistory) error
	// PurchaseItem owns the whole purchase transaction: conditional balance
	// decrement, reward application, and ledger insert.
	PurchaseItem(ctx context.Context, userID, itemID int64) (*model.BonusStoreItem, int64, error)
	// SeedingCounts returns the number of distinct torrents each user is
	// currently seeding, keyed by user id.
	SeedingCounts(ctx context.Context) (map[int64]int64, error)
}

// PromotionRepository defines persistence operations for the auto class
// promotion ladder and engine.
type PromotionRepository interface {
	// Rule configuration.
	ListRules(ctx context.Context) ([]model.PromotionRule, error)
	UpsertRule(ctx context.Context, rule *model.PromotionRule) error
	DeleteRule(ctx context.Context, groupID int64) error

	// Engine inputs and effects.
	LadderMetrics(ctx context.Context, groupIDs []int64) ([]model.PromotionUserMetrics, error)
	SeedHoursByUser(ctx context.Context, since time.Time, capSeconds int) (map[int64]int, error)
	SetUserGroup(ctx context.Context, userID, groupID int64) error

	// Run bookkeeping.
	LastRunAt(ctx context.Context) (time.Time, bool, error)
	RecordRun(ctx context.Context, promoted, demoted int) error
}

// InviteDistributionRepository defines persistence operations for the auto
// invite distribution rules and engine.
type InviteDistributionRepository interface {
	// Rule configuration.
	ListRules(ctx context.Context) ([]model.InviteDistributionRule, error)
	UpsertRule(ctx context.Context, rule *model.InviteDistributionRule) error
	DeleteRule(ctx context.Context, groupID int64) error

	// Engine inputs. GroupMetrics reuses the same per-user
	// uploaded/downloaded/tenure query PromotionRepository.LadderMetrics
	// uses — there is exactly one query for that shape in the codebase.
	// UserInviteStates is a bulk lookup (keyed by user id) for the fields
	// GroupMetrics doesn't carry: current invite balance and the can_invite
	// privilege flag.
	GroupMetrics(ctx context.Context, groupIDs []int64) ([]model.PromotionUserMetrics, error)
	UserInviteStates(ctx context.Context, userIDs []int64) (map[int64]model.UserInviteState, error)

	// Run bookkeeping.
	LastRunAt(ctx context.Context) (time.Time, bool, error)
	RecordRun(ctx context.Context, granted int) error
}

// ReportRepository defines persistence operations for reports.
type ReportRepository interface {
	Create(ctx context.Context, report *model.Report) error
	GetByID(ctx context.Context, id int64) (*model.Report, error)
	ExistsByReporterAndTorrent(ctx context.Context, reporterID int64, torrentID *int64) (bool, error)
	List(ctx context.Context, opts ListReportsOptions) ([]model.Report, int64, error)
	Resolve(ctx context.Context, id, resolvedByUserID int64) error
}

// CommentRepository defines persistence operations for torrent comments.
type CommentRepository interface {
	Create(ctx context.Context, comment *model.Comment) error
	GetByID(ctx context.Context, id int64) (*model.Comment, error)
	ListByTorrent(ctx context.Context, torrentID int64, page, perPage int) ([]model.Comment, int64, error)
	Update(ctx context.Context, comment *model.Comment) error
	Delete(ctx context.Context, id int64) error
}

// RatingRepository defines persistence operations for torrent ratings.
type RatingRepository interface {
	Upsert(ctx context.Context, rating *model.Rating) error
	GetByTorrentAndUser(ctx context.Context, torrentID, userID int64) (*model.Rating, error)
	GetStatsByTorrent(ctx context.Context, torrentID int64) (average float64, count int, err error)
}

// MetadataFilterOp enumerates the comparison a MetadataFilter applies to a
// torrent's JSONB metadata.
type MetadataFilterOp string

const (
	// MetaFilterEq matches by JSONB containment: metadata @> {key: value}.
	MetaFilterEq MetadataFilterOp = "eq"
	// MetaFilterGte matches numeric fields where (metadata->>key)::numeric >= value.
	MetaFilterGte MetadataFilterOp = "gte"
	// MetaFilterLte matches numeric fields where (metadata->>key)::numeric <= value.
	MetaFilterLte MetadataFilterOp = "lte"
)

// MetadataFilter is a single typed predicate on a torrent's JSONB metadata.
// Value has already been coerced to the field's real type (float64, bool,
// string, or []string for multiselect containment) from the category schema,
// so the repository builds the SQL predicate without re-parsing.
type MetadataFilter struct {
	Key   string
	Op    MetadataFilterOp
	Value any
}

// ListTorrentsOptions holds filtering and pagination options for listing torrents.
type ListTorrentsOptions struct {
	CategoryID       *int64
	Search           string
	SortBy           string // name, created_at, size, seeders, leechers
	SortOrder        string // asc, desc
	Page             int
	PerPage          int
	CreatedAfter     *time.Time       // for "today's torrents"
	MaxSeeders       *int             // for "need seed" (seeders <= N)
	UploaderID       *int64           // for "my uploads" or user's torrents
	ExcludeAnonymous bool             // when true, filter out anonymous torrents (for non-owner/non-staff viewers)
	IncludeHidden    bool             // when true, skip visible/banned filters (admin context)
	MetadataFilters  []MetadataFilter // category-schema metadata predicates (BE-3.13a)
}

// GroupRepository defines persistence operations for groups.
type GroupRepository interface {
	GetByID(ctx context.Context, id int64) (*model.Group, error)
	List(ctx context.Context) ([]model.Group, error)
}

// GroupWriteRepository defines mutating operations for groups. It is kept
// separate from GroupRepository so the many read-only consumers (and their
// mocks) don't have to implement write methods they never use.
type GroupWriteRepository interface {
	Create(ctx context.Context, group *model.Group) error
	Update(ctx context.Context, group *model.Group) error
	Delete(ctx context.Context, id int64) error
	// CountMembers returns how many users belong to the group, used to block
	// deletion of a group that still has members (the users FK would reject it).
	CountMembers(ctx context.Context, groupID int64) (int, error)
}

// ActivityLogRepository defines persistence operations for activity logs.
type ActivityLogRepository interface {
	Create(ctx context.Context, log *model.ActivityLog) error
	List(ctx context.Context, opts ListActivityLogsOptions) ([]model.ActivityLog, int64, error)
}

// ListActivityLogsOptions holds filtering and pagination options for activity logs.
type ListActivityLogsOptions struct {
	EventType *string
	ActorID   *int64
	// IncludeStaffOnly is interpreted by the service layer: when false, the
	// service populates ExcludeEventTypes with the staff-only event types.
	IncludeStaffOnly bool
	// ExcludeEventTypes filters out entries with these event types.
	ExcludeEventTypes []string
	Page              int
	PerPage           int
}

// ReseedRequestRepository defines persistence operations for reseed requests.
type ReseedRequestRepository interface {
	Create(ctx context.Context, req *model.ReseedRequest) error
	ExistsByTorrentAndUser(ctx context.Context, torrentID, userID int64) (bool, error)
	CountByTorrent(ctx context.Context, torrentID int64) (int, error)
}

// InviteRepository defines persistence operations for invites.
type InviteRepository interface {
	Create(ctx context.Context, invite *model.Invite) error
	GetByID(ctx context.Context, id int64) (*model.Invite, error)
	GetByToken(ctx context.Context, token string) (*model.Invite, error)
	ListByInviter(ctx context.Context, inviterID int64, page, perPage int) ([]model.Invite, int64, error)
	Redeem(ctx context.Context, token string, inviteeID int64) error
	CountPendingByInviter(ctx context.Context, inviterID int64) (int, error)
	Delete(ctx context.Context, id int64) error
}

// SiteSettingsRepository defines persistence operations for site settings.
type SiteSettingsRepository interface {
	Get(ctx context.Context, key string) (*model.SiteSetting, error)
	Set(ctx context.Context, key, value string) error
	GetAll(ctx context.Context) ([]model.SiteSetting, error)
}

// BanRepository defines persistence operations for email and IP bans.
type BanRepository interface {
	CreateEmailBan(ctx context.Context, ban *model.BannedEmail) error
	DeleteEmailBan(ctx context.Context, id int64) error
	ListEmailBans(ctx context.Context) ([]model.BannedEmail, error)
	IsEmailBanned(ctx context.Context, email string) (bool, error)

	CreateIPBan(ctx context.Context, ban *model.BannedIP) error
	DeleteIPBan(ctx context.Context, id int64) error
	ListIPBans(ctx context.Context) ([]model.BannedIP, error)
	IsIPBanned(ctx context.Context, ip string) (bool, error)
}

// CategoryRepository defines persistence operations for categories.
type CategoryRepository interface {
	GetByID(ctx context.Context, id int64) (*model.Category, error)
	List(ctx context.Context) ([]model.Category, error)
	Create(ctx context.Context, cat *model.Category) error
	Update(ctx context.Context, cat *model.Category) error
	Delete(ctx context.Context, id int64) error
	CountTorrentsByCategory(ctx context.Context, categoryID int64) (int64, error)
}

// MessageRepository defines persistence operations for private messages.
type MessageRepository interface {
	Create(ctx context.Context, msg *model.Message) error
	GetByID(ctx context.Context, id int64) (*model.Message, error)
	ListInbox(ctx context.Context, userID int64, page, perPage int) ([]model.Message, int64, error)
	ListOutbox(ctx context.Context, userID int64, page, perPage int) ([]model.Message, int64, error)
	MarkAsRead(ctx context.Context, id, userID int64) error
	DeleteForUser(ctx context.Context, id, userID int64) error
	CountUnread(ctx context.Context, userID int64) (int, error)
}

// SavedMessageRepository defines persistence operations for saved PM drafts
// and templates (see model.SavedMessage — kind discriminates the two).
type SavedMessageRepository interface {
	Create(ctx context.Context, sm *model.SavedMessage) error
	// Update performs an optimistic-concurrency update: sm.Version must hold
	// the version the caller last read, and the underlying conditional
	// UPDATE only matches a row whose stored version is still exactly that.
	// On success sm.Version and sm.UpdatedAt are updated in place to the new
	// values. If the row exists (owned by sm.UserID) but its stored version
	// has moved on, Update returns *SavedMessageConflictError instead of
	// silently overwriting it. If no such row exists at all (wrong id or
	// wrong owner), it returns sql.ErrNoRows, same as before this existed.
	Update(ctx context.Context, sm *model.SavedMessage) error
	GetByID(ctx context.Context, id int64) (*model.SavedMessage, error)
	ListByUser(ctx context.Context, userID int64, kind model.SavedMessageKind, page, perPage int) ([]model.SavedMessage, int64, error)
	Delete(ctx context.Context, id, userID int64) error
}

// SavedMessageConflictError is returned by SavedMessageRepository.Update when
// its conditional UPDATE (`... WHERE id = $1 AND user_id = $2 AND version =
// $3`) matches zero rows because the row's version has already moved past
// the one the caller last read — another save (a different browser tab, a
// different device) landed first. Current is that row as it stands right
// now, resolved by a follow-up lookup scoped exactly like the UPDATE itself,
// so a caller building a 409 response needs no extra round trip of its own.
type SavedMessageConflictError struct {
	Current *model.SavedMessage
}

func (e *SavedMessageConflictError) Error() string {
	return fmt.Sprintf("saved message %d: version conflict (current version is %d)", e.Current.ID, e.Current.Version)
}

// ChatMessageRepository defines persistence operations for chat messages.
type ChatMessageRepository interface {
	Create(ctx context.Context, msg *model.ChatMessage) error
	ListRecent(ctx context.Context, limit int) ([]model.ChatMessage, error)
	ListBefore(ctx context.Context, beforeID int64, limit int) ([]model.ChatMessage, error)
	Delete(ctx context.Context, id int64) error
	DeleteByUserID(ctx context.Context, userID int64) (int64, error)
}

// ChatMuteRepository defines persistence operations for chat mutes.
type ChatMuteRepository interface {
	Create(ctx context.Context, mute *model.ChatMute) error
	GetActiveMute(ctx context.Context, userID int64) (*model.ChatMute, error)
	ListActive(ctx context.Context, page, perPage int) ([]ChatMuteWithNames, int64, error)
	Delete(ctx context.Context, userID int64) error
	DeleteExpired(ctx context.Context) ([]int64, error)
}

// ChatMuteWithNames is a chat mute with resolved user and staff names.
type ChatMuteWithNames struct {
	model.ChatMute
	Username    string
	MutedByName *string
}

// WarningRepository defines persistence operations for user warnings.
type WarningRepository interface {
	Create(ctx context.Context, warning *model.Warning) error
	GetByID(ctx context.Context, id int64) (*model.Warning, error)
	ListByUser(ctx context.Context, userID int64, includeInactive bool) ([]model.Warning, error)
	ListAll(ctx context.Context, opts ListWarningsOptions) ([]model.Warning, int64, error)
	Update(ctx context.Context, warning *model.Warning) error
	CountActiveByUser(ctx context.Context, userID int64) (int, error)
	CountActiveManualByUser(ctx context.Context, userID int64) (int, error)
	GetActiveRatioWarning(ctx context.Context, userID int64) (*model.Warning, error)
	GetUsersWithLowRatio(ctx context.Context, threshold float64, minDownloaded int64) ([]model.User, error)
	ResolveExpiredManualWarnings(ctx context.Context) ([]int64, error)
}

// ListWarningsOptions holds filtering and pagination options for listing warnings.
type ListWarningsOptions struct {
	UserID  *int64
	Status  *string
	Search  string // search by username
	Page    int
	PerPage int
}

// ListReportsOptions holds filtering and pagination options for listing reports.
type ListReportsOptions struct {
	Status  *string // "pending", "resolved", or nil for all
	Page    int
	PerPage int
}

// NewsRepository defines persistence operations for news articles.
type NewsRepository interface {
	Create(ctx context.Context, article *model.NewsArticle) error
	GetByID(ctx context.Context, id int64) (*model.NewsArticle, error)
	GetPublishedByID(ctx context.Context, id int64) (*model.NewsArticle, error)
	Update(ctx context.Context, article *model.NewsArticle) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, opts ListNewsOptions) ([]model.NewsArticle, int64, error)
	ListPublished(ctx context.Context, page, perPage int) ([]model.NewsArticle, int64, error)
}

// ModNoteRepository defines persistence operations for staff mod notes.
type ModNoteRepository interface {
	Create(ctx context.Context, note *model.ModNote) error
	GetByID(ctx context.Context, id int64) (*model.ModNote, error)
	ListByUser(ctx context.Context, userID int64) ([]model.ModNote, error)
	Delete(ctx context.Context, id int64) error
}

// UserEditHistoryRepository defines persistence operations for the audit
// trail of admin edits to user profile fields.
type UserEditHistoryRepository interface {
	// Record appends one row per changed field. A no-op on an empty slice.
	Record(ctx context.Context, entries []model.UserEditHistory) error
	// ListByUser returns entries newest-first plus the total count.
	ListByUser(ctx context.Context, userID int64, limit, offset int) ([]model.UserEditHistory, int64, error)
}

// ListNewsOptions holds filtering and pagination options for listing news (admin).
type ListNewsOptions struct {
	Published *bool
	Page      int
	PerPage   int
}

// RestrictionRepository defines persistence operations for user privilege restrictions.
type RestrictionRepository interface {
	Create(ctx context.Context, r *model.Restriction) error
	GetByID(ctx context.Context, id int64) (*model.Restriction, error)
	ListByUser(ctx context.Context, userID int64) ([]model.Restriction, error)
	ListActive(ctx context.Context) ([]model.Restriction, error)
	Lift(ctx context.Context, id int64, liftedBy *int64) error
	LiftExpired(ctx context.Context) ([]model.Restriction, error)
	HasActiveByType(ctx context.Context, userID int64, restrictionType string) (bool, error)
}

// ForumCategoryRepository defines persistence operations for forum categories.
type ForumCategoryRepository interface {
	GetByID(ctx context.Context, id int64) (*model.ForumCategory, error)
	List(ctx context.Context) ([]model.ForumCategory, error)
	Create(ctx context.Context, cat *model.ForumCategory) error
	Update(ctx context.Context, cat *model.ForumCategory) error
	Delete(ctx context.Context, id int64) error
	CountForumsByCategory(ctx context.Context, categoryID int64) (int64, error)
}

// ForumRepository defines persistence operations for forums.
type ForumRepository interface {
	GetByID(ctx context.Context, id int64) (*model.Forum, error)
	ListByCategory(ctx context.Context, categoryID int64) ([]model.Forum, error)
	List(ctx context.Context) ([]model.Forum, error)
	Create(ctx context.Context, forum *model.Forum) error
	Update(ctx context.Context, forum *model.Forum) error
	Delete(ctx context.Context, id int64) error
	CountTopicsByForum(ctx context.Context, forumID int64) (int64, error)
	IncrementTopicCount(ctx context.Context, id int64, delta int) error
	IncrementPostCount(ctx context.Context, id int64, delta int) error
	UpdateLastPost(ctx context.Context, forumID int64, postID int64) error
	RecalculateLastPost(ctx context.Context, forumID int64) error
	RecalculateCounts(ctx context.Context, forumID int64) error
}

// ForumTopicRepository defines persistence operations for forum topics.
type ForumTopicRepository interface {
	GetByID(ctx context.Context, id int64) (*model.ForumTopic, error)
	ListByForum(ctx context.Context, forumID int64, page, perPage int) ([]model.ForumTopic, int64, error)
	Create(ctx context.Context, topic *model.ForumTopic) error
	IncrementViewCount(ctx context.Context, id int64) error
	IncrementPostCount(ctx context.Context, id int64, delta int) error
	UpdateLastPost(ctx context.Context, topicID int64, postID int64, postAt time.Time) error
	RecalculateLastPost(ctx context.Context, topicID int64) error
	SetLocked(ctx context.Context, id int64, locked bool) error
	SetPinned(ctx context.Context, id int64, pinned bool) error
	UpdateTitle(ctx context.Context, id int64, title string) error
	UpdateForumID(ctx context.Context, id int64, forumID int64) error
	Delete(ctx context.Context, id int64) error
}

// ErrEditConflict is returned by ForumPostRepository.Update when oldBody no
// longer matches the post's current body — a concurrent edit landed first
// (BE-9.25). It originates inside the conditional UPDATE (only the repo can
// observe the affected row count), so it lives here rather than in service,
// matching ErrInsufficientBonusPoints above.
var ErrEditConflict = errors.New("post body changed since it was read")

// ForumPostRepository defines persistence operations for forum posts.
type ForumPostRepository interface {
	GetByID(ctx context.Context, id int64) (*model.ForumPost, error)
	ListByTopic(ctx context.Context, topicID int64, page, perPage int) ([]model.ForumPost, int64, error)
	Create(ctx context.Context, post *model.ForumPost) error
	// Update conditionally applies post.Body/MentionedUsernames/EditedBy: the
	// write only lands if the row's current body still equals oldBody, the
	// exact text the caller's diff/comparison was computed against. If a
	// concurrent edit already changed it, no rows are touched and this
	// returns ErrEditConflict (BE-9.25) instead of silently overwriting a
	// diff that no longer applies.
	Update(ctx context.Context, post *model.ForumPost, oldBody string) error
	Delete(ctx context.Context, id int64) error
	CountByUser(ctx context.Context, userID int64) (int, error)
	Search(ctx context.Context, query string, forumID *int64, maxGroupLevel int, page, perPage int) ([]model.ForumSearchResult, int64, error)
	GetFirstPostIDByTopic(ctx context.Context, topicID int64) (int64, error)
	SoftDelete(ctx context.Context, id int64, deletedBy int64) error
	Restore(ctx context.Context, id int64) error
	CreateEdit(ctx context.Context, edit *model.ForumPostEdit) error
	ListEdits(ctx context.Context, postID int64) ([]model.ForumPostEdit, error)
}

// CheatFlagRepository defines persistence operations for cheat detection flags.
type CheatFlagRepository interface {
	Create(ctx context.Context, flag *model.CheatFlag) error
	GetByID(ctx context.Context, id int64) (*model.CheatFlag, error)
	List(ctx context.Context, opts ListCheatFlagsOptions) ([]model.CheatFlag, int64, error)
	Dismiss(ctx context.Context, id, dismissedBy int64) error
	HasRecentUndismissed(ctx context.Context, userID int64, torrentID int64, flagType string, cooldownHours int) (bool, error)
}

// ListCheatFlagsOptions holds filtering and pagination options for listing cheat flags.
type ListCheatFlagsOptions struct {
	UserID    *int64
	FlagType  *string
	Dismissed *bool
	Page      int
	PerPage   int
}

// NotificationRepository defines persistence operations for notifications.
type NotificationRepository interface {
	Create(ctx context.Context, notif *model.Notification) error
	GetByID(ctx context.Context, id int64) (*model.Notification, error)
	List(ctx context.Context, userID int64, opts ListNotificationsOptions) ([]model.Notification, int64, error)
	MarkRead(ctx context.Context, userID, id int64) error
	MarkAllRead(ctx context.Context, userID int64) error
	// MarkTopicReplyGroupRead marks every unread topic_reply notification for
	// userID and topicID as read in a single set-based UPDATE, returning the
	// number of rows affected. Backs the grouped-notifications (BE-9.14)
	// "mark all read" batch endpoint (BE-9.26) so a topic with many replies
	// doesn't need one MarkRead call per notification.
	MarkTopicReplyGroupRead(ctx context.Context, userID, topicID int64) (int64, error)
	CountUnread(ctx context.Context, userID int64) (int, error)
	DeleteOld(ctx context.Context, before time.Time) (int64, error)
	// CountUnreadSince returns how many unread notifications were created after
	// since. Used by the email digest job to size "N unread since your last digest".
	CountUnreadSince(ctx context.Context, userID int64, since time.Time) (int, error)
	// ListUnreadSince returns up to limit unread notifications created after
	// since, most recent first. Used to build the digest email body.
	ListUnreadSince(ctx context.Context, userID int64, since time.Time, limit int) ([]model.Notification, error)
}

// ListNotificationsOptions holds filtering and pagination options for listing notifications.
type ListNotificationsOptions struct {
	UnreadOnly bool
	Page       int
	PerPage    int
}

// TopicSubscriptionRepository defines persistence operations for topic subscriptions.
type TopicSubscriptionRepository interface {
	Subscribe(ctx context.Context, userID, topicID int64) error
	Unsubscribe(ctx context.Context, userID, topicID int64) error
	IsSubscribed(ctx context.Context, userID, topicID int64) (bool, error)
	ListSubscribers(ctx context.Context, topicID int64) ([]int64, error)
}

// NotificationPreferenceRepository defines persistence operations for notification preferences.
type NotificationPreferenceRepository interface {
	Get(ctx context.Context, userID int64, notifType string) (*model.NotificationPreference, error)
	GetAll(ctx context.Context, userID int64) ([]model.NotificationPreference, error)
	Set(ctx context.Context, userID int64, notifType string, enabled bool) error
	IsEnabled(ctx context.Context, userID int64, notifType string) (bool, error)
}

// NotificationDigestPreferenceRepository defines persistence operations for a
// user's email digest frequency and send cursor. Kept separate from
// NotificationPreferenceRepository — see migration 061 for why.
type NotificationDigestPreferenceRepository interface {
	// GetFrequency returns the user's digest frequency, defaulting to
	// model.DigestOff when no preference has been saved.
	GetFrequency(ctx context.Context, userID int64) (string, error)
	SetFrequency(ctx context.Context, userID int64, frequency string) error
	// ListDue returns every enabled user on frequency whose last digest (if
	// any) was sent before sentBefore — i.e. who is due for another run.
	ListDue(ctx context.Context, frequency string, sentBefore time.Time) ([]model.DigestRecipient, error)
	// MarkSent records that a digest run was processed for the user at sentAt,
	// advancing the "since" cursor regardless of whether an email was sent.
	MarkSent(ctx context.Context, userID int64, sentAt time.Time) error
}

// DashboardStats holds aggregated counts for the admin dashboard.
type DashboardStats struct {
	UsersTotal     int64
	UsersToday     int64
	UsersWeek      int64
	TorrentsTotal  int64
	TorrentsToday  int64
	PeersTotal     int64
	PeersSeeders   int64
	PeersLeechers  int64
	PendingReports int64
	ActiveWarnings int64
	ActiveMutes    int64
}

// DashboardRepository defines read operations for the admin dashboard.
type DashboardRepository interface {
	GetStats(ctx context.Context) (*DashboardStats, error)
}
