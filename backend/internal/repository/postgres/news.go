package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// NewsRepo implements repository.NewsRepository using PostgreSQL.
type NewsRepo struct {
	db *sql.DB
}

// NewNewsRepo returns a new PostgreSQL-backed NewsRepository.
func NewNewsRepo(db *sql.DB) repository.NewsRepository {
	return &NewsRepo{db: db}
}

func (r *NewsRepo) Create(ctx context.Context, a *model.NewsArticle) error {
	query := `INSERT INTO news (title, body, author_id, published)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at`
	err := r.db.QueryRowContext(ctx, query,
		a.Title, a.Body, a.AuthorID, a.Published,
	).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create news article: %w", err)
	}
	return nil
}

func (r *NewsRepo) GetByID(ctx context.Context, id int64) (*model.NewsArticle, error) {
	query := `SELECT n.id, n.title, n.body, n.author_id, n.published, n.created_at, n.updated_at,
		u.username AS author_name
		FROM news n
		LEFT JOIN users u ON u.id = n.author_id
		WHERE n.id = $1`
	return scanNewsArticle(r.db.QueryRowContext(ctx, query, id))
}

func (r *NewsRepo) Update(ctx context.Context, a *model.NewsArticle) error {
	query := `UPDATE news SET title = $1, body = $2, published = $3, updated_at = NOW()
		WHERE id = $4`
	res, err := r.db.ExecContext(ctx, query, a.Title, a.Body, a.Published, a.ID)
	if err != nil {
		return fmt.Errorf("update news article: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update news rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *NewsRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM news WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete news article: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete news rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *NewsRepo) List(ctx context.Context, opts repository.ListNewsOptions) ([]model.NewsArticle, int64, error) {
	page := opts.Page
	if page <= 0 {
		page = 1
	}
	perPage := opts.PerPage
	if perPage <= 0 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}

	where := "WHERE 1=1"
	args := []interface{}{}
	argIdx := 1

	if opts.Published != nil {
		where += fmt.Sprintf(" AND n.published = $%d", argIdx)
		args = append(args, *opts.Published)
		argIdx++
	}

	// Count
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM news n %s`, where)
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count news: %w", err)
	}

	// Fetch
	offset := (page - 1) * perPage
	query := fmt.Sprintf(`SELECT n.id, n.title, n.body, n.author_id, n.published, n.created_at, n.updated_at,
		u.username AS author_name
		FROM news n
		LEFT JOIN users u ON u.id = n.author_id
		%s ORDER BY n.created_at DESC LIMIT $%d OFFSET $%d`,
		where, argIdx, argIdx+1)
	args = append(args, perPage, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list news: %w", err)
	}
	defer func() { _ = rows.Close() }()

	articles, err := scanNewsArticles(rows)
	if err != nil {
		return nil, 0, err
	}

	return articles, total, nil
}

func (r *NewsRepo) ListPublished(ctx context.Context, page, perPage int) ([]model.NewsArticle, int64, error) {
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM news WHERE published = true`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count published news: %w", err)
	}

	offset := (page - 1) * perPage
	query := `SELECT n.id, n.title, n.body, n.author_id, n.published, n.created_at, n.updated_at,
		u.username AS author_name
		FROM news n
		LEFT JOIN users u ON u.id = n.author_id
		WHERE n.published = true
		ORDER BY n.created_at DESC LIMIT $1 OFFSET $2`

	rows, err := r.db.QueryContext(ctx, query, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list published news: %w", err)
	}
	defer func() { _ = rows.Close() }()

	articles, err := scanNewsArticles(rows)
	if err != nil {
		return nil, 0, err
	}

	return articles, total, nil
}

func scanNewsArticle(row interface{ Scan(...any) error }) (*model.NewsArticle, error) {
	var a model.NewsArticle
	err := row.Scan(
		&a.ID, &a.Title, &a.Body, &a.AuthorID, &a.Published,
		&a.CreatedAt, &a.UpdatedAt, &a.AuthorName,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func scanNewsArticles(rows *sql.Rows) ([]model.NewsArticle, error) {
	var articles []model.NewsArticle
	for rows.Next() {
		var a model.NewsArticle
		if err := rows.Scan(
			&a.ID, &a.Title, &a.Body, &a.AuthorID, &a.Published,
			&a.CreatedAt, &a.UpdatedAt, &a.AuthorName,
		); err != nil {
			return nil, fmt.Errorf("scan news article: %w", err)
		}
		articles = append(articles, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate news articles: %w", err)
	}
	return articles, nil
}

// Ensure compile-time interface satisfaction.
var _ repository.NewsRepository = (*NewsRepo)(nil)
