package handler

import (
	"net/http"
	"strconv"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
)

// HandleListNotificationsGrouped handles GET /api/v1/notifications/grouped.
// It accepts the same page/per_page/unread_only query params as
// HandleListNotifications, but folds same-topic topic_reply notifications
// into a single entry (count + last few actors + the underlying
// notifications, for expand-on-click). Kept as a separate endpoint rather
// than a flag on the existing one so /notifications' response shape and
// handler are untouched by this story.
func (h *NotificationHandler) HandleListNotificationsGrouped(w http.ResponseWriter, r *http.Request) {
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

	groups, total, err := h.notifSvc.ListGrouped(r.Context(), userID, page, perPage, unreadOnly)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list notifications")
		return
	}

	items := make([]map[string]interface{}, len(groups))
	for i, g := range groups {
		notifs := make([]map[string]interface{}, len(g.Notifications))
		for j, n := range g.Notifications {
			notifs[j] = map[string]interface{}{
				"id":         n.ID,
				"type":       n.Type,
				"data":       n.Data,
				"read":       n.Read,
				"created_at": n.CreatedAt,
			}
		}
		items[i] = map[string]interface{}{
			"key":               g.Key,
			"type":              g.Type,
			"count":             g.Count,
			"unread":            g.Unread,
			"last_actors":       g.LastActors,
			"latest_created_at": g.LatestCreatedAt,
			"data":              g.Data,
			"notifications":     notifs,
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"groups":   items,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}
