package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// EmailConfirmationRepo implements service.EmailConfirmationStore using PostgreSQL.
type EmailConfirmationRepo struct {
	db *sql.DB
}

// NewEmailConfirmationRepo returns a new PostgreSQL-backed EmailConfirmationStore.
func NewEmailConfirmationRepo(db *sql.DB) service.EmailConfirmationStore {
	return &EmailConfirmationRepo{db: db}
}

// Create inserts a new email confirmation record.
func (r *EmailConfirmationRepo) Create(ctx context.Context, userID int64, tokenHash []byte, expiresAt time.Time) error {
	query := `INSERT INTO email_confirmations (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)`
	_, err := r.db.ExecContext(ctx, query, userID, tokenHash, expiresAt)
	if err != nil {
		return fmt.Errorf("insert email confirmation: %w", err)
	}
	return nil
}

// ClaimByTokenHash atomically marks an unclaimed, non-expired token as confirmed and returns it.
// Returns nil, nil if not found, already confirmed, or expired.
func (r *EmailConfirmationRepo) ClaimByTokenHash(ctx context.Context, tokenHash []byte) (*service.EmailConfirmation, error) {
	query := `UPDATE email_confirmations
		SET confirmed_at = NOW()
		WHERE token_hash = $1 AND confirmed_at IS NULL AND expires_at > NOW()
		RETURNING id, user_id, token_hash, expires_at, confirmed_at, created_at`
	var ec service.EmailConfirmation
	err := r.db.QueryRowContext(ctx, query, tokenHash).Scan(
		&ec.ID, &ec.UserID, &ec.TokenHash, &ec.ExpiresAt, &ec.ConfirmedAt, &ec.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim email confirmation by token hash: %w", err)
	}
	return &ec, nil
}

// GetLatestByUserID returns the most recent confirmation for a user.
func (r *EmailConfirmationRepo) GetLatestByUserID(ctx context.Context, userID int64) (*service.EmailConfirmation, error) {
	query := `SELECT id, user_id, token_hash, expires_at, confirmed_at, created_at
		FROM email_confirmations
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT 1`
	var ec service.EmailConfirmation
	err := r.db.QueryRowContext(ctx, query, userID).Scan(
		&ec.ID, &ec.UserID, &ec.TokenHash, &ec.ExpiresAt, &ec.ConfirmedAt, &ec.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get latest email confirmation: %w", err)
	}
	return &ec, nil
}

// DeleteByUserID removes all confirmation tokens for a user.
func (r *EmailConfirmationRepo) DeleteByUserID(ctx context.Context, userID int64) error {
	query := `DELETE FROM email_confirmations WHERE user_id = $1`
	_, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("delete email confirmations by user: %w", err)
	}
	return nil
}
