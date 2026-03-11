package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// ForumRepo implements repository.ForumRepository using PostgreSQL.
type ForumRepo struct {
	db *sql.DB
}

// NewForumRepo returns a new PostgreSQL-backed ForumRepository.
func NewForumRepo(db *sql.DB) repository.ForumRepository {
	return &ForumRepo{db: db}
}

func (r *ForumRepo) GetByID(ctx context.Context, id int64) (*model.Forum, error) {
	query := `SELECT f.id, f.category_id, f.name, f.description, f.sort_order,
		f.topic_count, f.post_count, f.last_post_id, f.min_group_level, f.min_post_level, f.created_at,
		p.created_at, u.username, t.id, t.title
	FROM forums f
	LEFT JOIN forum_posts p ON p.id = f.last_post_id
	LEFT JOIN users u ON u.id = p.user_id
	LEFT JOIN forum_topics t ON t.id = p.topic_id
	WHERE f.id = $1`

	var f model.Forum
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&f.ID, &f.CategoryID, &f.Name, &f.Description, &f.SortOrder,
		&f.TopicCount, &f.PostCount, &f.LastPostID, &f.MinGroupLevel, &f.MinPostLevel, &f.CreatedAt,
		&f.LastPostAt, &f.LastPostUsername, &f.LastPostTopicID, &f.LastPostTopicTitle,
	)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (r *ForumRepo) ListByCategory(ctx context.Context, categoryID int64) ([]model.Forum, error) {
	return r.listForums(ctx, "WHERE f.category_id = $1 ORDER BY f.sort_order, f.id", categoryID)
}

func (r *ForumRepo) List(ctx context.Context) ([]model.Forum, error) {
	return r.listForums(ctx, "ORDER BY f.sort_order, f.id")
}

func (r *ForumRepo) listForums(ctx context.Context, whereClause string, args ...interface{}) ([]model.Forum, error) {
	query := fmt.Sprintf(`SELECT f.id, f.category_id, f.name, f.description, f.sort_order,
		f.topic_count, f.post_count, f.last_post_id, f.min_group_level, f.min_post_level, f.created_at,
		p.created_at, u.username, t.id, t.title
	FROM forums f
	LEFT JOIN forum_posts p ON p.id = f.last_post_id
	LEFT JOIN users u ON u.id = p.user_id
	LEFT JOIN forum_topics t ON t.id = p.topic_id
	%s`, whereClause)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list forums: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var forums []model.Forum
	for rows.Next() {
		var f model.Forum
		if err := rows.Scan(
			&f.ID, &f.CategoryID, &f.Name, &f.Description, &f.SortOrder,
			&f.TopicCount, &f.PostCount, &f.LastPostID, &f.MinGroupLevel, &f.MinPostLevel, &f.CreatedAt,
			&f.LastPostAt, &f.LastPostUsername, &f.LastPostTopicID, &f.LastPostTopicTitle,
		); err != nil {
			return nil, fmt.Errorf("scan forum: %w", err)
		}
		forums = append(forums, f)
	}
	return forums, rows.Err()
}

func (r *ForumRepo) IncrementTopicCount(ctx context.Context, id int64, delta int) error {
	_, err := r.db.ExecContext(ctx, "UPDATE forums SET topic_count = GREATEST(topic_count + $1, 0) WHERE id = $2", delta, id)
	return err
}

func (r *ForumRepo) IncrementPostCount(ctx context.Context, id int64, delta int) error {
	_, err := r.db.ExecContext(ctx, "UPDATE forums SET post_count = GREATEST(post_count + $1, 0) WHERE id = $2", delta, id)
	return err
}

func (r *ForumRepo) UpdateLastPost(ctx context.Context, forumID int64, postID int64) error {
	_, err := r.db.ExecContext(ctx, "UPDATE forums SET last_post_id = $1 WHERE id = $2", postID, forumID)
	return err
}

func (r *ForumRepo) RecalculateLastPost(ctx context.Context, forumID int64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE forums SET last_post_id = sub.id
		FROM (
			SELECT p.id FROM forum_posts p
			JOIN forum_topics t ON t.id = p.topic_id
			WHERE t.forum_id = $1
			ORDER BY p.created_at DESC LIMIT 1
		) sub
		WHERE forums.id = $1`, forumID)
	if err != nil {
		return err
	}
	// If no posts remain, set last_post_id to NULL
	_, err = r.db.ExecContext(ctx, `
		UPDATE forums SET last_post_id = NULL
		WHERE id = $1
			AND NOT EXISTS (
				SELECT 1 FROM forum_posts p
				JOIN forum_topics t ON t.id = p.topic_id
				WHERE t.forum_id = $1
			)`, forumID)
	return err
}

func (r *ForumRepo) RecalculateCounts(ctx context.Context, forumID int64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE forums SET
			topic_count = COALESCE((SELECT COUNT(*) FROM forum_topics WHERE forum_id = $1), 0),
			post_count = COALESCE((SELECT COUNT(*) FROM forum_posts fp JOIN forum_topics ft ON ft.id = fp.topic_id WHERE ft.forum_id = $1), 0),
			last_post_id = (SELECT fp.id FROM forum_posts fp JOIN forum_topics ft ON ft.id = fp.topic_id WHERE ft.forum_id = $1 ORDER BY fp.created_at DESC LIMIT 1)
		WHERE id = $1`, forumID)
	return err
}
