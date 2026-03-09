package handler

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// HandleStats returns site-wide statistics (public endpoint).
func HandleStats(cache *service.StatsCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats, err := cache.Get(r.Context())
		if err != nil {
			if r.Context().Err() == context.Canceled {
				// Client disconnected — not a real error
				return
			}
			slog.Error("failed to query site stats", "error", err)
			ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to load site statistics")
			return
		}

		JSON(w, http.StatusOK, map[string]interface{}{"stats": stats})
	}
}
