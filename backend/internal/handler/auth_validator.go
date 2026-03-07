package handler

import (
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// SessionValidatorAdapter bridges the session store with middleware.SessionValidator.
type SessionValidatorAdapter struct {
	sessions service.SessionStore
}

// NewSessionValidatorAdapter creates a new adapter.
func NewSessionValidatorAdapter(sessions service.SessionStore) *SessionValidatorAdapter {
	return &SessionValidatorAdapter{sessions: sessions}
}

// ValidateSession implements middleware.SessionValidator.
func (a *SessionValidatorAdapter) ValidateSession(accessToken string) (userID int64, perms model.Permissions, ok bool) {
	sess := a.sessions.GetByAccessToken(accessToken)
	if sess == nil {
		return 0, model.Permissions{}, false
	}
	a.sessions.TouchLastActive(accessToken)
	return sess.UserID, sess.Permissions, true
}
