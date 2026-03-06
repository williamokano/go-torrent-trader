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

// PasswordResetStore provides in-memory storage for password reset tokens.
type PasswordResetStore struct {
	mu     sync.Mutex
	resets []*PasswordReset
	nextID int64
}

// NewPasswordResetStore creates a new in-memory password reset store.
func NewPasswordResetStore() *PasswordResetStore {
	return &PasswordResetStore{nextID: 1}
}

// Create stores a new password reset record.
func (s *PasswordResetStore) Create(pr *PasswordReset) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pr.ID = s.nextID
	s.nextID++
	s.resets = append(s.resets, pr)
}

// GetByTokenHash retrieves a password reset by its token hash.
// Returns nil if not found.
func (s *PasswordResetStore) GetByTokenHash(tokenHash string) *PasswordReset {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, pr := range s.resets {
		if pr.TokenHash == tokenHash {
			return pr
		}
	}
	return nil
}

// CountRecentByUserID counts non-expired, unused reset tokens for a user
// created within the given duration.
func (s *PasswordResetStore) CountRecentByUserID(userID int64, within time.Duration) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().Add(-within)
	count := 0
	for _, pr := range s.resets {
		if pr.UserID == userID && pr.CreatedAt.After(cutoff) {
			count++
		}
	}
	return count
}

// MarkUsed marks a password reset record as used.
func (s *PasswordResetStore) MarkUsed(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, pr := range s.resets {
		if pr.ID == id {
			pr.Used = true
			return
		}
	}
}
