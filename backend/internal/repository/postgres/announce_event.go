package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// AnnounceEventRepo implements repository.AnnounceEventRepository using PostgreSQL.
type AnnounceEventRepo struct {
	db *sql.DB
}

// NewAnnounceEventRepo returns a new PostgreSQL-backed AnnounceEventRepository.
func NewAnnounceEventRepo(db *sql.DB) repository.AnnounceEventRepository {
	return &AnnounceEventRepo{db: db}
}

func (r *AnnounceEventRepo) Create(ctx context.Context, e *model.AnnounceEvent) error {
	query := `INSERT INTO announce_events (
		user_id, torrent_id, peer_id, ip, port, event,
		uploaded, downloaded, left_bytes,
		uploaded_delta, downloaded_delta, counted_downloaded_delta,
		seeder, announced_at
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	RETURNING id`
	return r.db.QueryRowContext(ctx, query,
		e.UserID, e.TorrentID, e.PeerID, e.IP, e.Port, e.Event,
		e.Uploaded, e.Downloaded, e.LeftBytes,
		e.UploadedDelta, e.DownloadedDelta, e.CountedDownloadedDelta,
		e.Seeder, e.AnnouncedAt,
	).Scan(&e.ID)
}

func (r *AnnounceEventRepo) ListByUser(ctx context.Context, userID int64, page, perPage int) ([]repository.AnnounceEventWithTorrent, int64, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}
	offset := (page - 1) * perPage

	var total int64
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM announce_events WHERE user_id = $1`, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting announce events: %w", err)
	}

	query := `SELECT ae.id, ae.user_id, ae.torrent_id, ae.peer_id, ae.ip, ae.port, ae.event,
		ae.uploaded, ae.downloaded, ae.left_bytes,
		ae.uploaded_delta, ae.downloaded_delta, ae.counted_downloaded_delta,
		ae.seeder, ae.announced_at,
		COALESCE(t.name, 'Deleted Torrent') AS torrent_name
		FROM announce_events ae
		LEFT JOIN torrents t ON ae.torrent_id = t.id
		WHERE ae.user_id = $1
		ORDER BY ae.announced_at DESC, ae.id DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.db.QueryContext(ctx, query, userID, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing announce events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []repository.AnnounceEventWithTorrent
	for rows.Next() {
		var item repository.AnnounceEventWithTorrent
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.TorrentID, &item.PeerID, &item.IP, &item.Port, &item.Event,
			&item.Uploaded, &item.Downloaded, &item.LeftBytes,
			&item.UploadedDelta, &item.DownloadedDelta, &item.CountedDownloadedDelta,
			&item.Seeder, &item.AnnouncedAt, &item.TorrentName,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning announce event: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating announce events: %w", err)
	}

	return results, total, nil
}
