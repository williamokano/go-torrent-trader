package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// SavedMessageRepo implements repository.SavedMessageRepository using PostgreSQL.
type SavedMessageRepo struct {
	db *sql.DB
}

// NewSavedMessageRepo returns a new PostgreSQL-backed SavedMessageRepository.
func NewSavedMessageRepo(db *sql.DB) repository.SavedMessageRepository {
	return &SavedMessageRepo{db: db}
}

func (r *SavedMessageRepo) Create(ctx context.Context, sm *model.SavedMessage) error {
	query := `INSERT INTO saved_messages (user_id, kind, receiver_id, subject, body, parent_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at, version`

	return r.db.QueryRowContext(ctx, query,
		sm.UserID, sm.Kind, sm.ReceiverID, sm.Subject, sm.Body, sm.ParentID,
	).Scan(&sm.ID, &sm.CreatedAt, &sm.UpdatedAt, &sm.Version)
}

// Update is an optimistic-concurrency update — see the doc comment on
// repository.SavedMessageRepository.Update for the contract. It's scoped by
// user_id, not just id, as defense-in-depth: the service layer already
// checks ownership before calling Update, but this way a future caller that
// skips that check still can't overwrite someone else's row. Mirrors
// MessageRepo.MarkAsRead/DeleteForUser (message.go), which bake the same
// constraint into their SQL rather than relying solely on the service.
func (r *SavedMessageRepo) Update(ctx context.Context, sm *model.SavedMessage) error {
	query := `UPDATE saved_messages
		SET receiver_id = $1, subject = $2, body = $3, parent_id = $4, version = version + 1, updated_at = NOW()
		WHERE id = $5 AND user_id = $6 AND version = $7
		RETURNING updated_at, version`

	err := r.db.QueryRowContext(ctx, query,
		sm.ReceiverID, sm.Subject, sm.Body, sm.ParentID, sm.ID, sm.UserID, sm.Version,
	).Scan(&sm.UpdatedAt, &sm.Version)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	// The conditional UPDATE matched no row. That's either because no row
	// with this id is owned by this user (a real "not found" — wrong id, or
	// someone else's row), or because such a row exists but its version has
	// moved on since sm.Version was read (a conflict). A follow-up lookup,
	// scoped exactly like the UPDATE's own WHERE clause minus the version
	// check, tells the two apart: Postgres serializes concurrent UPDATEs
	// against the same row (the second waits for the first's row lock, then
	// re-evaluates its WHERE against the now-committed row), so there's no
	// window here where this lookup could see a row the UPDATE itself didn't.
	current, lookupErr := r.getByIDForUser(ctx, sm.ID, sm.UserID)
	if lookupErr != nil {
		if errors.Is(lookupErr, sql.ErrNoRows) {
			return sql.ErrNoRows
		}
		// A genuine infrastructure failure (connection drop, deadline, etc.)
		// on the disambiguation lookup itself must not be swallowed into a
		// false "not found" — that would silently turn a 500-class failure
		// into a misleading 404.
		return fmt.Errorf("disambiguate update conflict: %w", lookupErr)
	}
	return &repository.SavedMessageConflictError{Current: current}
}

const savedMessageColumns = `sm.id, sm.user_id, sm.kind, sm.receiver_id, COALESCE(ru.username, ''), sm.subject, sm.body, sm.parent_id, sm.created_at, sm.updated_at, sm.version`

func scanSavedMessage(row interface{ Scan(dest ...any) error }) (*model.SavedMessage, error) {
	var sm model.SavedMessage
	err := row.Scan(
		&sm.ID, &sm.UserID, &sm.Kind, &sm.ReceiverID, &sm.ReceiverUsername,
		&sm.Subject, &sm.Body, &sm.ParentID, &sm.CreatedAt, &sm.UpdatedAt, &sm.Version,
	)
	if err != nil {
		return nil, err
	}
	return &sm, nil
}

func (r *SavedMessageRepo) GetByID(ctx context.Context, id int64) (*model.SavedMessage, error) {
	query := `SELECT ` + savedMessageColumns + `
		FROM saved_messages sm
		LEFT JOIN users ru ON ru.id = sm.receiver_id
		WHERE sm.id = $1`

	return scanSavedMessage(r.db.QueryRowContext(ctx, query, id))
}

// getByIDForUser is GetByID additionally scoped to user_id, used by Update to
// disambiguate a "no row owned by this user" 404 from a version conflict
// (see the comment in Update).
func (r *SavedMessageRepo) getByIDForUser(ctx context.Context, id, userID int64) (*model.SavedMessage, error) {
	query := `SELECT ` + savedMessageColumns + `
		FROM saved_messages sm
		LEFT JOIN users ru ON ru.id = sm.receiver_id
		WHERE sm.id = $1 AND sm.user_id = $2`

	return scanSavedMessage(r.db.QueryRowContext(ctx, query, id, userID))
}

func (r *SavedMessageRepo) ListByUser(ctx context.Context, userID int64, kind model.SavedMessageKind, page, perPage int) ([]model.SavedMessage, int64, error) {
	var total int64
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM saved_messages WHERE user_id = $1 AND kind = $2", userID, kind,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count saved messages: %w", err)
	}

	offset := (page - 1) * perPage
	query := `SELECT ` + savedMessageColumns + `
		FROM saved_messages sm
		LEFT JOIN users ru ON ru.id = sm.receiver_id
		WHERE sm.user_id = $1 AND sm.kind = $2
		ORDER BY sm.updated_at DESC
		LIMIT $3 OFFSET $4`

	rows, err := r.db.QueryContext(ctx, query, userID, kind, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list saved messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []model.SavedMessage
	for rows.Next() {
		var sm model.SavedMessage
		if err := rows.Scan(
			&sm.ID, &sm.UserID, &sm.Kind, &sm.ReceiverID, &sm.ReceiverUsername,
			&sm.Subject, &sm.Body, &sm.ParentID, &sm.CreatedAt, &sm.UpdatedAt, &sm.Version,
		); err != nil {
			return nil, 0, fmt.Errorf("scan saved message: %w", err)
		}
		items = append(items, sm)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate saved messages: %w", err)
	}

	return items, total, nil
}

// Delete removes a saved message, scoped to userID as defense-in-depth (see
// the comment on Update) — the service layer already verifies ownership
// first, but a caller that skipped that check still couldn't delete someone
// else's draft/template through this method.
func (r *SavedMessageRepo) Delete(ctx context.Context, id, userID int64) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM saved_messages WHERE id = $1 AND user_id = $2", id, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
