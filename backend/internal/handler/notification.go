package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// NotificationHandler handles notification HTTP endpoints.
type NotificationHandler struct {
	notifSvc *service.NotificationService
}

// NewNotificationHandler creates a new NotificationHandler.
func NewNotificationHandler(notifSvc *service.NotificationService) *NotificationHandler {
	return &NotificationHandler{notifSvc: notifSvc}
}

// HandleListNotifications handles GET /api/v1/notifications.
func (h *NotificationHandler) HandleListNotifications(w http.ResponseWriter, r *http.Request) {
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
	unreadOnly := r.URL.Query().Get("unread_only") == "true"

	notifications, total, err := h.notifSvc.List(r.Context(), userID, page, perPage, unreadOnly)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list notifications")
		return
	}

	items := make([]map[string]interface{}, len(notifications))
	for i, n := range notifications {
		items[i] = map[string]interface{}{
			"id":         n.ID,
			"type":       n.Type,
			"data":       n.Data,
			"read":       n.Read,
			"created_at": n.CreatedAt,
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"notifications": items,
		"total":         total,
		"page":          page,
		"per_page":      perPage,
	})
}

// HandleUnreadCount handles GET /api/v1/notifications/unread-count.
func (h *NotificationHandler) HandleUnreadCount(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	count, err := h.notifSvc.UnreadCount(r.Context(), userID)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to count unread notifications")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"count": count,
	})
}

// HandleMarkRead handles PUT /api/v1/notifications/{id}/read.
func (h *NotificationHandler) HandleMarkRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	notifID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || notifID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid notification ID")
		return
	}

	if err := h.notifSvc.MarkRead(r.Context(), userID, notifID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			ErrorResponse(w, http.StatusNotFound, "not_found", "notification not found")
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to mark notification as read")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleMarkAllRead handles PUT /api/v1/notifications/read-all.
func (h *NotificationHandler) HandleMarkAllRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	if err := h.notifSvc.MarkAllRead(r.Context(), userID); err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to mark all notifications as read")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetPreferences handles GET /api/v1/notifications/preferences.
func (h *NotificationHandler) HandleGetPreferences(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	prefs, err := h.notifSvc.GetPreferences(r.Context(), userID)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to get notification preferences")
		return
	}

	items := make([]map[string]interface{}, len(prefs))
	for i, p := range prefs {
		items[i] = map[string]interface{}{
			"notification_type": p.NotificationType,
			"enabled":           p.Enabled,
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"preferences": items,
	})
}

// HandleUpdatePreferences handles PUT /api/v1/notifications/preferences.
func (h *NotificationHandler) HandleUpdatePreferences(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	var body struct {
		NotificationType string `json:"notification_type"`
		Enabled          *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	if body.NotificationType == "" {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "notification_type is required")
		return
	}
	if body.Enabled == nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "enabled is required")
		return
	}

	if err := h.notifSvc.SetPreference(r.Context(), userID, body.NotificationType, *body.Enabled); err != nil {
		if errors.Is(err, service.ErrInvalidNotification) {
			ErrorResponse(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to update preference")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
