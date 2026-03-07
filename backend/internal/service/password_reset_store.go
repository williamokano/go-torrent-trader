package service

import (
	"sync"
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

// MemoryPasswordResetStore provides in-memory storage for password reset tokens.
// Suitable for tests; use a database-backed implementation in production.
type MemoryPasswordResetStore struct {
	mu     sync.Mutex
	resets []*PasswordReset
	nextID int64
}

// NewMemoryPasswordResetStore creates a new in-memory password reset store.
func NewMemoryPasswordResetStore() *MemoryPasswordResetStore {
	return &MemoryPasswordResetStore{nextID: 1}
}

// Create stores a new password reset record.
func (s *MemoryPasswordResetStore) Create(pr *PasswordReset) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	pr.ID = s.nextID
	s.nextID++
	s.resets = append(s.resets, pr)
	return nil
}

// ClaimByTokenHash atomically finds and marks a token as used.
func (s *MemoryPasswordResetStore) ClaimByTokenHash(tokenHash string) (*PasswordReset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for _, pr := range s.resets {
		if pr.TokenHash == tokenHash && !pr.Used && pr.ExpiresAt.After(now) {
			pr.Used = true
			return pr, nil
		}
	}
	return nil, nil
}

// CountRecentByUserID counts reset tokens for a user created within the given duration.
func (s *MemoryPasswordResetStore) CountRecentByUserID(userID int64, within time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().Add(-within)
	count := 0
	for _, pr := range s.resets {
		if pr.UserID == userID && pr.CreatedAt.After(cutoff) {
			count++
		}
	}
	return count, nil
}

// Resets returns the internal slice for test inspection.
func (s *MemoryPasswordResetStore) Resets() []*PasswordReset {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resets
}

// ClearResets clears all stored resets (for test setup).
func (s *MemoryPasswordResetStore) ClearResets() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resets = nil
}
