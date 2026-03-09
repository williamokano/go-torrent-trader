package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// TransferHistoryRepo implements repository.TransferHistoryRepository using PostgreSQL.
type TransferHistoryRepo struct {
	db *sql.DB
}

// NewTransferHistoryRepo returns a new PostgreSQL-backed TransferHistoryRepository.
func NewTransferHistoryRepo(db *sql.DB) repository.TransferHistoryRepository {
	return &TransferHistoryRepo{db: db}
}

func (r *TransferHistoryRepo) Upsert(ctx context.Context, th *model.TransferHistory) error {
	query := `INSERT INTO transfer_history (user_id, torrent_id, uploaded, downloaded, seeder, completed_at, last_announce)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id, torrent_id) DO UPDATE SET
			uploaded = EXCLUDED.uploaded,
			downloaded = EXCLUDED.downloaded,
			seeder = EXCLUDED.seeder,
			last_announce = EXCLUDED.last_announce
		RETURNING id`
	return r.db.QueryRowContext(ctx, query,
		th.UserID, th.TorrentID, th.Uploaded, th.Downloaded,
		th.Seeder, th.CompletedAt, th.LastAnnounce,
	).Scan(&th.ID)
}

func (r *TransferHistoryRepo) ListByUser(ctx context.Context, userID int64, page, perPage int) ([]repository.TransferHistoryWithTorrent, int64, error) {
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
	countQuery := `SELECT COUNT(*) FROM transfer_history WHERE user_id = $1`
	if err := r.db.QueryRowContext(ctx, countQuery, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting transfer history: %w", err)
	}

	query := `SELECT th.id, th.user_id, th.torrent_id, th.uploaded, th.downloaded,
		th.seeder, th.completed_at, th.last_announce,
		COALESCE(t.name, 'Deleted Torrent') AS torrent_name
		FROM transfer_history th
		LEFT JOIN torrents t ON th.torrent_id = t.id
		WHERE th.user_id = $1
		ORDER BY th.completed_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.db.QueryContext(ctx, query, userID, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing transfer history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []repository.TransferHistoryWithTorrent
	for rows.Next() {
		var item repository.TransferHistoryWithTorrent
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.TorrentID, &item.Uploaded, &item.Downloaded,
			&item.Seeder, &item.CompletedAt, &item.LastAnnounce, &item.TorrentName,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning transfer history: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating transfer history: %w", err)
	}

	return results, total, nil
}
