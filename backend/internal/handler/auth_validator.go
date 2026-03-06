package handler

import "github.com/williamokano/go-torrent-trader/backend/internal/service"

// SessionValidatorAdapter bridges the session store with middleware.SessionValidator.
type SessionValidatorAdapter struct {
	sessions service.SessionStore
}

// NewSessionValidatorAdapter creates a new adapter.
func NewSessionValidatorAdapter(sessions service.SessionStore) *SessionValidatorAdapter {
	return &SessionValidatorAdapter{sessions: sessions}
}

// ValidateSession implements middleware.SessionValidator.
func (a *SessionValidatorAdapter) ValidateSession(accessToken string) (userID int64, groupID int64, ok bool) {
	sess := a.sessions.GetByAccessToken(accessToken)
	if sess == nil {
		return 0, 0, false
	}
	a.sessions.TouchLastActive(accessToken)
	return sess.UserID, sess.GroupID, true
}
