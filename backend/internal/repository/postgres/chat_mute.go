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
