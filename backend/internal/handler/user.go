package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// UserHandler handles user profile HTTP endpoints.
type UserHandler struct {
	users *service.UserService
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(users *service.UserService) *UserHandler {
	return &UserHandler{users: users}
}

// HandleGetProfile handles GET /api/v1/users/{id}.
func (h *UserHandler) HandleGetProfile(w http.ResponseWriter, r *http.Request) {
	viewerID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	idParam := chi.URLParam(r, "id")
	userID, err := strconv.ParseInt(idParam, 10, 64)
	if err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid user ID")
		return
	}

	profile, err := h.users.GetProfile(r.Context(), userID, viewerID)
	if err != nil {
		handleUserError(w, err)
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"user": profile,
	})
}

// HandleUpdateProfile handles PUT /api/v1/users/me/profile.
func (h *UserHandler) HandleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	var req service.UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	profile, err := h.users.UpdateProfile(r.Context(), userID, req)
	if err != nil {
		handleUserError(w, err)
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"user": profile,
	})
}

// HandleChangePassword handles PUT /api/v1/users/me/password.
func (h *UserHandler) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	accessToken, ok := middleware.AccessTokenFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	var req service.ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	if err := h.users.ChangePassword(r.Context(), userID, accessToken, req); err != nil {
		handleUserError(w, err)
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "password changed successfully",
	})
}

// HandleRegeneratePasskey handles POST /api/v1/users/me/passkey.
func (h *UserHandler) HandleRegeneratePasskey(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	passkey, err := h.users.RegeneratePasskey(r.Context(), userID)
	if err != nil {
		handleUserError(w, err)
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"passkey": passkey,
	})
}

func handleUserError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrUserNotFound):
		ErrorResponse(w, http.StatusNotFound, "not_found", "user not found")
	case errors.Is(err, service.ErrIncorrectPassword):
		ErrorResponse(w, http.StatusUnauthorized, "incorrect_password", "current password is incorrect")
	case errors.Is(err, service.ErrValidationFailed):
		ErrorResponse(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
	default:
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "an unexpected error occurred")
	}
}
