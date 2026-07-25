package handler

import (
	"log/slog"
	"net/http"

	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// AnnounceFeedsHandler serves the list of live feeds a member can subscribe to.
type AnnounceFeedsHandler struct {
	connectors *service.ConnectorService
}

// NewAnnounceFeedsHandler creates the handler.
func NewAnnounceFeedsHandler(connectors *service.ConnectorService) *AnnounceFeedsHandler {
	return &AnnounceFeedsHandler{connectors: connectors}
}

// HandleList handles GET /api/v1/announce-feeds.
//
// It returns only a slug and a name per enabled feed — enough to render a picker
// and build a stream URL, and nothing about how the feed is configured. The
// admin list is the place for that, and it is behind RequireAdmin.
func (h *AnnounceFeedsHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	feeds, err := h.connectors.PublicFeeds(r.Context())
	if err != nil {
		slog.Error("failed to list live feeds", "error", err)
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to load live feeds")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{"feeds": feeds})
}
