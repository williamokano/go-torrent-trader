package postgres

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// PasswordResetRepo implements service.PasswordResetStore using PostgreSQL.
type PasswordResetRepo struct {
	db *sql.DB
}

// NewPasswordResetRepo returns a new PostgreSQL-backed PasswordResetStore.
func NewPasswordResetRepo(db *sql.DB) service.PasswordResetStore {
	return &PasswordResetRepo{db: db}
}

// Create inserts a new password reset record.
func (r *PasswordResetRepo) Create(pr *service.PasswordReset) error {
	query := `INSERT INTO password_resets (user_id, token_hash, expires_at, used, created_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`
	err := r.db.QueryRow(query, pr.UserID, pr.TokenHash, pr.ExpiresAt, pr.Used, pr.CreatedAt).Scan(&pr.ID)
	if err != nil {
		return fmt.Errorf("insert password reset: %w", err)
	}
	return nil
}

// ClaimByTokenHash atomically marks an unused, non-expired token as used and returns it.
// Returns nil, nil if not found, already used, or expired.
func (r *PasswordResetRepo) ClaimByTokenHash(tokenHash string) (*service.PasswordReset, error) {
	query := `UPDATE password_resets
		SET used = true
		WHERE token_hash = $1 AND used = false AND expires_at > NOW()
		RETURNING id, user_id, token_hash, expires_at, used, created_at`
	var pr service.PasswordReset
	err := r.db.QueryRow(query, tokenHash).Scan(
		&pr.ID, &pr.UserID, &pr.TokenHash, &pr.ExpiresAt, &pr.Used, &pr.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim password reset by token hash: %w", err)
	}
	return &pr, nil
}

// CountRecentByUserID counts password reset tokens for a user created within the given duration.
func (r *PasswordResetRepo) CountRecentByUserID(userID int64, within time.Duration) (int, error) {
	query := `SELECT COUNT(*) FROM password_resets
		WHERE user_id = $1 AND created_at > NOW() - $2::interval`
	var count int
	err := r.db.QueryRow(query, userID, within.String()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count recent password resets: %w", err)
	}
	return count, nil
}
