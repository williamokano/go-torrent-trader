package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

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

	// Past the end there is nothing to fetch, and this is the largest table on the
	// site: `?page=9999999` would otherwise make Postgres walk a billion index
	// entries to discard every one of them. The count is already in hand, so the
	// check is free.
	if int64(offset) >= total {
		return nil, total, nil
	}

	query := announceEventSelect + `
		WHERE ae.user_id = $1
		ORDER BY ae.announced_at DESC, ae.id DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.db.QueryContext(ctx, query, userID, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing announce events: %w", err)
	}

	results, err := scanAnnounceEvents(rows)
	if err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

func (r *AnnounceEventRepo) PageByUser(ctx context.Context, userID int64, afterID int64, limit int) ([]repository.AnnounceEventWithTorrent, error) {
	if limit < 1 {
		limit = 1000
	}
	if limit > 5000 {
		limit = 5000
	}

	// Ascending by id, which is also chronological: the ids come from a sequence
	// and AnnouncedAt is stamped at insert. Keyset on the primary key rather than
	// on announced_at, because two announces can share a timestamp and a keyset
	// over a non-unique column either repeats rows or skips them.
	query := announceEventSelect + `
		WHERE ae.user_id = $1 AND ae.id > $2
		ORDER BY ae.id
		LIMIT $3`

	rows, err := r.db.QueryContext(ctx, query, userID, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("paging announce events: %w", err)
	}
	return scanAnnounceEvents(rows)
}

func (r *AnnounceEventRepo) DeleteOlderThan(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	if limit < 1 {
		return 0, nil
	}

	// The subselect is what bounds the work: it walks idx_announce_events_announced_at
	// in order and stops at limit, so each statement deletes a predictable slice
	// regardless of how far behind retention has fallen.
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM announce_events WHERE id IN (
			SELECT id FROM announce_events WHERE announced_at < $1 ORDER BY announced_at LIMIT $2
		)`, cutoff, limit)
	if err != nil {
		return 0, fmt.Errorf("deleting old announce events: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("checking rows affected: %w", err)
	}
	return n, nil
}

// announceEventSelect is the shared projection for the listing and export paths.
// The torrent name is a LEFT JOIN because torrent_id is set to NULL when a torrent
// is deleted, and the log outliving the torrent is the point of an append-only log.
const announceEventSelect = `SELECT ae.id, ae.user_id, ae.torrent_id, ae.peer_id, ae.ip, ae.port, ae.event,
		ae.uploaded, ae.downloaded, ae.left_bytes,
		ae.uploaded_delta, ae.downloaded_delta, ae.counted_downloaded_delta,
		ae.seeder, ae.announced_at,
		COALESCE(t.name, 'Deleted Torrent') AS torrent_name
		FROM announce_events ae
		LEFT JOIN torrents t ON ae.torrent_id = t.id`

func scanAnnounceEvents(rows *sql.Rows) ([]repository.AnnounceEventWithTorrent, error) {
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
			return nil, fmt.Errorf("scanning announce event: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating announce events: %w", err)
	}
	return results, nil
}
