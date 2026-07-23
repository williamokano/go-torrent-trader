package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// TorrentModerationMessageRepo implements
// repository.TorrentModerationMessageRepository using PostgreSQL.
type TorrentModerationMessageRepo struct {
	db DBTX
}

// NewTorrentModerationMessageRepo returns a new PostgreSQL-backed repo.
func NewTorrentModerationMessageRepo(db *sql.DB) *TorrentModerationMessageRepo {
	return &TorrentModerationMessageRepo{db: db}
}

func (r *TorrentModerationMessageRepo) Create(ctx context.Context, msg *model.TorrentModerationMessage) error {
	query := `INSERT INTO torrent_moderation_messages (torrent_id, author_id, body)
		VALUES ($1, $2, $3) RETURNING id, created_at`
	return r.db.QueryRowContext(ctx, query, msg.TorrentID, msg.AuthorID, msg.Body).
		Scan(&msg.ID, &msg.CreatedAt)
}

func (r *TorrentModerationMessageRepo) ListByTorrent(ctx context.Context, torrentID int64) ([]model.TorrentModerationMessage, error) {
	query := `SELECT m.id, m.torrent_id, m.author_id, m.body, m.created_at, COALESCE(u.username, 'Unknown')
		FROM torrent_moderation_messages m
		LEFT JOIN users u ON m.author_id = u.id
		WHERE m.torrent_id = $1
		ORDER BY m.created_at ASC`
	rows, err := r.db.QueryContext(ctx, query, torrentID)
	if err != nil {
		return nil, fmt.Errorf("listing moderation messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var msgs []model.TorrentModerationMessage
	for rows.Next() {
		var m model.TorrentModerationMessage
		if err := rows.Scan(&m.ID, &m.TorrentID, &m.AuthorID, &m.Body, &m.CreatedAt, &m.AuthorUsername); err != nil {
			return nil, fmt.Errorf("scanning moderation message: %w", err)
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating moderation messages: %w", err)
	}
	return msgs, nil
}

func (r *TorrentModerationMessageRepo) CountByTorrent(ctx context.Context, torrentID int64) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM torrent_moderation_messages WHERE torrent_id = $1`, torrentID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("counting moderation messages: %w", err)
	}
	return n, nil
}
