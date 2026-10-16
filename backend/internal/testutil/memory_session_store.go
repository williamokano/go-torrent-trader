package testutil

import (
	"sync"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// MemorySessionStore provides in-memory session storage for tests.
//
// It mirrors the PostgreSQL store's behaviour, because handler tests prove
// revocation and expiry against this double: one record per session holding
// both halves; lookups that ignore an expired half without deleting the other
// (an expired access token leaves the refresh token usable); revocation by user
// that reaches sessions whose access token has expired (#231); and a Rotate
// that only succeeds once per refresh token.
type MemorySessionStore struct {
	mu       sync.Mutex
	sessions []*service.Session
}

var _ service.SessionStore = (*MemorySessionStore)(nil)

// NewMemorySessionStore creates a new in-memory session store.
func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{}
}

func (s *MemorySessionStore) Create(session *service.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored := *session
	s.sessions = append(s.sessions, &stored)
	return nil
}

func (s *MemorySessionStore) GetByAccessToken(token string) (*service.Session, error) {
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

func (s *MemorySessionStore) GetByRefreshToken(token string) (*service.Session, error) {
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
func (s *MemorySessionStore) remove(drop func(*service.Session) bool) int {
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

func (s *MemorySessionStore) Delete(accessToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remove(func(sess *service.Session) bool { return accessToken != "" && sess.AccessToken == accessToken })
}

// DeleteByRefreshToken revokes a session by its refresh token, which is the half
// that outlives the access token (#231).
func (s *MemorySessionStore) DeleteByRefreshToken(refreshToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remove(func(sess *service.Session) bool { return refreshToken != "" && sess.RefreshToken == refreshToken })
}

func (s *MemorySessionStore) Rotate(oldRefreshToken string, newSession *service.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	removed := s.remove(func(sess *service.Session) bool {
		return oldRefreshToken != "" && sess.RefreshToken == oldRefreshToken && now.Before(sess.RefreshExpiresAt)
	})
	if removed != 1 {
		return service.ErrInvalidToken
	}
	stored := *newSession
	s.sessions = append(s.sessions, &stored)
	return nil
}

func (s *MemorySessionStore) DeleteByUserID(userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remove(func(sess *service.Session) bool { return sess.UserID == userID })
}

func (s *MemorySessionStore) DeleteByUserIDExcept(userID int64, keepAccessToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remove(func(sess *service.Session) bool { return sess.UserID == userID && sess.AccessToken != keepAccessToken })
}

func (s *MemorySessionStore) TouchLastActive(accessToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sess := range s.sessions {
		if accessToken != "" && sess.AccessToken == accessToken {
			sess.LastActive = time.Now()
		}
	}
}
