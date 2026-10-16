package service

import (
	"errors"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// ErrSessionStoreUnavailable is returned, wrapped, when a session could not be
// looked up because the store failed — as opposed to the session not existing.
// HTTP callers answer it with 503, never 401.
var ErrSessionStoreUnavailable = errors.New("session store unavailable")

// SessionTouchInterval is the least time between two recorded activity
// updates for one session. TouchLastActive runs on every authenticated
// request, so without it browsing would be a write per click.
const SessionTouchInterval = time.Minute

// SessionStore defines the interface for session persistence.
//
// PostgreSQL implements it (repository/postgres.SessionStore) and
// testutil.MemorySessionStore stands in for tests. It is the seam a different
// backend — Redis again, say — would plug into, without touching auth.
//
// A lookup answers with the member's current authority, never a copy taken at
// login (#291): it returns no session when the account is disabled, and the
// GroupID and Permissions of the member's group as it is now. The PostgreSQL
// store does this by joining the account on every lookup.
//
// A lookup returns (nil, nil) when there is no such live session, and an error
// only when the store could not answer. The two must stay apart: a member whose
// session simply could not be read during a database blip has to see a
// retryable failure, not a 401 — the frontend treats a 401 from refresh as a
// real logout and throws its tokens away.
type SessionStore interface {
	Create(session *Session) error
	GetByAccessToken(token string) (*Session, error)
	GetByRefreshToken(token string) (*Session, error)
	Delete(accessToken string)
	// DeleteByRefreshToken revokes a session identified by its refresh token.
	//
	// The access token lives an hour and the refresh token thirty days, so a
	// session outlives the only credential Delete could identify it by. Without
	// this there was no way to revoke one after that first hour (#231).
	DeleteByRefreshToken(refreshToken string)
	DeleteByUserID(userID int64)
	DeleteByUserIDExcept(userID int64, keepAccessToken string)
	// Rotate replaces the session holding oldRefreshToken with newSession. It
	// returns ErrInvalidToken, creating nothing, when that session is no longer
	// there — already rotated by a concurrent refresh, or revoked since the
	// caller looked it up — so one refresh token can never mint two pairs, nor
	// bring back a session that was logged out.
	Rotate(oldRefreshToken string, newSession *Session) error
	TouchLastActive(accessToken string)
}

// Session represents an authenticated user session.
type Session struct {
	UserID           int64             `json:"user_id"`
	GroupID          int64             `json:"group_id"`
	Permissions      model.Permissions `json:"permissions"`
	AccessToken      string            `json:"access_token"`
	RefreshToken     string            `json:"refresh_token"`
	DeviceName       string            `json:"device_name"`
	IP               string            `json:"ip"`
	CreatedAt        time.Time         `json:"created_at"`
	LastActive       time.Time         `json:"last_active"`
	ExpiresAt        time.Time         `json:"expires_at"`
	RefreshExpiresAt time.Time         `json:"refresh_expires_at"`
}
