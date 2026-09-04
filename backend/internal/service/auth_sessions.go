package service

import (
	"fmt"
	"sort"
	"time"
)

// revokeAttempts bounds the retry below. A session is revoked by deleting the
// refresh token a listing named, and /auth/refresh replaces that token — so a
// device that rotates in the window between the two deletes nothing, and the
// member is told a session is gone while it carries on under a new pair. The
// session most worth evicting is the one in active use, which is exactly the one
// that rotates. Three passes is far more than a legitimate client can force.
const revokeAttempts = 3

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
	// ExpiresAt is when the session itself dies — its refresh expiry, not the
	// hourly access-token one. A member reading "expires" wants to know when
	// this device stops being signed in, and the access token's expiry is in
	// the past for every session that is not mid-request.
	ExpiresAt time.Time `json:"expires_at"`
	// Current marks the session making the request, so the UI can label it and
	// keep the member from signing themselves out without meaning to.
	Current bool `json:"current"`
}

// ListSessions returns the member's live sessions, most recently active first.
//
// currentAccessToken names the caller's own session; pass "" when there is
// none, and no row is marked current.
func (s *AuthService) ListSessions(userID int64, currentAccessToken string) ([]SessionInfo, error) {
	sessions, err := s.sessions.ListByUserID(userID)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}

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
			ExpiresAt:  sess.RefreshExpiresAt,
			Current:    currentAccessToken != "" && sess.AccessToken == currentAccessToken,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].LastActive.Equal(out[j].LastActive) {
			return out[i].ID < out[j].ID // stable order for sessions created together
		}
		return out[i].LastActive.After(out[j].LastActive)
	})
	return out, nil
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
//
// The loop is what makes the answer honest. Deleting a refresh token cannot
// report whether it hit anything, and the target may rotate that token, or a
// store may fail, between the listing and the delete — so the session is looked
// up again and only a listing that no longer contains it counts as revoked.
// Anything else is an error, never a 204 over a session that is still alive.
func (s *AuthService) RevokeSession(userID int64, sessionID string) error {
	if sessionID == "" {
		return ErrSessionNotFound
	}

	for attempt := 0; attempt < revokeAttempts; attempt++ {
		sessions, err := s.sessions.ListByUserID(userID)
		if err != nil {
			return fmt.Errorf("revoke session: %w", err)
		}

		target := findSession(sessions, sessionID)
		if target == nil {
			// Gone. On the first pass that means the member never had it; on a
			// later one it means the delete below worked.
			if attempt == 0 {
				return ErrSessionNotFound
			}
			return nil
		}
		s.sessions.DeleteByRefreshToken(target.RefreshToken)
	}

	return fmt.Errorf("revoke session %q: still present after %d attempts", sessionID, revokeAttempts)
}

// RevokeOtherSessions signs the member out everywhere except here, and reports
// how many sessions it ended.
//
// The panic button: it is what someone reaches for when they believe another
// person is on their account, so it must not log the caller out in the process —
// they still have a password to change.
//
// Which is why it revokes each session by name instead of calling
// DeleteByUserIDExcept. That helper is told which session to keep by access
// token and resolves it through the access key; if that lookup misses — the
// caller's own session rotated in another tab, the key expired, the store
// hiccuped — it keeps nothing and deletes everything, including the session
// making the request. Failing closed is the only safe reading: if the caller's
// own session cannot be found, nothing is revoked.
func (s *AuthService) RevokeOtherSessions(userID int64, keepAccessToken string) (int, error) {
	if keepAccessToken == "" {
		return 0, fmt.Errorf("revoke other sessions: no session to keep")
	}

	sessions, err := s.sessions.ListByUserID(userID)
	if err != nil {
		return 0, fmt.Errorf("revoke other sessions: %w", err)
	}

	var keep *Session
	for _, sess := range sessions {
		if sess != nil && sess.AccessToken == keepAccessToken {
			keep = sess
			break
		}
	}
	if keep == nil {
		return 0, fmt.Errorf("revoke other sessions: the calling session could not be identified")
	}

	revoked := 0
	for _, sess := range sessions {
		if sess == nil || sess.RefreshToken == keep.RefreshToken {
			continue
		}
		if err := s.RevokeSession(userID, SessionID(sess)); err != nil {
			// ErrSessionNotFound here means it expired while we worked, which is
			// the outcome asked for; anything else means a session survived.
			if err == ErrSessionNotFound {
				continue
			}
			return revoked, err
		}
		revoked++
	}
	return revoked, nil
}

// findSession returns the session with the given public ID, or nil.
func findSession(sessions []*Session, sessionID string) *Session {
	for _, sess := range sessions {
		if sess != nil && SessionID(sess) == sessionID {
			return sess
		}
	}
	return nil
}
