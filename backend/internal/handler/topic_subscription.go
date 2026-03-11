package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// TopicSubscriptionHandler handles topic subscription HTTP endpoints.
type TopicSubscriptionHandler struct {
	notifSvc *service.NotificationService
}

// NewTopicSubscriptionHandler creates a new TopicSubscriptionHandler.
func NewTopicSubscriptionHandler(notifSvc *service.NotificationService) *TopicSubscriptionHandler {
	return &TopicSubscriptionHandler{notifSvc: notifSvc}
}

// HandleSubscribe handles POST /api/v1/forums/topics/{id}/subscribe.
func (h *TopicSubscriptionHandler) HandleSubscribe(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	topicID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || topicID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid topic ID")
		return
	}

	perms := middleware.PermissionsFromContext(r.Context())
	if err := h.notifSvc.SubscribeWithAccessCheck(r.Context(), userID, topicID, perms); err != nil {
		switch {
		case errors.Is(err, service.ErrTopicNotFound):
			ErrorResponse(w, http.StatusNotFound, "not_found", "topic not found")
		case errors.Is(err, service.ErrForbidden):
			ErrorResponse(w, http.StatusForbidden, "forbidden", "insufficient forum access")
		default:
			ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to subscribe")
		}
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"subscribed": true,
	})
}

// HandleUnsubscribe handles DELETE /api/v1/forums/topics/{id}/subscribe.
func (h *TopicSubscriptionHandler) HandleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	topicID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || topicID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid topic ID")
		return
	}

	if err := h.notifSvc.Unsubscribe(r.Context(), userID, topicID); err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to unsubscribe")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"subscribed": false,
	})
}

// HandleGetSubscription handles GET /api/v1/forums/topics/{id}/subscription.
func (h *TopicSubscriptionHandler) HandleGetSubscription(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	topicID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || topicID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid topic ID")
		return
	}

	subscribed, err := h.notifSvc.IsSubscribed(r.Context(), userID, topicID)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to check subscription")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"subscribed": subscribed,
	})
}
