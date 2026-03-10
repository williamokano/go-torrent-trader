package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// ForumCategoryRepo implements repository.ForumCategoryRepository using PostgreSQL.
type ForumCategoryRepo struct {
	db *sql.DB
}

// NewForumCategoryRepo returns a new PostgreSQL-backed ForumCategoryRepository.
func NewForumCategoryRepo(db *sql.DB) repository.ForumCategoryRepository {
	return &ForumCategoryRepo{db: db}
}

func (r *ForumCategoryRepo) List(ctx context.Context) ([]model.ForumCategory, error) {
	query := `SELECT id, name, sort_order, created_at FROM forum_categories ORDER BY sort_order, id`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list forum categories: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var categories []model.ForumCategory
	for rows.Next() {
		var c model.ForumCategory
		if err := rows.Scan(&c.ID, &c.Name, &c.SortOrder, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan forum category: %w", err)
		}
		categories = append(categories, c)
	}
	return categories, rows.Err()
}
