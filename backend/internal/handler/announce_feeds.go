package handler

import (
	"log/slog"
	"net/http"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// AnnounceFeedsHandler serves the list of live feeds a member can subscribe to.
type AnnounceFeedsHandler struct {
	connectors *service.ConnectorService
	access     FeedAccess
}

// NewAnnounceFeedsHandler creates the handler. A nil access check refuses
// everyone, for the same reason the stream does: an unwired gate must not be an
// open one.
func NewAnnounceFeedsHandler(connectors *service.ConnectorService, access FeedAccess) *AnnounceFeedsHandler {
	return &AnnounceFeedsHandler{connectors: connectors, access: access}
}

// HandleList handles GET /api/v1/announce-feeds.
//
// It returns only a slug and a name per enabled feed — enough to render a picker
// and build a stream URL, and nothing about how the feed is configured. The
// admin list is the place for that, and it is behind RequireAdmin.
func (h *AnnounceFeedsHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	// Gated the same way the streams are: listing the feeds to someone who
	// cannot open any of them would offer a page that only ever fails.
	{
		if h.access == nil {
			slog.Error("live feeds: no access check wired, refusing")
			ErrorResponse(w, http.StatusForbidden, "forbidden", "you do not have access to the live feeds")
			return
		}
		userID, ok := middleware.UserIDFromContext(r.Context())
		if !ok {
			// The route is behind auth, so this means the middleware chain
			// changed under it. Refusing beats listing to an unknown caller.
			ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		allowed, err := h.access.Allowed(r.Context(), userID)
		if err != nil {
			slog.Error("failed to read feed access", "error", err)
			ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to load live feeds")
			return
		}
		if !allowed {
			ErrorResponse(w, http.StatusForbidden, "forbidden", "you do not have access to the live feeds")
			return
		}
	}

	feeds, err := h.connectors.PublicFeeds(r.Context())
	if err != nil {
		slog.Error("failed to list live feeds", "error", err)
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to load live feeds")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{"feeds": feeds})
}
