package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// CategoryRepo implements repository.CategoryRepository using PostgreSQL.
type CategoryRepo struct {
	db *sql.DB
}

// NewCategoryRepo returns a new PostgreSQL-backed CategoryRepository.
func NewCategoryRepo(db *sql.DB) repository.CategoryRepository {
	return &CategoryRepo{db: db}
}

func (r *CategoryRepo) GetByID(ctx context.Context, id int64) (*model.Category, error) {
	query := `SELECT id, name, slug, parent_id, image_url, sort_order, created_at, updated_at
		FROM categories WHERE id = $1`

	var c model.Category
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&c.ID, &c.Name, &c.Slug, &c.ParentID, &c.ImageURL, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get category by id: %w", err)
	}
	return &c, nil
}

func (r *CategoryRepo) List(ctx context.Context) ([]model.Category, error) {
	query := `SELECT id, name, slug, parent_id, image_url, sort_order, created_at, updated_at
		FROM categories ORDER BY sort_order, name`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var categories []model.Category
	for rows.Next() {
		var c model.Category
		if err := rows.Scan(
			&c.ID, &c.Name, &c.Slug, &c.ParentID, &c.ImageURL, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		categories = append(categories, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate categories: %w", err)
	}

	return categories, nil
}

func (r *CategoryRepo) Create(ctx context.Context, cat *model.Category) error {
	query := `INSERT INTO categories (name, slug, parent_id, image_url, sort_order)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at`

	err := r.db.QueryRowContext(ctx, query, cat.Name, cat.Slug, cat.ParentID, cat.ImageURL, cat.SortOrder).
		Scan(&cat.ID, &cat.CreatedAt, &cat.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create category: %w", err)
	}
	return nil
}

func (r *CategoryRepo) Update(ctx context.Context, cat *model.Category) error {
	query := `UPDATE categories SET name = $1, slug = $2, parent_id = $3, image_url = $4, sort_order = $5, updated_at = NOW()
		WHERE id = $6
		RETURNING updated_at`

	err := r.db.QueryRowContext(ctx, query, cat.Name, cat.Slug, cat.ParentID, cat.ImageURL, cat.SortOrder, cat.ID).
		Scan(&cat.UpdatedAt)
	if err != nil {
		return fmt.Errorf("update category: %w", err)
	}
	return nil
}

func (r *CategoryRepo) Delete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM categories WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete category: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete category rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("category not found")
	}
	return nil
}

func (r *CategoryRepo) CountTorrentsByCategory(ctx context.Context, categoryID int64) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM torrents WHERE category_id = $1`, categoryID).
		Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count torrents by category: %w", err)
	}
	return count, nil
}
