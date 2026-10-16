package handler

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// StatsReader reads the site-wide stats; *service.StatsService in production.
type StatsReader interface {
	Get(ctx context.Context) (*service.SiteStats, error)
}

// HandleStats returns site-wide statistics. Requires authentication — this is
// a private tracker with no anonymous browsing (see docs/NOT_PORTING.md §9).
func HandleStats(stats StatsReader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteStats, err := stats.Get(r.Context())
		if err != nil {
			if r.Context().Err() == context.Canceled {
				// Client disconnected — not a real error
				return
			}
			slog.Error("failed to query site stats", "error", err)
			ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to load site statistics")
			return
		}

		JSON(w, http.StatusOK, map[string]interface{}{"stats": siteStats})
	}
}
