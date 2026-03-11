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

func (r *ForumCategoryRepo) GetByID(ctx context.Context, id int64) (*model.ForumCategory, error) {
	var c model.ForumCategory
	err := r.db.QueryRowContext(ctx, `SELECT id, name, sort_order, created_at FROM forum_categories WHERE id = $1`, id).
		Scan(&c.ID, &c.Name, &c.SortOrder, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
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

func (r *ForumCategoryRepo) Create(ctx context.Context, cat *model.ForumCategory) error {
	return r.db.QueryRowContext(ctx,
		`INSERT INTO forum_categories (name, sort_order) VALUES ($1, $2) RETURNING id, created_at`,
		cat.Name, cat.SortOrder,
	).Scan(&cat.ID, &cat.CreatedAt)
}

func (r *ForumCategoryRepo) Update(ctx context.Context, cat *model.ForumCategory) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE forum_categories SET name = $1, sort_order = $2 WHERE id = $3`,
		cat.Name, cat.SortOrder, cat.ID,
	)
	return err
}

func (r *ForumCategoryRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM forum_categories WHERE id = $1`, id)
	return err
}

func (r *ForumCategoryRepo) CountForumsByCategory(ctx context.Context, categoryID int64) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM forums WHERE category_id = $1`, categoryID).Scan(&count)
	return count, err
}
