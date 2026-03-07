package service

import (
	"context"
	"sync"
	"time"
)

// newTestSessionStore creates a MemorySessionStore for service-package tests.
// This avoids an import cycle with the testutil package.
type memorySessionStore struct {
	mu             sync.RWMutex
	byAccessToken  map[string]*Session
	byRefreshToken map[string]*Session
}

func newTestSessionStore() *memorySessionStore {
	return &memorySessionStore{
		byAccessToken:  make(map[string]*Session),
		byRefreshToken: make(map[string]*Session),
	}
}

func (s *memorySessionStore) Create(session *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byAccessToken[session.AccessToken] = session
	s.byRefreshToken[session.RefreshToken] = session
	return nil
}

func (s *memorySessionStore) GetByAccessToken(token string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.byAccessToken[token]
	if !ok {
		return nil
	}
	if time.Now().After(sess.ExpiresAt) {
		delete(s.byAccessToken, token)
		delete(s.byRefreshToken, sess.RefreshToken)
		return nil
	}
	return sess
}

func (s *memorySessionStore) GetByRefreshToken(token string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.byRefreshToken[token]
	if !ok {
		return nil
	}
	if time.Now().After(sess.RefreshExpiresAt) {
		delete(s.byRefreshToken, token)
		delete(s.byAccessToken, sess.AccessToken)
		return nil
	}
	return sess
}

func (s *memorySessionStore) Delete(accessToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.byAccessToken[accessToken]; ok {
		delete(s.byRefreshToken, sess.RefreshToken)
		delete(s.byAccessToken, accessToken)
	}
}

func (s *memorySessionStore) Rotate(oldRefreshToken string, newSession *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.byRefreshToken[oldRefreshToken]; ok {
		delete(s.byAccessToken, old.AccessToken)
		delete(s.byRefreshToken, oldRefreshToken)
	}
	s.byAccessToken[newSession.AccessToken] = newSession
	s.byRefreshToken[newSession.RefreshToken] = newSession
	return nil
}

func (s *memorySessionStore) DeleteByUserID(userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, sess := range s.byAccessToken {
		if sess.UserID == userID {
			delete(s.byRefreshToken, sess.RefreshToken)
			delete(s.byAccessToken, token)
		}
	}
}

func (s *memorySessionStore) DeleteByUserIDExcept(userID int64, keepAccessToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, sess := range s.byAccessToken {
		if sess.UserID == userID && token != keepAccessToken {
			delete(s.byRefreshToken, sess.RefreshToken)
			delete(s.byAccessToken, token)
		}
	}
}

func (s *memorySessionStore) TouchLastActive(accessToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.byAccessToken[accessToken]; ok {
		sess.LastActive = time.Now()
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
