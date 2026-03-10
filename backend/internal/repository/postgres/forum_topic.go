package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// ForumTopicRepo implements repository.ForumTopicRepository using PostgreSQL.
type ForumTopicRepo struct {
	db *sql.DB
}

// NewForumTopicRepo returns a new PostgreSQL-backed ForumTopicRepository.
func NewForumTopicRepo(db *sql.DB) repository.ForumTopicRepository {
	return &ForumTopicRepo{db: db}
}

func (r *ForumTopicRepo) GetByID(ctx context.Context, id int64) (*model.ForumTopic, error) {
	query := `SELECT t.id, t.forum_id, t.user_id, t.title, t.pinned, t.locked,
		t.post_count, t.view_count, t.last_post_id, t.last_post_at, t.created_at, t.updated_at,
		u.username, lu.username, f.name
	FROM forum_topics t
	JOIN users u ON u.id = t.user_id
	LEFT JOIN forum_posts lp ON lp.id = t.last_post_id
	LEFT JOIN users lu ON lu.id = lp.user_id
	JOIN forums f ON f.id = t.forum_id
	WHERE t.id = $1`

	var t model.ForumTopic
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&t.ID, &t.ForumID, &t.UserID, &t.Title, &t.Pinned, &t.Locked,
		&t.PostCount, &t.ViewCount, &t.LastPostID, &t.LastPostAt, &t.CreatedAt, &t.UpdatedAt,
		&t.Username, &t.LastPostUsername, &t.ForumName,
	)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *ForumTopicRepo) ListByForum(ctx context.Context, forumID int64, page, perPage int) ([]model.ForumTopic, int64, error) {
	var total int64
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM forum_topics WHERE forum_id = $1", forumID,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count topics: %w", err)
	}

	offset := (page - 1) * perPage
	query := `SELECT t.id, t.forum_id, t.user_id, t.title, t.pinned, t.locked,
		t.post_count, t.view_count, t.last_post_id, t.last_post_at, t.created_at, t.updated_at,
		u.username, lu.username, f.name
	FROM forum_topics t
	JOIN users u ON u.id = t.user_id
	LEFT JOIN forum_posts lp ON lp.id = t.last_post_id
	LEFT JOIN users lu ON lu.id = lp.user_id
	JOIN forums f ON f.id = t.forum_id
	WHERE t.forum_id = $1
	ORDER BY t.pinned DESC, t.last_post_at DESC NULLS LAST, t.created_at DESC
	LIMIT $2 OFFSET $3`

	rows, err := r.db.QueryContext(ctx, query, forumID, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list topics: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var topics []model.ForumTopic
	for rows.Next() {
		var t model.ForumTopic
		if err := rows.Scan(
			&t.ID, &t.ForumID, &t.UserID, &t.Title, &t.Pinned, &t.Locked,
			&t.PostCount, &t.ViewCount, &t.LastPostID, &t.LastPostAt, &t.CreatedAt, &t.UpdatedAt,
			&t.Username, &t.LastPostUsername, &t.ForumName,
		); err != nil {
			return nil, 0, fmt.Errorf("scan topic: %w", err)
		}
		topics = append(topics, t)
	}
	return topics, total, rows.Err()
}

func (r *ForumTopicRepo) Create(ctx context.Context, topic *model.ForumTopic) error {
	query := `INSERT INTO forum_topics (forum_id, user_id, title)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at`

	return r.db.QueryRowContext(ctx, query,
		topic.ForumID, topic.UserID, topic.Title,
	).Scan(&topic.ID, &topic.CreatedAt, &topic.UpdatedAt)
}

func (r *ForumTopicRepo) IncrementViewCount(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "UPDATE forum_topics SET view_count = view_count + 1 WHERE id = $1", id)
	return err
}

func (r *ForumTopicRepo) IncrementPostCount(ctx context.Context, id int64, delta int) error {
	_, err := r.db.ExecContext(ctx, "UPDATE forum_topics SET post_count = post_count + $1 WHERE id = $2", delta, id)
	return err
}

func (r *ForumTopicRepo) UpdateLastPost(ctx context.Context, topicID int64, postID int64, postAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE forum_topics SET last_post_id = $1, last_post_at = $2, updated_at = NOW() WHERE id = $3",
		postID, postAt, topicID,
	)
	return err
}

func (r *ForumTopicRepo) RecalculateLastPost(ctx context.Context, topicID int64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE forum_topics SET
			last_post_id = (SELECT id FROM forum_posts WHERE topic_id = $1 ORDER BY created_at DESC LIMIT 1),
			last_post_at = (SELECT created_at FROM forum_posts WHERE topic_id = $1 ORDER BY created_at DESC LIMIT 1),
			updated_at = NOW()
		WHERE id = $1`, topicID)
	return err
}
