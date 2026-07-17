package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// CommentRepo implements repository.CommentRepository using PostgreSQL.
type CommentRepo struct {
	db *sql.DB
}

// NewCommentRepo returns a new PostgreSQL-backed CommentRepository.
func NewCommentRepo(db *sql.DB) repository.CommentRepository {
	return &CommentRepo{db: db}
}

func (r *CommentRepo) Create(ctx context.Context, comment *model.Comment) error {
	mentioned, err := MarshalMentionedUsernames(comment.MentionedUsernames)
	if err != nil {
		return fmt.Errorf("marshal mentioned usernames: %w", err)
	}

	query := `INSERT INTO torrent_comments (torrent_id, user_id, body, mentioned_usernames)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at`

	return r.db.QueryRowContext(ctx, query,
		comment.TorrentID, comment.UserID, comment.Body, mentioned,
	).Scan(&comment.ID, &comment.CreatedAt, &comment.UpdatedAt)
}

func (r *CommentRepo) GetByID(ctx context.Context, id int64) (*model.Comment, error) {
	query := `SELECT tc.id, tc.torrent_id, tc.user_id, u.username, tc.body, tc.mentioned_usernames, tc.created_at, tc.updated_at
		FROM torrent_comments tc
		JOIN users u ON u.id = tc.user_id
		WHERE tc.id = $1`

	var c model.Comment
	var mentioned []byte
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&c.ID, &c.TorrentID, &c.UserID, &c.Username, &c.Body, &mentioned, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	c.MentionedUsernames = ScanMentionedUsernames(mentioned)
	return &c, nil
}

func (r *CommentRepo) ListByTorrent(ctx context.Context, torrentID int64, page, perPage int) ([]model.Comment, int64, error) {
	// Count total
	var total int64
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM torrent_comments WHERE torrent_id = $1", torrentID,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count comments: %w", err)
	}

	offset := (page - 1) * perPage
	query := `SELECT tc.id, tc.torrent_id, tc.user_id, u.username, tc.body, tc.mentioned_usernames, tc.created_at, tc.updated_at
		FROM torrent_comments tc
		JOIN users u ON u.id = tc.user_id
		WHERE tc.torrent_id = $1
		ORDER BY tc.created_at ASC
		LIMIT $2 OFFSET $3`

	rows, err := r.db.QueryContext(ctx, query, torrentID, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list comments: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var comments []model.Comment
	for rows.Next() {
		var c model.Comment
		var mentioned []byte
		if err := rows.Scan(&c.ID, &c.TorrentID, &c.UserID, &c.Username, &c.Body, &mentioned, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan comment: %w", err)
		}
		c.MentionedUsernames = ScanMentionedUsernames(mentioned)
		comments = append(comments, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate comments: %w", err)
	}

	return comments, total, nil
}

func (r *CommentRepo) Update(ctx context.Context, comment *model.Comment) error {
	mentioned, err := MarshalMentionedUsernames(comment.MentionedUsernames)
	if err != nil {
		return fmt.Errorf("marshal mentioned usernames: %w", err)
	}

	query := `UPDATE torrent_comments SET body = $1, mentioned_usernames = $2, updated_at = NOW()
		WHERE id = $3
		RETURNING updated_at`

	return r.db.QueryRowContext(ctx, query, comment.Body, mentioned, comment.ID).Scan(&comment.UpdatedAt)
}

func (r *CommentRepo) Delete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM torrent_comments WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete comment: %w", err)
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
