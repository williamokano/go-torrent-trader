package handler

import (
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// DashboardStats holds the aggregated admin dashboard data.
type DashboardStats struct {
	Users          UserStats              `json:"users"`
	Torrents       TorrentStats           `json:"torrents"`
	Peers          PeerStats              `json:"peers"`
	PendingReports int64                  `json:"pending_reports"`
	ActiveWarnings int64                  `json:"active_warnings"`
	ActiveMutes    int64                  `json:"active_mutes"`
	RecentActivity []map[string]interface{} `json:"recent_activity"`
}

// UserStats holds user-related dashboard counts.
type UserStats struct {
	Total   int64 `json:"total"`
	Today   int64 `json:"today"`
	Week    int64 `json:"week"`
}

// TorrentStats holds torrent-related dashboard counts.
type TorrentStats struct {
	Total int64 `json:"total"`
	Today int64 `json:"today"`
}

// PeerStats holds peer-related dashboard counts.
type PeerStats struct {
	Total    int64 `json:"total"`
	Seeders  int64 `json:"seeders"`
	Leechers int64 `json:"leechers"`
}

// HandleDashboard returns GET /api/v1/admin/dashboard.
func HandleDashboard(db *sql.DB, activityLogs *service.ActivityLogService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		now := time.Now().UTC()
		todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		weekStart := todayStart.AddDate(0, 0, -int(todayStart.Weekday()))

		var stats DashboardStats

		// Single query for user, torrent, and peer counts
		err := db.QueryRowContext(ctx, `
			SELECT
				(SELECT COUNT(*) FROM users WHERE enabled = true),
				(SELECT COUNT(*) FROM users WHERE enabled = true AND created_at >= $1),
				(SELECT COUNT(*) FROM users WHERE enabled = true AND created_at >= $2),
				(SELECT COUNT(*) FROM torrents WHERE visible = true AND banned = false),
				(SELECT COUNT(*) FROM torrents WHERE visible = true AND banned = false AND created_at >= $1),
				(SELECT COUNT(*) FROM peers),
				(SELECT COUNT(*) FROM peers WHERE seeder = true),
				(SELECT COUNT(*) FROM peers WHERE seeder = false),
				(SELECT COUNT(*) FROM reports WHERE resolved = false),
				(SELECT COUNT(*) FROM warnings WHERE status = 'active'),
				(SELECT COUNT(*) FROM chat_mutes WHERE (expires_at IS NULL OR expires_at > NOW()))
		`, todayStart, weekStart).Scan(
			&stats.Users.Total, &stats.Users.Today, &stats.Users.Week,
			&stats.Torrents.Total, &stats.Torrents.Today,
			&stats.Peers.Total, &stats.Peers.Seeders, &stats.Peers.Leechers,
			&stats.PendingReports, &stats.ActiveWarnings, &stats.ActiveMutes,
		)
		if err != nil {
			slog.Error("dashboard: failed to query stats", "error", err)
			ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to load dashboard stats")
			return
		}

		// Fetch last 10 activity log entries
		logs, _, err := activityLogs.List(ctx, repository.ListActivityLogsOptions{
			Page:    1,
			PerPage: 10,
		})
		if err != nil {
			slog.Error("dashboard: failed to query activity logs", "error", err)
			ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to load dashboard stats")
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
		stats.RecentActivity = items

		JSON(w, http.StatusOK, stats)
	}
}
