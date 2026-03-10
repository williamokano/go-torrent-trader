package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// ModNoteRepo implements repository.ModNoteRepository using PostgreSQL.
type ModNoteRepo struct {
	db *sql.DB
}

// NewModNoteRepo returns a new PostgreSQL-backed ModNoteRepository.
func NewModNoteRepo(db *sql.DB) repository.ModNoteRepository {
	return &ModNoteRepo{db: db}
}

func (r *ModNoteRepo) Create(ctx context.Context, note *model.ModNote) error {
	query := `INSERT INTO mod_notes (user_id, author_id, note)
		VALUES ($1, $2, $3)
		RETURNING id, created_at`
	err := r.db.QueryRowContext(ctx, query, note.UserID, note.AuthorID, note.Note).
		Scan(&note.ID, &note.CreatedAt)
	if err != nil {
		return fmt.Errorf("create mod note: %w", err)
	}
	return nil
}

func (r *ModNoteRepo) GetByID(ctx context.Context, id int64) (*model.ModNote, error) {
	query := `SELECT mn.id, mn.user_id, mn.author_id, mn.note, mn.created_at,
		COALESCE(u.username, '') AS author_username
		FROM mod_notes mn
		LEFT JOIN users u ON u.id = mn.author_id
		WHERE mn.id = $1`
	var n model.ModNote
	err := r.db.QueryRowContext(ctx, query, id).Scan(&n.ID, &n.UserID, &n.AuthorID, &n.Note, &n.CreatedAt, &n.AuthorUsername)
	if err != nil {
		return nil, fmt.Errorf("get mod note: %w", err)
	}
	return &n, nil
}

func (r *ModNoteRepo) ListByUser(ctx context.Context, userID int64) ([]model.ModNote, error) {
	query := `SELECT mn.id, mn.user_id, mn.author_id, mn.note, mn.created_at,
		COALESCE(u.username, '') AS author_username
		FROM mod_notes mn
		LEFT JOIN users u ON u.id = mn.author_id
		WHERE mn.user_id = $1
		ORDER BY mn.created_at DESC`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list mod notes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var notes []model.ModNote
	for rows.Next() {
		var n model.ModNote
		if err := rows.Scan(&n.ID, &n.UserID, &n.AuthorID, &n.Note, &n.CreatedAt, &n.AuthorUsername); err != nil {
			return nil, fmt.Errorf("scan mod note: %w", err)
		}
		notes = append(notes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate mod notes: %w", err)
	}
	return notes, nil
}

func (r *ModNoteRepo) Delete(ctx context.Context, id int64) error {
	query := `DELETE FROM mod_notes WHERE id = $1`
	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete mod note: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete mod note rows affected: %w", err)
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}
