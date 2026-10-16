package handler

import (
	"time"

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
//
// It records activity only when the session's LastActive is older than
// service.SessionTouchInterval. This runs on every authenticated request, and
// the session was read a moment ago, so checking here skips a second database
// round trip on all but about one request a minute per session.
func (a *SessionValidatorAdapter) ValidateSession(accessToken string) (userID int64, perms model.Permissions, ok bool, err error) {
	sess, err := a.sessions.GetByAccessToken(accessToken)
	if err != nil {
		return 0, model.Permissions{}, false, err
	}
	if sess == nil {
		return 0, model.Permissions{}, false, nil
	}
	if time.Since(sess.LastActive) >= service.SessionTouchInterval {
		a.sessions.TouchLastActive(accessToken)
	}
	return sess.UserID, sess.Permissions, true, nil
}
