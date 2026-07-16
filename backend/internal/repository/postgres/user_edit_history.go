package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// UserEditHistoryRepo implements repository.UserEditHistoryRepository using PostgreSQL.
type UserEditHistoryRepo struct {
	db *sql.DB
}

// NewUserEditHistoryRepo returns a new PostgreSQL-backed UserEditHistoryRepository.
func NewUserEditHistoryRepo(db *sql.DB) repository.UserEditHistoryRepository {
	return &UserEditHistoryRepo{db: db}
}

func (r *UserEditHistoryRepo) Record(ctx context.Context, entries []model.UserEditHistory) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("record user edit history: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO user_edit_history
		(user_id, changed_by, changed_by_username, field, old_value, new_value)
		VALUES ($1, $2, $3, $4, $5, $6)`)
	if err != nil {
		return fmt.Errorf("record user edit history: prepare: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for i := range entries {
		e := &entries[i]
		if _, err := stmt.ExecContext(ctx, e.UserID, e.ChangedBy, e.ChangedByUsername, e.Field, e.OldValue, e.NewValue); err != nil {
			return fmt.Errorf("record user edit history: insert %s: %w", e.Field, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("record user edit history: commit: %w", err)
	}
	return nil
}

func (r *UserEditHistoryRepo) ListByUser(ctx context.Context, userID int64, limit, offset int) ([]model.UserEditHistory, int64, error) {
	var total int64
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_edit_history WHERE user_id = $1`, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count user edit history: %w", err)
	}

	// Prefer the live username; fall back to the write-time snapshot when the
	// acting admin has since been deleted.
	query := `SELECT h.id, h.user_id, h.changed_by, h.field, h.old_value, h.new_value, h.created_at,
		COALESCE(u.username, h.changed_by_username) AS changed_by_username
		FROM user_edit_history h
		LEFT JOIN users u ON u.id = h.changed_by
		WHERE h.user_id = $1
		ORDER BY h.created_at DESC, h.id DESC
		LIMIT $2 OFFSET $3`
	rows, err := r.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list user edit history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var entries []model.UserEditHistory
	for rows.Next() {
		var e model.UserEditHistory
		if err := rows.Scan(&e.ID, &e.UserID, &e.ChangedBy, &e.Field, &e.OldValue, &e.NewValue, &e.CreatedAt, &e.ChangedByUsername); err != nil {
			return nil, 0, fmt.Errorf("scan user edit history: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate user edit history: %w", err)
	}
	return entries, total, nil
}
