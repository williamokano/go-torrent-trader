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
}

// TorrentRepository defines persistence operations for torrents.
type TorrentRepository interface {
	GetByID(ctx context.Context, id int64) (*model.Torrent, error)
	GetByInfoHash(ctx context.Context, infoHash []byte) (*model.Torrent, error)
	List(ctx context.Context, opts ListTorrentsOptions) ([]model.Torrent, int64, error)
	Create(ctx context.Context, torrent *model.Torrent) error
	Update(ctx context.Context, torrent *model.Torrent) error
	IncrementSeeders(ctx context.Context, id int64, delta int) error
	IncrementLeechers(ctx context.Context, id int64, delta int) error
}

// PeerRepository defines persistence operations for peers.
type PeerRepository interface {
	GetByTorrentAndUser(ctx context.Context, torrentID, userID int64) (*model.Peer, error)
	ListByTorrent(ctx context.Context, torrentID int64, limit int) ([]model.Peer, error)
	Upsert(ctx context.Context, peer *model.Peer) error
	Delete(ctx context.Context, torrentID, userID int64, peerID []byte) error
	DeleteStale(ctx context.Context, before time.Time) (int64, error)
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
