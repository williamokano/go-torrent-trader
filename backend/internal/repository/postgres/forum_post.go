package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// ForumPostRepo implements repository.ForumPostRepository using PostgreSQL.
type ForumPostRepo struct {
	db *sql.DB
}

// NewForumPostRepo returns a new PostgreSQL-backed ForumPostRepository.
func NewForumPostRepo(db *sql.DB) repository.ForumPostRepository {
	return &ForumPostRepo{db: db}
}

func (r *ForumPostRepo) GetByID(ctx context.Context, id int64) (*model.ForumPost, error) {
	query := `SELECT p.id, p.topic_id, p.user_id, p.body, p.reply_to_post_id,
		p.edited_at, p.edited_by, p.created_at,
		u.username, u.avatar, g.name, u.created_at,
		(SELECT COUNT(*) FROM forum_posts WHERE user_id = p.user_id)
	FROM forum_posts p
	JOIN users u ON u.id = p.user_id
	JOIN groups g ON g.id = u.group_id
	WHERE p.id = $1`

	var post model.ForumPost
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&post.ID, &post.TopicID, &post.UserID, &post.Body, &post.ReplyToPostID,
		&post.EditedAt, &post.EditedBy, &post.CreatedAt,
		&post.Username, &post.Avatar, &post.GroupName, &post.UserCreatedAt,
		&post.UserPostCount,
	)
	if err != nil {
		return nil, err
	}
	return &post, nil
}

func (r *ForumPostRepo) ListByTopic(ctx context.Context, topicID int64, page, perPage int) ([]model.ForumPost, int64, error) {
	var total int64
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM forum_posts WHERE topic_id = $1", topicID,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count posts: %w", err)
	}

	offset := (page - 1) * perPage
	query := `SELECT p.id, p.topic_id, p.user_id, p.body, p.reply_to_post_id,
		p.edited_at, p.edited_by, p.created_at,
		u.username, u.avatar, g.name, u.created_at,
		(SELECT COUNT(*) FROM forum_posts WHERE user_id = p.user_id)
	FROM forum_posts p
	JOIN users u ON u.id = p.user_id
	JOIN groups g ON g.id = u.group_id
	WHERE p.topic_id = $1
	ORDER BY p.created_at ASC
	LIMIT $2 OFFSET $3`

	rows, err := r.db.QueryContext(ctx, query, topicID, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list posts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var posts []model.ForumPost
	for rows.Next() {
		var post model.ForumPost
		if err := rows.Scan(
			&post.ID, &post.TopicID, &post.UserID, &post.Body, &post.ReplyToPostID,
			&post.EditedAt, &post.EditedBy, &post.CreatedAt,
			&post.Username, &post.Avatar, &post.GroupName, &post.UserCreatedAt,
			&post.UserPostCount,
		); err != nil {
			return nil, 0, fmt.Errorf("scan post: %w", err)
		}
		posts = append(posts, post)
	}
	return posts, total, rows.Err()
}

func (r *ForumPostRepo) Create(ctx context.Context, post *model.ForumPost) error {
	query := `INSERT INTO forum_posts (topic_id, user_id, body, reply_to_post_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`

	return r.db.QueryRowContext(ctx, query,
		post.TopicID, post.UserID, post.Body, post.ReplyToPostID,
	).Scan(&post.ID, &post.CreatedAt)
}

func (r *ForumPostRepo) CountByUser(ctx context.Context, userID int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM forum_posts WHERE user_id = $1", userID,
	).Scan(&count)
	return count, err
}
