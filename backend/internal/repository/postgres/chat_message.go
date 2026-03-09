package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// ChatMessageRepo implements repository.ChatMessageRepository using PostgreSQL.
type ChatMessageRepo struct {
	db *sql.DB
}

// NewChatMessageRepo returns a new PostgreSQL-backed ChatMessageRepository.
func NewChatMessageRepo(db *sql.DB) repository.ChatMessageRepository {
	return &ChatMessageRepo{db: db}
}

func (r *ChatMessageRepo) Create(ctx context.Context, msg *model.ChatMessage) error {
	query := `INSERT INTO chat_messages (user_id, message)
		VALUES ($1, $2)
		RETURNING id, created_at`

	return r.db.QueryRowContext(ctx, query, msg.UserID, msg.Message).Scan(&msg.ID, &msg.CreatedAt)
}

func (r *ChatMessageRepo) ListRecent(ctx context.Context, limit int) ([]model.ChatMessage, error) {
	query := `SELECT cm.id, cm.user_id, u.username, cm.message, cm.created_at
		FROM chat_messages cm
		JOIN users u ON u.id = cm.user_id
		ORDER BY cm.created_at DESC
		LIMIT $1`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent chat messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var msgs []model.ChatMessage
	for rows.Next() {
		var m model.ChatMessage
		if err := rows.Scan(&m.ID, &m.UserID, &m.Username, &m.Message, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan chat message: %w", err)
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chat messages: %w", err)
	}

	// Reverse for chronological order (oldest first).
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}

	return msgs, nil
}

func (r *ChatMessageRepo) ListBefore(ctx context.Context, beforeID int64, limit int) ([]model.ChatMessage, error) {
	query := `SELECT cm.id, cm.user_id, u.username, cm.message, cm.created_at
		FROM chat_messages cm
		JOIN users u ON u.id = cm.user_id
		WHERE cm.id < $1
		ORDER BY cm.created_at DESC
		LIMIT $2`

	rows, err := r.db.QueryContext(ctx, query, beforeID, limit)
	if err != nil {
		return nil, fmt.Errorf("list chat messages before: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var msgs []model.ChatMessage
	for rows.Next() {
		var m model.ChatMessage
		if err := rows.Scan(&m.ID, &m.UserID, &m.Username, &m.Message, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan chat message: %w", err)
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chat messages: %w", err)
	}

	// Reverse for chronological order (oldest first).
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}

	return msgs, nil
}

func (r *ChatMessageRepo) DeleteByUserID(ctx context.Context, userID int64) (int64, error) {
	result, err := r.db.ExecContext(ctx, "DELETE FROM chat_messages WHERE user_id = $1", userID)
	if err != nil {
		return 0, fmt.Errorf("delete chat messages by user: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("checking rows affected: %w", err)
	}
	return n, nil
}

func (r *ChatMessageRepo) Delete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM chat_messages WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete chat message: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
