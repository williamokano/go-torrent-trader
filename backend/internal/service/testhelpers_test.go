package service

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// memorySessionStore is testutil.MemorySessionStore for this package, which
// cannot import testutil (testutil imports service). Keep the two in step.
//
// It mirrors the PostgreSQL store's behaviour, because handler tests prove
// revocation and expiry against this double: one record per session holding
// both halves; lookups that ignore an expired half without deleting the other
// (an expired access token leaves the refresh token usable); revocation by user
// that reaches sessions whose access token has expired (#231); and a Rotate
// that only succeeds once per refresh token.
type memorySessionStore struct {
	mu       sync.Mutex
	sessions []*Session
}

var _ SessionStore = (*memorySessionStore)(nil)

func newTestSessionStore() *memorySessionStore {
	return &memorySessionStore{}
}

func (s *memorySessionStore) Create(session *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored := *session
	s.sessions = append(s.sessions, &stored)
	return nil
}

func (s *memorySessionStore) GetByAccessToken(token string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for _, sess := range s.sessions {
		if token != "" && sess.AccessToken == token && now.Before(sess.ExpiresAt) {
			found := *sess
			return &found, nil
		}
	}
	return nil, nil
}

func (s *memorySessionStore) GetByRefreshToken(token string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for _, sess := range s.sessions {
		if token != "" && sess.RefreshToken == token && now.Before(sess.RefreshExpiresAt) {
			found := *sess
			return &found, nil
		}
	}
	return nil, nil
}

// remove deletes every session drop reports true for, and returns how many.
func (s *memorySessionStore) remove(drop func(*Session) bool) int {
	kept := s.sessions[:0]
	removed := 0
	for _, sess := range s.sessions {
		if drop(sess) {
			removed++
			continue
		}
		kept = append(kept, sess)
	}
	s.sessions = kept
	return removed
}

func (s *memorySessionStore) Delete(accessToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remove(func(sess *Session) bool { return accessToken != "" && sess.AccessToken == accessToken })
}

// DeleteByRefreshToken revokes a session by its refresh token, which is the half
// that outlives the access token (#231).
func (s *memorySessionStore) DeleteByRefreshToken(refreshToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remove(func(sess *Session) bool { return refreshToken != "" && sess.RefreshToken == refreshToken })
}

func (s *memorySessionStore) Rotate(oldRefreshToken string, newSession *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	removed := s.remove(func(sess *Session) bool {
		return oldRefreshToken != "" && sess.RefreshToken == oldRefreshToken && now.Before(sess.RefreshExpiresAt)
	})
	if removed != 1 {
		return ErrInvalidToken
	}
	stored := *newSession
	s.sessions = append(s.sessions, &stored)
	return nil
}

func (s *memorySessionStore) DeleteByUserID(userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remove(func(sess *Session) bool { return sess.UserID == userID })
}

func (s *memorySessionStore) DeleteByUserIDExcept(userID int64, keepAccessToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remove(func(sess *Session) bool { return sess.UserID == userID && sess.AccessToken != keepAccessToken })
}

func (s *memorySessionStore) TouchLastActive(accessToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sess := range s.sessions {
		if accessToken != "" && sess.AccessToken == accessToken {
			sess.LastActive = time.Now()
		}
	}
}

// memoryPasswordResetStore is an in-memory PasswordResetStore for service-package tests.
type memoryPasswordResetStore struct {
	mu     sync.Mutex
	resets []*PasswordReset
	nextID int64
}

func newTestPasswordResetStore() *memoryPasswordResetStore {
	return &memoryPasswordResetStore{nextID: 1}
}

func (s *memoryPasswordResetStore) Create(pr *PasswordReset) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	pr.ID = s.nextID
	s.nextID++
	s.resets = append(s.resets, pr)
	return nil
}

func (s *memoryPasswordResetStore) ClaimByTokenHash(tokenHash string) (*PasswordReset, error) {
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

func (s *memoryPasswordResetStore) CountRecentByUserID(userID int64, within time.Duration) (int, error) {
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

func (s *memoryPasswordResetStore) Resets() []*PasswordReset {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resets
}

func (s *memoryPasswordResetStore) ClearResets() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resets = nil
}

// noopSender is a no-op email sender for service-package tests.
type noopSender struct {
	LastTo      string
	LastSubject string
	LastBody    string
	SendCount   int
}

func (n *noopSender) Send(_ context.Context, to, subject, body string) error {
	n.LastTo = to
	n.LastSubject = subject
	n.LastBody = body
	n.SendCount++
	return nil
}

// sessionOf unwraps a session lookup for tests that only care whether a
// session came back: a store error is a broken fixture, not an answer.
func sessionOf(sess *Session, err error) *Session {
	if err != nil {
		panic(fmt.Sprintf("session lookup failed: %v", err))
	}
	return sess
}
