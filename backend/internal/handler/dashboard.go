package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

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
func HandleDashboard(dashRepo repository.DashboardRepository, activityLogs *service.ActivityLogService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		dbStats, err := dashRepo.GetStats(ctx)
		if err != nil {
			slog.Error("dashboard: failed to query stats", "error", err)
			ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to load dashboard stats")
			return
		}

		stats := DashboardStats{
			Users: UserStats{
				Total: dbStats.UsersTotal,
				Today: dbStats.UsersToday,
				Week:  dbStats.UsersWeek,
			},
			Torrents: TorrentStats{
				Total: dbStats.TorrentsTotal,
				Today: dbStats.TorrentsToday,
			},
			Peers: PeerStats{
				Total:    dbStats.PeersTotal,
				Seeders:  dbStats.PeersSeeders,
				Leechers: dbStats.PeersLeechers,
			},
			PendingReports: dbStats.PendingReports,
			ActiveWarnings: dbStats.ActiveWarnings,
			ActiveMutes:    dbStats.ActiveMutes,
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
				// Parse the JSON string into json.RawMessage to avoid double-encoding.
				var raw json.RawMessage
				if err := json.Unmarshal([]byte(*l.Metadata), &raw); err == nil {
					items[i]["metadata"] = raw
				} else {
					items[i]["metadata"] = l.Metadata
				}
			}
		}
		stats.RecentActivity = items

		JSON(w, http.StatusOK, stats)
	}
}
