package handler

import (
	"database/sql"
	"log/slog"
	"net/http"
)

// HandleStats returns site-wide statistics (public endpoint).
func HandleStats(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var stats struct {
			Users    int64 `json:"users"`
			Torrents int64 `json:"torrents"`
			Peers    int64 `json:"peers"`
			Seeders  int64 `json:"seeders"`
			Leechers int64 `json:"leechers"`
		}

		err := db.QueryRowContext(r.Context(), `
			SELECT
				(SELECT COUNT(*) FROM users WHERE enabled = true),
				(SELECT COUNT(*) FROM torrents WHERE visible = true AND banned = false),
				(SELECT COUNT(*) FROM peers),
				(SELECT COUNT(*) FROM peers WHERE seeder = true),
				(SELECT COUNT(*) FROM peers WHERE seeder = false)
		`).Scan(&stats.Users, &stats.Torrents, &stats.Peers, &stats.Seeders, &stats.Leechers)
		if err != nil {
			slog.Error("failed to query site stats", "error", err)
			ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to load site statistics")
			return
		}

		JSON(w, http.StatusOK, map[string]interface{}{"stats": stats})
	}
}
