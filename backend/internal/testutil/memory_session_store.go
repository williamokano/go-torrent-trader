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

// DeleteByRefreshToken revokes a session by its refresh token, which is the half
// that outlives the access token (#231).
func (s *MemorySessionStore) DeleteByRefreshToken(refreshToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.byRefreshToken[refreshToken]; ok {
		delete(s.byAccessToken, sess.AccessToken)
		delete(s.byRefreshToken, refreshToken)
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

// ListByUserID returns the user's live sessions.
//
// Iterates the refresh index, which is the one that holds every session: the
// access index legitimately misses a session whose access token has expired,
// and those are exactly the ones a member most needs to see and revoke.
func (s *MemorySessionStore) ListByUserID(userID int64) ([]*service.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	var out []*service.Session
	for token, sess := range s.byRefreshToken {
		if sess.UserID != userID {
			continue
		}
		if now.After(sess.RefreshExpiresAt) {
			delete(s.byRefreshToken, token)
			delete(s.byAccessToken, sess.AccessToken)
			continue
		}
		// A copy, because Redis hands back a copy: the store holds serialised
		// sessions, and a caller that mutated a listed one there would change
		// nothing. Returning the live pointer here would let a test pass over
		// behaviour that cannot work against the real store.
		listed := *sess
		out = append(out, &listed)
	}
	return out, nil
}

func (s *MemorySessionStore) TouchLastActive(accessToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.byAccessToken[accessToken]; ok {
		sess.LastActive = time.Now()
	}
}
