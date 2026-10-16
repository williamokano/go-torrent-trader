package service

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// statsTTL is how long one reading of the site stats is served. The query
// scans the whole peers table — about 30ms at a million peers — and every
// member's header asks for it, so without this the cost would grow with how
// many tabs are open rather than staying at one query per node per TTL.
const statsTTL = 30 * time.Second

// SiteStats holds the site-wide statistics returned by the stats endpoint.
type SiteStats struct {
	Users       int64 `json:"users"`
	Torrents    int64 `json:"torrents"`
	Peers       int64 `json:"peers"`
	Seeders     int64 `json:"seeders"`
	Leechers    int64 `json:"leechers"`
	OnlineUsers int64 `json:"online_users"`
}

// statsQueryTimeout bounds one refresh. The refresh runs detached from the
// request that triggered it (see Get), so this is its only limit.
const statsQueryTimeout = 5 * time.Second

// StatsService reads the site stats, remembering the last reading for
// statsTTL in this process. It replaced a Redis cache (#289): a per-node copy
// means each node runs the query once per TTL instead of once overall, which
// is nothing next to every member's request running it.
type StatsService struct {
	db  *sql.DB
	now func() time.Time

	refresh singleflight.Group

	mu      sync.Mutex
	cached  *SiteStats
	expires time.Time
}

// NewStatsService returns a stats reader over db.
func NewStatsService(db *sql.DB) *StatsService {
	return &StatsService{db: db, now: time.Now}
}

// Get returns the site stats, from the last reading if it is still fresh.
//
// When the reading has expired, concurrent callers share one refresh
// (singleflight) instead of each starting the same scan of peers. The refresh
// runs on its own context, so the request that happened to start it can go
// away without cancelling it for everyone waiting; each caller still waits
// only as long as its own context allows.
//
// If the refresh fails, the last reading is served, however old: the numbers
// in a header are better slightly stale than missing while the database has a
// bad minute. An error comes back only when there has never been a reading.
func (s *StatsService) Get(ctx context.Context) (*SiteStats, error) {
	if stats, fresh := s.reading(); fresh {
		return stats, nil
	}

	result := s.refresh.DoChan("stats", func() (any, error) {
		queryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), statsQueryTimeout)
		defer cancel()
		stats, err := s.query(queryCtx)
		if err != nil {
			return nil, err
		}
		s.mu.Lock()
		s.cached = stats
		s.expires = s.now().Add(statsTTL)
		s.mu.Unlock()
		return nil, nil
	})

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-result:
		stats, _ := s.reading()
		if res.Err != nil {
			if stats != nil {
				slog.Warn("site stats: refresh failed, serving the last reading", "error", res.Err)
				return stats, nil
			}
			return nil, res.Err
		}
		return stats, nil
	}
}

// reading returns a copy of the last reading, if any, and whether it is still
// fresh. A copy, so one caller adjusting its stats cannot change the next
// caller's.
func (s *StatsService) reading() (*SiteStats, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cached == nil {
		return nil, false
	}
	stats := *s.cached
	return &stats, s.now().Before(s.expires)
}

// query reads the stats. peers is scanned once, with the seeder split taken in
// the same pass: counting it three times was most of the query's cost.
func (s *StatsService) query(ctx context.Context) (*SiteStats, error) {
	var stats SiteStats
	err := s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM users WHERE enabled = true),
			(SELECT COUNT(*) FROM torrents WHERE visible = true AND banned = false),
			p.total,
			p.seeders,
			(SELECT COUNT(*) FROM users WHERE enabled = true AND last_access > NOW() - INTERVAL '15 minutes')
		FROM (SELECT COUNT(*) AS total, COUNT(*) FILTER (WHERE seeder) AS seeders FROM peers) p
	`).Scan(&stats.Users, &stats.Torrents, &stats.Peers, &stats.Seeders, &stats.OnlineUsers)
	if err != nil {
		return nil, fmt.Errorf("query site stats: %w", err)
	}
	stats.Leechers = stats.Peers - stats.Seeders
	return &stats, nil
}
