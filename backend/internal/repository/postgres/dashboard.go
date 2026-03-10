package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// DashboardRepo implements repository.DashboardRepository using PostgreSQL.
type DashboardRepo struct {
	db *sql.DB
}

// NewDashboardRepo returns a new PostgreSQL-backed DashboardRepository.
func NewDashboardRepo(db *sql.DB) repository.DashboardRepository {
	return &DashboardRepo{db: db}
}

func (r *DashboardRepo) GetStats(ctx context.Context) (*repository.DashboardStats, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	now := time.Now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	weekStart := todayStart.AddDate(0, 0, -int(todayStart.Weekday()))

	var s repository.DashboardStats
	err := r.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM users WHERE enabled = true),
			(SELECT COUNT(*) FROM users WHERE enabled = true AND created_at >= $1),
			(SELECT COUNT(*) FROM users WHERE enabled = true AND created_at >= $2),
			(SELECT COUNT(*) FROM torrents WHERE visible = true AND banned = false),
			(SELECT COUNT(*) FROM torrents WHERE visible = true AND banned = false AND created_at >= $1),
			(SELECT COUNT(*) FROM peers),
			(SELECT COUNT(*) FILTER (WHERE seeder = true) FROM peers),
			(SELECT COUNT(*) FILTER (WHERE seeder = false) FROM peers),
			(SELECT COUNT(*) FROM reports WHERE resolved = false),
			(SELECT COUNT(*) FROM warnings WHERE status = 'active'),
			(SELECT COUNT(*) FROM chat_mutes WHERE (expires_at IS NULL OR expires_at > NOW()))
	`, todayStart, weekStart).Scan(
		&s.UsersTotal, &s.UsersToday, &s.UsersWeek,
		&s.TorrentsTotal, &s.TorrentsToday,
		&s.PeersTotal, &s.PeersSeeders, &s.PeersLeechers,
		&s.PendingReports, &s.ActiveWarnings, &s.ActiveMutes,
	)
	if err != nil {
		return nil, fmt.Errorf("dashboard stats: %w", err)
	}
	return &s, nil
}
