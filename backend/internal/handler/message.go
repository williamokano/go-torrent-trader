package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// MessageHandler handles private message HTTP endpoints.
type MessageHandler struct {
	messageSvc *service.MessageService
}

// NewMessageHandler creates a new MessageHandler.
func NewMessageHandler(messageSvc *service.MessageService) *MessageHandler {
	return &MessageHandler{messageSvc: messageSvc}
}

// HandleSendMessage handles POST /api/v1/messages.
func (h *MessageHandler) HandleSendMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	var body struct {
		ReceiverID int64  `json:"receiver_id"`
		Subject    string `json:"subject"`
		Body       string `json:"body"`
		ParentID   *int64 `json:"parent_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	msg, err := h.messageSvc.SendMessage(r.Context(), userID, service.SendMessageRequest{
		ReceiverID: body.ReceiverID,
		Subject:    body.Subject,
		Body:       body.Body,
		ParentID:   body.ParentID,
	})
	if err != nil {
		handleMessageError(w, err)
		return
	}

	JSON(w, http.StatusCreated, map[string]interface{}{
		"message": messageResponse(msg),
	})
}

// HandleListInbox handles GET /api/v1/messages/inbox.
func (h *MessageHandler) HandleListInbox(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	page := 1
	perPage := 25
	if p := r.URL.Query().Get("page"); p != "" {
		page, _ = strconv.Atoi(p)
	}
	if pp := r.URL.Query().Get("per_page"); pp != "" {
		perPage, _ = strconv.Atoi(pp)
	}

	messages, total, err := h.messageSvc.ListInbox(r.Context(), userID, page, perPage)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list inbox")
		return
	}

	items := make([]map[string]interface{}, len(messages))
	for i := range messages {
		items[i] = messageResponse(&messages[i])
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"messages": items,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

// HandleListOutbox handles GET /api/v1/messages/outbox.
func (h *MessageHandler) HandleListOutbox(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	page := 1
	perPage := 25
	if p := r.URL.Query().Get("page"); p != "" {
		page, _ = strconv.Atoi(p)
	}
	if pp := r.URL.Query().Get("per_page"); pp != "" {
		perPage, _ = strconv.Atoi(pp)
	}

	messages, total, err := h.messageSvc.ListOutbox(r.Context(), userID, page, perPage)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list outbox")
		return
	}

	items := make([]map[string]interface{}, len(messages))
	for i := range messages {
		items[i] = messageResponse(&messages[i])
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"messages": items,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

// HandleGetMessage handles GET /api/v1/messages/{id}.
func (h *MessageHandler) HandleGetMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	msgID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || msgID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid message ID")
		return
	}

	msg, err := h.messageSvc.GetMessage(r.Context(), msgID, userID)
	if err != nil {
		handleMessageError(w, err)
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"message": messageResponse(msg),
	})
}

// HandleDeleteMessage handles DELETE /api/v1/messages/{id}.
func (h *MessageHandler) HandleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	msgID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || msgID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid message ID")
		return
	}

	if err := h.messageSvc.DeleteMessage(r.Context(), msgID, userID); err != nil {
		handleMessageError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleUnreadCount handles GET /api/v1/messages/unread-count.
func (h *MessageHandler) HandleUnreadCount(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	count, err := h.messageSvc.CountUnread(r.Context(), userID)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to count unread messages")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"unread_count": count,
	})
}

func handleMessageError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrMessageNotFound):
		ErrorResponse(w, http.StatusNotFound, "not_found", "message not found")
	case errors.Is(err, service.ErrInvalidMessage):
		ErrorResponse(w, http.StatusBadRequest, "bad_request", err.Error())
	case errors.Is(err, service.ErrCannotMessageSelf):
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "cannot send message to yourself")
	default:
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "an unexpected error occurred")
	}
}

func messageResponse(m *model.Message) map[string]interface{} {
	resp := map[string]interface{}{
		"id":                m.ID,
		"sender_id":         m.SenderID,
		"sender_username":   m.SenderUsername,
		"receiver_id":       m.ReceiverID,
		"receiver_username": m.ReceiverUsername,
		"subject":           m.Subject,
		"body":              m.Body,
		"is_read":           m.IsRead,
		"created_at":        m.CreatedAt,
	}
	if m.ParentID != nil {
		resp["parent_id"] = *m.ParentID
	}
	return resp
}
