package service

import (
	"time"
)

// PasswordReset represents a password reset token record.
type PasswordReset struct {
	ID        int64
	UserID    int64
	TokenHash string // SHA-256 hash of the raw token
	ExpiresAt time.Time
	Used      bool
	CreatedAt time.Time
}

// PasswordResetStore defines the interface for password reset token persistence.
type PasswordResetStore interface {
	Create(pr *PasswordReset) error
	// ClaimByTokenHash atomically finds an unused, non-expired token by hash,
	// marks it as used, and returns it. Returns nil, nil if not found/expired/used.
	// This prevents TOCTOU race conditions on token redemption.
	ClaimByTokenHash(tokenHash string) (*PasswordReset, error)
	CountRecentByUserID(userID int64, within time.Duration) (int, error)
}
