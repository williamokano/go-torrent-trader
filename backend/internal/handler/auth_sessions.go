package handler

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// The session endpoints answer "who else is signed in as me, and how do I stop
// them" without a password reset. Until now the only remedy for a stolen
// session was changing the password, which revokes everything including the
// member's own devices and tells them nothing about what happened (#171).
//
// All three sit behind RequireAuth. Unlike /auth/logout, which deliberately
// accepts a refresh token because the session it revokes may have no live access
// token left, these act on *other* sessions — so the caller has to prove they
// are currently signed in, not merely holding a credential.

// HandleListSessions handles GET /api/v1/auth/sessions.
// Must be behind RequireAuth middleware.
func (h *AuthHandler) HandleListSessions(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	current, _ := middleware.AccessTokenFromContext(r.Context())
	JSON(w, http.StatusOK, map[string]interface{}{
		"sessions": h.auth.ListSessions(userID, current),
	})
}

// HandleRevokeSession handles DELETE /api/v1/auth/sessions/{id}.
// Must be behind RequireAuth middleware.
//
// Revoking the caller's own session is allowed — it is a logout by another
// name, and refusing it would mean a member could not sign out a device from
// that device.
func (h *AuthHandler) HandleRevokeSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	err := h.auth.RevokeSession(userID, chi.URLParam(r, "id"))
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, service.ErrSessionNotFound):
		// Also the answer for another member's session ID: the lookup only ever
		// sees the caller's own sessions, so "yours has expired" and "that one
		// was never yours" are the same response, and neither can be used to
		// probe for somebody else's.
		ErrorResponse(w, http.StatusNotFound, "not_found", "no such session")
	default:
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to revoke session")
	}
}

// HandleRevokeOtherSessions handles DELETE /api/v1/auth/sessions.
// Must be behind RequireAuth middleware.
//
// Keeps the calling session alive on purpose: this is what a member reaches for
// when they think someone else is on their account, and logging them out in the
// middle of that would stop them changing their password.
func (h *AuthHandler) HandleRevokeOtherSessions(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	current, ok := middleware.AccessTokenFromContext(r.Context())
	if !ok || current == "" {
		// RequireAuth cannot have let the request through without one, so this
		// is a wiring error rather than a client one. Refuse rather than pass ""
		// to a call that would read it as "keep nothing" and sign the member out
		// of every device including this one.
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"revoked": h.auth.RevokeOtherSessions(userID, current),
	})
}
