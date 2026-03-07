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
}

// TorrentRepository defines persistence operations for torrents.
type TorrentRepository interface {
	GetByID(ctx context.Context, id int64) (*model.Torrent, error)
	GetByInfoHash(ctx context.Context, infoHash []byte) (*model.Torrent, error)
	List(ctx context.Context, opts ListTorrentsOptions) ([]model.Torrent, int64, error)
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
	ListByTorrent(ctx context.Context, torrentID int64, limit int) ([]model.Peer, error)
	Upsert(ctx context.Context, peer *model.Peer) error
	Delete(ctx context.Context, torrentID, userID int64, peerID []byte) error
	DeleteStale(ctx context.Context, before time.Time) (int64, error)
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
	CategoryID *int64
	Search     string
	SortBy     string // name, created_at, size, seeders, leechers
	SortOrder  string // asc, desc
	Page       int
	PerPage    int
}

// ListReportsOptions holds filtering and pagination options for listing reports.
type ListReportsOptions struct {
	Status  *string // "pending", "resolved", or nil for all
	Page    int
	PerPage int
}
