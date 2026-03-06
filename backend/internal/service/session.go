package service

import (
	"sync"
	"time"
)

const (
	AccessTokenTTL  = 1 * time.Hour
	RefreshTokenTTL = 30 * 24 * time.Hour
)

// Session represents an authenticated user session.
type Session struct {
	UserID           int64
	GroupID          int64
	AccessToken      string
	RefreshToken     string
	DeviceName       string
	IP               string
	CreatedAt        time.Time
	LastActive       time.Time
	ExpiresAt        time.Time // access token expiry
	RefreshExpiresAt time.Time
}

// SessionStore provides in-memory session storage.
// Keyed by access token for fast lookup, with a secondary index by refresh token.
type SessionStore struct {
	mu              sync.RWMutex
	byAccessToken   map[string]*Session
	byRefreshToken  map[string]*Session
}

// NewSessionStore creates a new in-memory session store.
func NewSessionStore() *SessionStore {
	return &SessionStore{
		byAccessToken:  make(map[string]*Session),
		byRefreshToken: make(map[string]*Session),
	}
}

// Create stores a new session.
func (s *SessionStore) Create(session *Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byAccessToken[session.AccessToken] = session
	s.byRefreshToken[session.RefreshToken] = session
}

// GetByAccessToken retrieves a session by its access token.
// Returns nil if not found or expired. Expired sessions are lazily cleaned up.
func (s *SessionStore) GetByAccessToken(token string) *Session {
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

// GetByRefreshToken retrieves a session by its refresh token.
// Returns nil if not found or expired. Expired sessions are lazily cleaned up.
func (s *SessionStore) GetByRefreshToken(token string) *Session {
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

// Delete removes a session by access token.
func (s *SessionStore) Delete(accessToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.byAccessToken[accessToken]; ok {
		delete(s.byRefreshToken, sess.RefreshToken)
		delete(s.byAccessToken, accessToken)
	}
}

// Rotate invalidates the old session and creates a new one with fresh tokens.
func (s *SessionStore) Rotate(oldRefreshToken string, newSession *Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.byRefreshToken[oldRefreshToken]; ok {
		delete(s.byAccessToken, old.AccessToken)
		delete(s.byRefreshToken, oldRefreshToken)
	}
	s.byAccessToken[newSession.AccessToken] = newSession
	s.byRefreshToken[newSession.RefreshToken] = newSession
}

// DeleteByUserID removes all sessions for a given user ID.
func (s *SessionStore) DeleteByUserID(userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, sess := range s.byAccessToken {
		if sess.UserID == userID {
			delete(s.byRefreshToken, sess.RefreshToken)
			delete(s.byAccessToken, token)
		}
	}
}

// DeleteByUserIDExcept removes all sessions for a given user ID except the one
// matching the provided access token. Used when changing password to keep
// the current session alive.
func (s *SessionStore) DeleteByUserIDExcept(userID int64, keepAccessToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, sess := range s.byAccessToken {
		if sess.UserID == userID && token != keepAccessToken {
			delete(s.byRefreshToken, sess.RefreshToken)
			delete(s.byAccessToken, token)
		}
	}
}

// TouchLastActive updates the session's LastActive timestamp.
func (s *SessionStore) TouchLastActive(accessToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.byAccessToken[accessToken]; ok {
		sess.LastActive = time.Now()
	}
}
