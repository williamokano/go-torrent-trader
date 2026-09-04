package service

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// SessionStore defines the interface for session persistence.
type SessionStore interface {
	Create(session *Session) error
	GetByAccessToken(token string) *Session
	GetByRefreshToken(token string) *Session
	Delete(accessToken string)
	// DeleteByRefreshToken revokes a session identified by its refresh token.
	//
	// The access token lives an hour and the refresh token thirty days, so a
	// session outlives the only credential Delete could identify it by. Without
	// this there was no way to revoke one after that first hour (#231).
	DeleteByRefreshToken(refreshToken string)
	DeleteByUserID(userID int64)
	DeleteByUserIDExcept(userID int64, keepAccessToken string)
	// ListByUserID returns every live session belonging to a user.
	//
	// The per-user index this reads already existed so that sessions could be
	// revoked together; the missing piece was ever letting the member see them.
	// Order is unspecified — callers that show the list sort it themselves.
	//
	// The error matters: this backs the page a member opens to find out whether
	// somebody else is signed in as them, and a storage failure rendered as an
	// empty list would answer that question wrongly in the reassuring direction.
	ListByUserID(userID int64) ([]*Session, error)
	Rotate(oldRefreshToken string, newSession *Session) error
	TouchLastActive(accessToken string)
}

// Session represents an authenticated user session.
type Session struct {
	// ID is a stable, opaque handle for the session, minted when it is created
	// and carried across every refresh rotation. It is what the session
	// endpoints name a session by: both tokens are credentials, so neither may
	// leave the store, and a member still has to be able to point at one device
	// and say "not that one".
	ID               string            `json:"id"`
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

// SessionID returns the identifier a session is named by over the API.
//
// Sessions created before Session.ID existed unmarshal with an empty one, and
// they stay usable for up to thirty days, so they need an identifier too: a hash
// of the refresh token is stable for the life of that session, opaque, and not
// reversible into the credential it comes from. Refresh persists whatever this
// returns, so such a session keeps its hash-derived id for the rest of its life
// rather than being renamed under the member mid-list.
//
// A session with neither an id nor a refresh token has no identity at all —
// hashing the empty string would give every one of them the same well-known
// digest — so it is deliberately unaddressable.
func SessionID(s *Session) string {
	if s == nil {
		return ""
	}
	if s.ID != "" {
		return s.ID
	}
	if s.RefreshToken == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(s.RefreshToken))
	return hex.EncodeToString(sum[:16])
}

// SessionStoreConfig holds configuration for the session store factory.
type SessionStoreConfig struct {
	Type            string        // "redis"
	RedisURL        string        // Redis connection URL
	AccessTokenTTL  time.Duration // TTL for access token keys
	RefreshTokenTTL time.Duration // TTL for refresh token keys
}
