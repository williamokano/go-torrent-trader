package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
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
	// Staff-only entries (backups, cheat flags, ban patterns, moderation
	// actions) and raw event metadata are restricted to staff viewers.
	isStaff := middleware.PermissionsFromContext(r.Context()).IsStaff()
	opts := repository.ListActivityLogsOptions{IncludeStaffOnly: isStaff}

	if eventType := r.URL.Query().Get("event_type"); eventType != "" {
		opts.EventType = &eventType
	}
	if actorStr := r.URL.Query().Get("actor_id"); actorStr != "" {
		if aid, err := strconv.ParseInt(actorStr, 10, 64); err == nil {
			opts.ActorID = &aid
		}
	}
	// Resolve the defaults here rather than passing 0s down and echoing them
	// back: the service silently substitutes page 1 / 25 per page, so the raw
	// values would tell the client `per_page: 0` for a response that actually
	// holds 25 rows — and any client computing total/per_page divides by zero.
	opts.Page, opts.PerPage = parsePagination(r)

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
		if isStaff && l.Metadata != nil {
			var raw json.RawMessage
			if err := json.Unmarshal([]byte(*l.Metadata), &raw); err == nil {
				items[i]["metadata"] = raw
			} else {
				items[i]["metadata"] = l.Metadata
			}
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"logs":     items,
		"total":    total,
		"page":     opts.Page,
		"per_page": opts.PerPage,
	})
}
