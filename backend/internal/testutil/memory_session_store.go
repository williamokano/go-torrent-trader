package testutil

import (
	"sync"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// MemorySessionStore provides in-memory session storage for tests.
type MemorySessionStore struct {
	mu             sync.RWMutex
	byAccessToken  map[string]*service.Session
	byRefreshToken map[string]*service.Session
}

// NewMemorySessionStore creates a new in-memory session store.
func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{
		byAccessToken:  make(map[string]*service.Session),
		byRefreshToken: make(map[string]*service.Session),
	}
}

func (s *MemorySessionStore) Create(session *service.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byAccessToken[session.AccessToken] = session
	s.byRefreshToken[session.RefreshToken] = session
	return nil
}

func (s *MemorySessionStore) GetByAccessToken(token string) *service.Session {
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

func (s *MemorySessionStore) GetByRefreshToken(token string) *service.Session {
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

func (s *MemorySessionStore) Delete(accessToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.byAccessToken[accessToken]; ok {
		delete(s.byRefreshToken, sess.RefreshToken)
		delete(s.byAccessToken, accessToken)
	}
}

func (s *MemorySessionStore) Rotate(oldRefreshToken string, newSession *service.Session) error {
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

func (s *MemorySessionStore) TouchLastActive(accessToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.byAccessToken[accessToken]; ok {
		sess.LastActive = time.Now()
	}
}
