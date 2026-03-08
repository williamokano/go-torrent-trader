package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// ChatHandler handles chat REST endpoints.
type ChatHandler struct {
	chatSvc *service.ChatService
	hub     *ChatHub
}

// NewChatHandler creates a new ChatHandler.
func NewChatHandler(chatSvc *service.ChatService, hub *ChatHub) *ChatHandler {
	return &ChatHandler{chatSvc: chatSvc, hub: hub}
}

// HandleHistory handles GET /api/v1/chat/history?before_id=&limit=.
func (h *ChatHandler) HandleHistory(w http.ResponseWriter, r *http.Request) {
	beforeID, _ := strconv.ParseInt(r.URL.Query().Get("before_id"), 10, 64)
	if beforeID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "before_id is required and must be positive")
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	msgs, err := h.chatSvc.ListHistory(r.Context(), beforeID, limit)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list chat history")
		return
	}

	items := make([]map[string]interface{}, len(msgs))
	for i := range msgs {
		items[i] = chatMessagePayload(&msgs[i])
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"messages": items,
	})
}

// HandleDelete handles DELETE /api/v1/chat/{id}.
func (h *ChatHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
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

	if err := h.chatSvc.DeleteMessage(r.Context(), msgID, userID, perms); err != nil {
		switch {
		case errors.Is(err, service.ErrForbidden):
			ErrorResponse(w, http.StatusForbidden, "forbidden", "you do not have permission to delete this message")
		case errors.Is(err, service.ErrChatMessageNotFound):
			ErrorResponse(w, http.StatusNotFound, "not_found", "chat message not found")
		default:
			ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to delete chat message")
		}
		return
	}

	// Broadcast deletion to all WebSocket clients.
	h.hub.BroadcastDelete(msgID)

	w.WriteHeader(http.StatusNoContent)
}
