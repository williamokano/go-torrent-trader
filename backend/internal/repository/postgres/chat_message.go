package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// chatMessageColumns is shared by the two list queries so their scan order can
// never drift apart.
//
// System messages have a NULL user_id (there is no system user row), so the JOIN
// has to be a LEFT JOIN and the username has to be synthesised. Reading user_id
// straight into an int64 would fail on those rows, hence the NullInt64 hop in
// scanChatMessages.
const chatMessageColumns = `cm.id, cm.user_id,
	CASE WHEN cm.system THEN 'System' ELSE COALESCE(u.username, 'Unknown') END AS username,
	cm.message, cm.system, cm.created_at`

const chatMessageJoins = `FROM chat_messages cm LEFT JOIN users u ON u.id = cm.user_id`

// ChatMessageRepo implements repository.ChatMessageRepository using PostgreSQL.
type ChatMessageRepo struct {
	db *sql.DB
}

// NewChatMessageRepo returns a new PostgreSQL-backed ChatMessageRepository.
func NewChatMessageRepo(db *sql.DB) repository.ChatMessageRepository {
	return &ChatMessageRepo{db: db}
}

func (r *ChatMessageRepo) Create(ctx context.Context, msg *model.ChatMessage) error {
	// A system message has no author, so user_id must go in as SQL NULL rather
	// than 0 — there is no user with id 0 and the FK would reject it.
	var userID any
	if !msg.System {
		userID = msg.UserID
	}

	query := `INSERT INTO chat_messages (user_id, message, system)
		VALUES ($1, $2, $3)
		RETURNING id, created_at`

	return r.db.QueryRowContext(ctx, query, userID, msg.Message, msg.System).
		Scan(&msg.ID, &msg.CreatedAt)
}

func (r *ChatMessageRepo) ListRecent(ctx context.Context, limit int) ([]model.ChatMessage, error) {
	query := fmt.Sprintf(`SELECT %s %s ORDER BY cm.created_at DESC LIMIT $1`,
		chatMessageColumns, chatMessageJoins)

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent chat messages: %w", err)
	}
	return scanChatMessages(rows)
}

func (r *ChatMessageRepo) ListBefore(ctx context.Context, beforeID int64, limit int) ([]model.ChatMessage, error) {
	query := fmt.Sprintf(`SELECT %s %s WHERE cm.id < $1 ORDER BY cm.created_at DESC LIMIT $2`,
		chatMessageColumns, chatMessageJoins)

	rows, err := r.db.QueryContext(ctx, query, beforeID, limit)
	if err != nil {
		return nil, fmt.Errorf("list chat messages before: %w", err)
	}
	return scanChatMessages(rows)
}

// scanChatMessages drains rows and reverses them into chronological (oldest
// first) order, which is how the UI renders — both queries select DESC so the
// LIMIT takes the newest page.
func scanChatMessages(rows *sql.Rows) ([]model.ChatMessage, error) {
	defer func() { _ = rows.Close() }()

	var msgs []model.ChatMessage
	for rows.Next() {
		var (
			m      model.ChatMessage
			userID sql.NullInt64
		)
		if err := rows.Scan(&m.ID, &userID, &m.Username, &m.Message, &m.System, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan chat message: %w", err)
		}
		m.UserID = userID.Int64 // NULL (system message) reads back as 0
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chat messages: %w", err)
	}

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
