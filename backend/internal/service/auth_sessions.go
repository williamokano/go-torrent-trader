package service

import (
	"sort"
	"time"
)

// SessionInfo is one row of "where am I signed in": everything a member needs
// to recognise a session, and nothing that could be used to resume it.
//
// Neither token appears here, in any form. The list exists so that a member who
// suspects someone else is on their account can act on it themselves, and an
// endpoint that answered that question by handing back live credentials would
// be a larger hole than the one it closes.
type SessionInfo struct {
	ID         string    `json:"id"`
	DeviceName string    `json:"device_name"`
	IP         string    `json:"ip"`
	CreatedAt  time.Time `json:"created_at"`
	LastActive time.Time `json:"last_active"`
	ExpiresAt  time.Time `json:"expires_at"`
	// Current marks the session making the request, so the UI can label it and
	// keep the member from signing themselves out without meaning to.
	Current bool `json:"current"`
}

// ListSessions returns the member's live sessions, most recently active first.
//
// currentAccessToken names the caller's own session; pass "" when there is
// none, and no row is marked current.
func (s *AuthService) ListSessions(userID int64, currentAccessToken string) []SessionInfo {
	sessions := s.sessions.ListByUserID(userID)

	out := make([]SessionInfo, 0, len(sessions))
	for _, sess := range sessions {
		if sess == nil {
			continue
		}
		out = append(out, SessionInfo{
			ID:         SessionID(sess),
			DeviceName: sess.DeviceName,
			IP:         sess.IP,
			CreatedAt:  sess.CreatedAt,
			LastActive: sess.LastActive,
			ExpiresAt:  sess.ExpiresAt,
			Current:    currentAccessToken != "" && sess.AccessToken == currentAccessToken,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].LastActive.Equal(out[j].LastActive) {
			return out[i].ID < out[j].ID // stable order for sessions created together
		}
		return out[i].LastActive.After(out[j].LastActive)
	})
	return out
}

// RevokeSession revokes one of the member's own sessions by its ID, returning
// ErrSessionNotFound when they have no such session.
//
// The lookup is scoped to the caller's own sessions, and that is the whole
// authorization check: an ID belonging to somebody else is simply not in the
// list, so it is indistinguishable from one that has already expired. There is
// nothing to leak and nothing to enumerate.
//
// Revoking by refresh token rather than access token is deliberate — the
// refresh half is the one that outlives the other by twenty-nine days, and a
// session whose access token has already expired is exactly the kind a member
// comes here to kill (#231).
func (s *AuthService) RevokeSession(userID int64, sessionID string) error {
	if sessionID == "" {
		return ErrSessionNotFound
	}

	for _, sess := range s.sessions.ListByUserID(userID) {
		if sess == nil || SessionID(sess) != sessionID {
			continue
		}
		s.sessions.DeleteByRefreshToken(sess.RefreshToken)
		return nil
	}
	return ErrSessionNotFound
}

// RevokeOtherSessions signs the member out everywhere except here, and reports
// how many sessions it ended.
//
// The panic button: it is what someone reaches for when they believe another
// person is on their account, so it must not log the caller out in the process —
// they still have a password to change.
func (s *AuthService) RevokeOtherSessions(userID int64, keepAccessToken string) int {
	revoked := 0
	for _, sess := range s.sessions.ListByUserID(userID) {
		if sess != nil && sess.AccessToken != keepAccessToken {
			revoked++
		}
	}

	s.sessions.DeleteByUserIDExcept(userID, keepAccessToken)
	return revoked
}
