package handler

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// AuthHandler handles authentication HTTP endpoints.
type AuthHandler struct {
	auth *service.AuthService
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(auth *service.AuthService) *AuthHandler {
	return &AuthHandler{auth: auth}
}

// HandleRegister handles POST /api/v1/auth/register.
func (h *AuthHandler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	var req service.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	ip := clientIP(r)
	user, tokens, err := h.auth.Register(r.Context(), req, ip)
	if err != nil {
		handleAuthError(w, err)
		return
	}

	JSON(w, http.StatusCreated, map[string]interface{}{
		"user":   userResponse(user),
		"tokens": tokens,
	})
}

// HandleLogin handles POST /api/v1/auth/login.
func (h *AuthHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var req service.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	ip := clientIP(r)
	user, tokens, err := h.auth.Login(r.Context(), req, ip)
	if err != nil {
		handleAuthError(w, err)
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"user":   userResponse(user),
		"tokens": tokens,
	})
}

// HandleRefresh handles POST /api/v1/auth/refresh.
func (h *AuthHandler) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	var req service.RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	ip := clientIP(r)
	tokens, err := h.auth.Refresh(req, ip)
	if err != nil {
		handleAuthError(w, err)
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"tokens": tokens,
	})
}

// HandleLogout handles POST /api/v1/auth/logout.
// Must be behind RequireAuth middleware.
func (h *AuthHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	token, ok := middleware.AccessTokenFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	h.auth.Logout(token)
	w.WriteHeader(http.StatusNoContent)
}

// HandleMe handles GET /api/v1/auth/me.
// Must be behind RequireAuth middleware.
func (h *AuthHandler) HandleMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	user, err := h.auth.GetCurrentUser(r.Context(), userID)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to get user")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"user": userResponse(user),
	})
}

func handleAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidCredentials):
		ErrorResponse(w, http.StatusUnauthorized, "invalid_credentials", "invalid username or password")
	case errors.Is(err, service.ErrUsernameTaken):
		ErrorResponse(w, http.StatusConflict, "username_taken", "username is already taken")
	case errors.Is(err, service.ErrEmailTaken):
		ErrorResponse(w, http.StatusConflict, "email_taken", "email is already taken")
	case errors.Is(err, service.ErrInvalidToken):
		ErrorResponse(w, http.StatusUnauthorized, "invalid_token", "invalid or expired token")
	case errors.Is(err, service.ErrValidationFailed):
		ErrorResponse(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
	default:
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "an unexpected error occurred")
	}
}

// clientIP extracts the IP address from RemoteAddr, stripping the port.
// Chi's RealIP middleware has already resolved X-Forwarded-For into RemoteAddr.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func userResponse(u *model.User) map[string]interface{} {
	return map[string]interface{}{
		"id":         u.ID,
		"username":   u.Username,
		"email":      u.Email,
		"group_id":   u.GroupID,
		"uploaded":   u.Uploaded,
		"downloaded": u.Downloaded,
		"enabled":    u.Enabled,
		"created_at": u.CreatedAt,
	}
}
