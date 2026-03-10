package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// ChatMuteRepo implements repository.ChatMuteRepository using PostgreSQL.
type ChatMuteRepo struct {
	db *sql.DB
}

// NewChatMuteRepo returns a new PostgreSQL-backed ChatMuteRepository.
func NewChatMuteRepo(db *sql.DB) repository.ChatMuteRepository {
	return &ChatMuteRepo{db: db}
}

func (r *ChatMuteRepo) Create(ctx context.Context, mute *model.ChatMute) error {
	query := `INSERT INTO chat_mutes (user_id, muted_by, reason, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`

	return r.db.QueryRowContext(ctx, query, mute.UserID, mute.MutedBy, mute.Reason, mute.ExpiresAt).
		Scan(&mute.ID, &mute.CreatedAt)
}

func (r *ChatMuteRepo) GetActiveMute(ctx context.Context, userID int64) (*model.ChatMute, error) {
	query := `SELECT id, user_id, muted_by, reason, expires_at, created_at
		FROM chat_mutes
		WHERE user_id = $1 AND expires_at > NOW()
		ORDER BY expires_at DESC
		LIMIT 1`

	var m model.ChatMute
	err := r.db.QueryRowContext(ctx, query, userID).Scan(
		&m.ID, &m.UserID, &m.MutedBy, &m.Reason, &m.ExpiresAt, &m.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get active chat mute: %w", err)
	}
	return &m, nil
}

func (r *ChatMuteRepo) ListActive(ctx context.Context, page, perPage int) ([]repository.ChatMuteWithNames, int64, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 25
	}
	offset := (page - 1) * perPage

	var total int64
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM chat_mutes WHERE expires_at > NOW()").Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count active chat mutes: %w", err)
	}

	query := `SELECT cm.id, cm.user_id, cm.muted_by, cm.reason, cm.expires_at, cm.created_at,
			u.username,
			mb.username AS muted_by_name
		FROM chat_mutes cm
		JOIN users u ON u.id = cm.user_id
		LEFT JOIN users mb ON mb.id = cm.muted_by
		WHERE cm.expires_at > NOW()
		ORDER BY cm.expires_at DESC
		LIMIT $1 OFFSET $2`

	rows, err := r.db.QueryContext(ctx, query, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list active chat mutes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var mutes []repository.ChatMuteWithNames
	for rows.Next() {
		var m repository.ChatMuteWithNames
		if err := rows.Scan(
			&m.ID, &m.UserID, &m.MutedBy, &m.Reason, &m.ExpiresAt, &m.CreatedAt,
			&m.Username, &m.MutedByName,
		); err != nil {
			return nil, 0, fmt.Errorf("scan active chat mute: %w", err)
		}
		mutes = append(mutes, m)
	}
	return mutes, total, rows.Err()
}

func (r *ChatMuteRepo) Delete(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM chat_mutes WHERE user_id = $1", userID)
	if err != nil {
		return fmt.Errorf("delete chat mutes: %w", err)
	}
	return nil
}

func (r *ChatMuteRepo) DeleteExpired(ctx context.Context) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, "DELETE FROM chat_mutes WHERE expires_at <= NOW() RETURNING user_id")
	if err != nil {
		return nil, fmt.Errorf("delete expired chat mutes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	seen := make(map[int64]bool)
	var userIDs []int64
	for rows.Next() {
		var uid int64
		if err := rows.Scan(&uid); err != nil {
			return nil, fmt.Errorf("scan expired mute user_id: %w", err)
		}
		if !seen[uid] {
			seen[uid] = true
			userIDs = append(userIDs, uid)
		}
	}
	return userIDs, rows.Err()
}
