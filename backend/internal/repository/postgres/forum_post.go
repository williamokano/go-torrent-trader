package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

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

func (r *ForumPostRepo) Update(ctx context.Context, post *model.ForumPost) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE forum_posts SET body = $1, edited_at = NOW(), edited_by = $2 WHERE id = $3",
		post.Body, post.EditedBy, post.ID,
	)
	return err
}

func (r *ForumPostRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM forum_posts WHERE id = $1", id)
	return err
}

func (r *ForumPostRepo) CountByUser(ctx context.Context, userID int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM forum_posts WHERE user_id = $1", userID,
	).Scan(&count)
	return count, err
}

func (r *ForumPostRepo) Search(ctx context.Context, query string, forumID *int64, maxGroupLevel int, page, perPage int) ([]model.ForumSearchResult, int64, error) {
	tsQuery := BuildPrefixQuery(query)
	if tsQuery == "" {
		return nil, 0, nil
	}

	// Build conditions for the WHERE clause.
	conditions := []string{
		"(p.search_vector @@ to_tsquery('english', $1) OR (t.search_vector @@ to_tsquery('english', $1) AND p.id = (SELECT MIN(fp.id) FROM forum_posts fp WHERE fp.topic_id = t.id)))",
		fmt.Sprintf("f.min_group_level <= $%d", 2),
	}
	args := []any{tsQuery, maxGroupLevel}
	argIdx := 2

	if forumID != nil {
		argIdx++
		conditions = append(conditions, fmt.Sprintf("t.forum_id = $%d", argIdx))
		args = append(args, *forumID)
	}

	whereClause := strings.Join(conditions, " AND ")

	// Count total matching results.
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM forum_posts p
		JOIN forum_topics t ON t.id = p.topic_id
		JOIN forums f ON f.id = t.forum_id
		WHERE %s`, whereClause)

	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count forum search results: %w", err)
	}

	// Fetch paginated results ordered by relevance.
	offset := (page - 1) * perPage
	argIdx++
	args = append(args, perPage)
	limitArg := argIdx
	argIdx++
	args = append(args, offset)
	offsetArg := argIdx

	dataQuery := fmt.Sprintf(`SELECT p.id, p.body, t.id, t.title, f.id, f.name, p.user_id, u.username, p.created_at,
		ts_headline('english', p.body, to_tsquery('english', $1), 'MaxWords=30,MinWords=15,StartSel=<mark>,StopSel=</mark>') AS snippet
		FROM forum_posts p
		JOIN forum_topics t ON t.id = p.topic_id
		JOIN forums f ON f.id = t.forum_id
		JOIN users u ON u.id = p.user_id
		WHERE %s
		ORDER BY GREATEST(ts_rank(p.search_vector, to_tsquery('english', $1)), ts_rank(t.search_vector, to_tsquery('english', $1))) DESC, p.created_at DESC
		LIMIT $%d OFFSET $%d`, whereClause, limitArg, offsetArg)

	rows, err := r.db.QueryContext(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("search forum posts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []model.ForumSearchResult
	for rows.Next() {
		var sr model.ForumSearchResult
		if err := rows.Scan(
			&sr.PostID, &sr.Body, &sr.TopicID, &sr.TopicTitle,
			&sr.ForumID, &sr.ForumName, &sr.UserID, &sr.Username, &sr.CreatedAt,
			&sr.Snippet,
		); err != nil {
			return nil, 0, fmt.Errorf("scan forum search result: %w", err)
		}
		results = append(results, sr)
	}
	return results, total, rows.Err()
}

func (r *ForumPostRepo) GetFirstPostIDByTopic(ctx context.Context, topicID int64) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx,
		"SELECT id FROM forum_posts WHERE topic_id = $1 ORDER BY id ASC LIMIT 1", topicID,
	).Scan(&id)
	return id, err
}
