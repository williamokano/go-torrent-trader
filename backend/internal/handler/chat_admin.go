package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// ChatAdminHandler handles admin chat moderation endpoints.
type ChatAdminHandler struct {
	chatSvc *service.ChatService
	hub     *ChatHub
}

// NewChatAdminHandler creates a new ChatAdminHandler.
func NewChatAdminHandler(chatSvc *service.ChatService, hub *ChatHub) *ChatAdminHandler {
	return &ChatAdminHandler{chatSvc: chatSvc, hub: hub}
}

// HandleDeleteMessage handles DELETE /api/v1/admin/chat/messages/{id}.
func (h *ChatAdminHandler) HandleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	actorID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}
	perms := middleware.PermissionsFromContext(r.Context())

	msgID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || msgID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid message ID")
		return
	}

	if err := h.chatSvc.DeleteMessage(r.Context(), msgID, actorID, perms); err != nil {
		switch {
		case errors.Is(err, service.ErrForbidden):
			ErrorResponse(w, http.StatusForbidden, "forbidden", "you do not have permission")
		case errors.Is(err, service.ErrChatMessageNotFound):
			ErrorResponse(w, http.StatusNotFound, "not_found", "chat message not found")
		default:
			ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to delete message")
		}
		return
	}

	h.hub.BroadcastDelete(msgID)
	w.WriteHeader(http.StatusNoContent)
}

// HandleDeleteUserMessages handles DELETE /api/v1/admin/chat/users/{id}/messages.
func (h *ChatAdminHandler) HandleDeleteUserMessages(w http.ResponseWriter, r *http.Request) {
	actorID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}
	perms := middleware.PermissionsFromContext(r.Context())

	userID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || userID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid user ID")
		return
	}

	count, err := h.chatSvc.DeleteUserMessages(r.Context(), userID, actorID, perms)
	if err != nil {
		if errors.Is(err, service.ErrForbidden) {
			ErrorResponse(w, http.StatusForbidden, "forbidden", "you do not have permission")
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to delete messages")
		return
	}

	// Broadcast a delete_user event so clients remove this user's messages.
	h.hub.BroadcastDeleteUser(userID)

	JSON(w, http.StatusOK, map[string]interface{}{
		"deleted": count,
	})
}

type muteUserRequest struct {
	DurationMinutes int    `json:"duration_minutes"`
	Reason          string `json:"reason"`
}

// HandleMuteUser handles POST /api/v1/admin/chat/users/{id}/mute.
func (h *ChatAdminHandler) HandleMuteUser(w http.ResponseWriter, r *http.Request) {
	actorID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}
	perms := middleware.PermissionsFromContext(r.Context())

	userID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || userID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid user ID")
		return
	}

	var req muteUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	if req.DurationMinutes <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "duration_minutes must be positive")
		return
	}

	mute, err := h.chatSvc.MuteUser(r.Context(), userID, actorID, req.DurationMinutes, req.Reason, perms)
	if err != nil {
		if errors.Is(err, service.ErrForbidden) {
			ErrorResponse(w, http.StatusForbidden, "forbidden", "you do not have permission")
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to mute user")
		return
	}

	// Notify the muted user's WebSocket client(s) in real time.
	mutePayload, err := json.Marshal(map[string]interface{}{
		"type":       "mute",
		"expires_at": mute.ExpiresAt.Format(time.RFC3339),
		"reason":     mute.Reason,
	})
	if err != nil {
		slog.Error("failed to marshal mute notification", "error", err)
	} else {
		h.hub.SendToUser(userID, mutePayload)
	}

	JSON(w, http.StatusCreated, map[string]interface{}{
		"id":         mute.ID,
		"user_id":    mute.UserID,
		"muted_by":   mute.MutedBy,
		"reason":     mute.Reason,
		"expires_at": mute.ExpiresAt,
		"created_at": mute.CreatedAt,
	})
}

// HandleUnmuteUser handles DELETE /api/v1/admin/chat/users/{id}/mute.
func (h *ChatAdminHandler) HandleUnmuteUser(w http.ResponseWriter, r *http.Request) {
	actorID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}
	perms := middleware.PermissionsFromContext(r.Context())

	userID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || userID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid user ID")
		return
	}

	if err := h.chatSvc.UnmuteUser(r.Context(), userID, actorID, perms); err != nil {
		if errors.Is(err, service.ErrForbidden) {
			ErrorResponse(w, http.StatusForbidden, "forbidden", "you do not have permission")
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to unmute user")
		return
	}

	// Notify the unmuted user's WebSocket client(s) in real time.
	unmutePayload, err := json.Marshal(map[string]interface{}{
		"type": "unmute",
	})
	if err != nil {
		slog.Error("failed to marshal unmute notification", "error", err)
	} else {
		h.hub.SendToUser(userID, unmutePayload)
	}

	w.WriteHeader(http.StatusNoContent)
}
