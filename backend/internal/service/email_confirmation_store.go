package service

import (
	"context"
	"time"
)

// EmailConfirmation represents an email confirmation token record.
type EmailConfirmation struct {
	ID          int64
	UserID      int64
	TokenHash   []byte
	ExpiresAt   time.Time
	ConfirmedAt *time.Time
	CreatedAt   time.Time
}

// EmailConfirmationStore defines the interface for email confirmation token persistence.
type EmailConfirmationStore interface {
	// Create inserts a new email confirmation record.
	Create(ctx context.Context, userID int64, tokenHash []byte, expiresAt time.Time) error
	// ClaimByTokenHash atomically finds an unexpired, unclaimed token by hash,
	// sets confirmed_at, and returns the confirmation. Returns nil, nil if not found/expired/already claimed.
	ClaimByTokenHash(ctx context.Context, tokenHash []byte) (*EmailConfirmation, error)
	// GetLatestByUserID returns the most recent confirmation for a user (for resend rate limiting).
	GetLatestByUserID(ctx context.Context, userID int64) (*EmailConfirmation, error)
	// DeleteByUserID removes all confirmation tokens for a user (cleanup before resend).
	DeleteByUserID(ctx context.Context, userID int64) error
}
