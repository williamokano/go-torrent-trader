package service

import (
	"sync"
	"time"
)

// SessionStore defines the interface for session persistence.
type SessionStore interface {
	Create(session *Session) error
	GetByAccessToken(token string) *Session
	GetByRefreshToken(token string) *Session
	Delete(accessToken string)
	DeleteByUserID(userID int64)
	DeleteByUserIDExcept(userID int64, keepAccessToken string)
	Rotate(oldRefreshToken string, newSession *Session) error
	TouchLastActive(accessToken string)
}

// Session represents an authenticated user session.
type Session struct {
	UserID           int64     `json:"user_id"`
	GroupID          int64     `json:"group_id"`
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token"`
	DeviceName       string    `json:"device_name"`
	IP               string    `json:"ip"`
	CreatedAt        time.Time `json:"created_at"`
	LastActive       time.Time `json:"last_active"`
	ExpiresAt        time.Time `json:"expires_at"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
}

// SessionStoreConfig holds configuration for the session store factory.
type SessionStoreConfig struct {
	Type            string        // "memory" or "redis"
	RedisURL        string        // Redis connection URL
	AccessTokenTTL  time.Duration // TTL for access token keys
	RefreshTokenTTL time.Duration // TTL for refresh token keys
}

// MemorySessionStore provides in-memory session storage.
// Keyed by access token for fast lookup, with a secondary index by refresh token.
type MemorySessionStore struct {
	mu             sync.RWMutex
	byAccessToken  map[string]*Session
	byRefreshToken map[string]*Session
}

// NewMemorySessionStore creates a new in-memory session store.
func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{
		byAccessToken:  make(map[string]*Session),
		byRefreshToken: make(map[string]*Session),
	}
}

// Create stores a new session.
func (s *MemorySessionStore) Create(session *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byAccessToken[session.AccessToken] = session
	s.byRefreshToken[session.RefreshToken] = session
	return nil
}

// GetByAccessToken retrieves a session by its access token.
// Returns nil if not found or expired. Expired sessions are lazily cleaned up.
func (s *MemorySessionStore) GetByAccessToken(token string) *Session {
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
func (s *MemorySessionStore) GetByRefreshToken(token string) *Session {
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
func (s *MemorySessionStore) Delete(accessToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.byAccessToken[accessToken]; ok {
		delete(s.byRefreshToken, sess.RefreshToken)
		delete(s.byAccessToken, accessToken)
	}
}

// Rotate invalidates the old session and creates a new one with fresh tokens.
func (s *MemorySessionStore) Rotate(oldRefreshToken string, newSession *Session) error {
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

// DeleteByUserID removes all sessions for a given user ID.
func (s *MemorySessionStore) DeleteByUserID(userID int64) {
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
func (s *MemorySessionStore) DeleteByUserIDExcept(userID int64, keepAccessToken string) {
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
func (s *MemorySessionStore) TouchLastActive(accessToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.byAccessToken[accessToken]; ok {
		sess.LastActive = time.Now()
	}
}
