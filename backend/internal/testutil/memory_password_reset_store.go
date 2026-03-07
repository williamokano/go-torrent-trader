package testutil

import (
	"sync"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// MemoryPasswordResetStore provides in-memory storage for password reset tokens (tests only).
type MemoryPasswordResetStore struct {
	mu     sync.Mutex
	resets []*service.PasswordReset
	nextID int64
}

// NewMemoryPasswordResetStore creates a new in-memory password reset store.
func NewMemoryPasswordResetStore() *MemoryPasswordResetStore {
	return &MemoryPasswordResetStore{nextID: 1}
}

func (s *MemoryPasswordResetStore) Create(pr *service.PasswordReset) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	pr.ID = s.nextID
	s.nextID++
	s.resets = append(s.resets, pr)
	return nil
}

func (s *MemoryPasswordResetStore) ClaimByTokenHash(tokenHash string) (*service.PasswordReset, error) {
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
func (s *MemoryPasswordResetStore) Resets() []*service.PasswordReset {
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
