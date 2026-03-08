package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// MessageRepo implements repository.MessageRepository using PostgreSQL.
type MessageRepo struct {
	db *sql.DB
}

// NewMessageRepo returns a new PostgreSQL-backed MessageRepository.
func NewMessageRepo(db *sql.DB) repository.MessageRepository {
	return &MessageRepo{db: db}
}

func (r *MessageRepo) Create(ctx context.Context, msg *model.Message) error {
	query := `INSERT INTO messages (sender_id, receiver_id, subject, body)
		VALUES ($1, $2, $3, $4)
		RETURNING id, is_read, sender_deleted, receiver_deleted, created_at`

	return r.db.QueryRowContext(ctx, query,
		msg.SenderID, msg.ReceiverID, msg.Subject, msg.Body,
	).Scan(&msg.ID, &msg.IsRead, &msg.SenderDeleted, &msg.ReceiverDeleted, &msg.CreatedAt)
}

func (r *MessageRepo) GetByID(ctx context.Context, id int64) (*model.Message, error) {
	query := `SELECT m.id, m.sender_id, su.username, m.receiver_id, ru.username,
			m.subject, m.body, m.is_read, m.sender_deleted, m.receiver_deleted, m.created_at
		FROM messages m
		JOIN users su ON su.id = m.sender_id
		JOIN users ru ON ru.id = m.receiver_id
		WHERE m.id = $1`

	var msg model.Message
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&msg.ID, &msg.SenderID, &msg.SenderUsername, &msg.ReceiverID, &msg.ReceiverUsername,
		&msg.Subject, &msg.Body, &msg.IsRead, &msg.SenderDeleted, &msg.ReceiverDeleted, &msg.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func (r *MessageRepo) ListInbox(ctx context.Context, userID int64, page, perPage int) ([]model.Message, int64, error) {
	var total int64
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM messages WHERE receiver_id = $1 AND receiver_deleted = false", userID,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count inbox: %w", err)
	}

	offset := (page - 1) * perPage
	query := `SELECT m.id, m.sender_id, su.username, m.receiver_id, ru.username,
			m.subject, m.body, m.is_read, m.sender_deleted, m.receiver_deleted, m.created_at
		FROM messages m
		JOIN users su ON su.id = m.sender_id
		JOIN users ru ON ru.id = m.receiver_id
		WHERE m.receiver_id = $1 AND m.receiver_deleted = false
		ORDER BY m.created_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.db.QueryContext(ctx, query, userID, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list inbox: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var messages []model.Message
	for rows.Next() {
		var msg model.Message
		if err := rows.Scan(
			&msg.ID, &msg.SenderID, &msg.SenderUsername, &msg.ReceiverID, &msg.ReceiverUsername,
			&msg.Subject, &msg.Body, &msg.IsRead, &msg.SenderDeleted, &msg.ReceiverDeleted, &msg.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan message: %w", err)
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate messages: %w", err)
	}

	return messages, total, nil
}

func (r *MessageRepo) ListOutbox(ctx context.Context, userID int64, page, perPage int) ([]model.Message, int64, error) {
	var total int64
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM messages WHERE sender_id = $1 AND sender_deleted = false", userID,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count outbox: %w", err)
	}

	offset := (page - 1) * perPage
	query := `SELECT m.id, m.sender_id, su.username, m.receiver_id, ru.username,
			m.subject, m.body, m.is_read, m.sender_deleted, m.receiver_deleted, m.created_at
		FROM messages m
		JOIN users su ON su.id = m.sender_id
		JOIN users ru ON ru.id = m.receiver_id
		WHERE m.sender_id = $1 AND m.sender_deleted = false
		ORDER BY m.created_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.db.QueryContext(ctx, query, userID, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list outbox: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var messages []model.Message
	for rows.Next() {
		var msg model.Message
		if err := rows.Scan(
			&msg.ID, &msg.SenderID, &msg.SenderUsername, &msg.ReceiverID, &msg.ReceiverUsername,
			&msg.Subject, &msg.Body, &msg.IsRead, &msg.SenderDeleted, &msg.ReceiverDeleted, &msg.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan message: %w", err)
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate messages: %w", err)
	}

	return messages, total, nil
}

func (r *MessageRepo) MarkAsRead(ctx context.Context, id, userID int64) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE messages SET is_read = true WHERE id = $1 AND receiver_id = $2",
		id, userID,
	)
	return err
}

func (r *MessageRepo) DeleteForUser(ctx context.Context, id, userID int64) error {
	// First check if user is sender or receiver
	var senderID, receiverID int64
	err := r.db.QueryRowContext(ctx,
		"SELECT sender_id, receiver_id FROM messages WHERE id = $1", id,
	).Scan(&senderID, &receiverID)
	if err != nil {
		return err
	}

	if userID == senderID {
		_, err = r.db.ExecContext(ctx,
			"UPDATE messages SET sender_deleted = true WHERE id = $1", id,
		)
		return err
	}
	if userID == receiverID {
		_, err = r.db.ExecContext(ctx,
			"UPDATE messages SET receiver_deleted = true WHERE id = $1", id,
		)
		return err
	}

	return sql.ErrNoRows
}

func (r *MessageRepo) CountUnread(ctx context.Context, userID int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM messages WHERE receiver_id = $1 AND is_read = false AND receiver_deleted = false",
		userID,
	).Scan(&count)
	return count, err
}
