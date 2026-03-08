package handler

import (
	"net/http"
	"strconv"

	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// ActivityLogHandler handles activity log HTTP endpoints.
type ActivityLogHandler struct {
	logs *service.ActivityLogService
}

// NewActivityLogHandler creates a new ActivityLogHandler.
func NewActivityLogHandler(logs *service.ActivityLogService) *ActivityLogHandler {
	return &ActivityLogHandler{logs: logs}
}

// HandleList handles GET /api/v1/activity-logs.
func (h *ActivityLogHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	opts := repository.ListActivityLogsOptions{}

	if eventType := r.URL.Query().Get("event_type"); eventType != "" {
		opts.EventType = &eventType
	}
	if actorStr := r.URL.Query().Get("actor_id"); actorStr != "" {
		if aid, err := strconv.ParseInt(actorStr, 10, 64); err == nil {
			opts.ActorID = &aid
		}
	}
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		opts.Page, _ = strconv.Atoi(pageStr)
	}
	if ppStr := r.URL.Query().Get("per_page"); ppStr != "" {
		opts.PerPage, _ = strconv.Atoi(ppStr)
	}

	logs, total, err := h.logs.List(r.Context(), opts)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list activity logs")
		return
	}

	items := make([]map[string]interface{}, len(logs))
	for i, l := range logs {
		items[i] = map[string]interface{}{
			"id":         l.ID,
			"event_type": l.EventType,
			"actor_id":   l.ActorID,
			"message":    l.Message,
			"created_at": l.CreatedAt,
		}
		if l.Metadata != nil {
			items[i]["metadata"] = l.Metadata
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"logs":     items,
		"total":    total,
		"page":     opts.Page,
		"per_page": opts.PerPage,
	})
}
