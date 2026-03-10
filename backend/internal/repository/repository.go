package repository

import (
	"context"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// UserRepository defines persistence operations for users.
type UserRepository interface {
	GetByID(ctx context.Context, id int64) (*model.User, error)
	GetByUsername(ctx context.Context, username string) (*model.User, error)
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
	Search    string
	GroupID   *int64
	Enabled   *bool
	SortBy    string // username, created_at, uploaded, downloaded
	SortOrder string // asc, desc
	Page      int
	PerPage   int
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

// ListTorrentsOptions holds filtering and pagination options for listing torrents.
type ListTorrentsOptions struct {
	CategoryID       *int64
	Search           string
	SortBy           string // name, created_at, size, seeders, leechers
	SortOrder        string // asc, desc
	Page             int
	PerPage          int
	CreatedAfter     *time.Time // for "today's torrents"
	MaxSeeders       *int       // for "need seed" (seeders <= N)
	UploaderID       *int64     // for "my uploads" or user's torrents
	ExcludeAnonymous bool       // when true, filter out anonymous torrents (for non-owner/non-staff viewers)
}

// GroupRepository defines persistence operations for groups.
type GroupRepository interface {
	GetByID(ctx context.Context, id int64) (*model.Group, error)
	List(ctx context.Context) ([]model.Group, error)
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
	Page      int
	PerPage   int
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
	GetByToken(ctx context.Context, token string) (*model.Invite, error)
	ListByInviter(ctx context.Context, inviterID int64, page, perPage int) ([]model.Invite, int64, error)
	Redeem(ctx context.Context, token string, inviteeID int64) error
	CountPendingByInviter(ctx context.Context, inviterID int64) (int, error)
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

// ListNewsOptions holds filtering and pagination options for listing news (admin).
type ListNewsOptions struct {
	Published *bool
	Page      int
	PerPage   int
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
