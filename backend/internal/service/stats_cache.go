package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

const statsCacheKey = "cache:site_stats"

// SiteStats holds the site-wide statistics returned by the stats endpoint.
type SiteStats struct {
	Users       int64 `json:"users"`
	Torrents    int64 `json:"torrents"`
	Peers       int64 `json:"peers"`
	Seeders     int64 `json:"seeders"`
	Leechers    int64 `json:"leechers"`
	OnlineUsers int64 `json:"online_users"`
}

// StatsCache wraps the site stats query with a Redis cache layer.
type StatsCache struct {
	db      *sql.DB
	redis   *redis.Client
	ttl     time.Duration // TTL for request-driven cache writes (Get)
	warmTTL time.Duration // TTL for background warm writes (Warm)
}

// NewStatsCache creates a StatsCache backed by the given Redis client and DB.
// The ttl controls how long request-driven cache entries live. Background
// warming via Warm() uses a longer TTL (1 hour) so the cache stays populated
// between hourly worker runs.
func NewStatsCache(db *sql.DB, redisClient *redis.Client, ttl time.Duration) *StatsCache {
	return &StatsCache{
		db:      db,
		redis:   redisClient,
		ttl:     ttl,
		warmTTL: 1 * time.Hour,
	}
}

// Get returns cached site stats from Redis. On a cache miss it queries the DB,
// stores the result in Redis with the configured TTL, and returns it.
func (c *StatsCache) Get(ctx context.Context) (*SiteStats, error) {
	// Try cache first.
	data, err := c.redis.Get(ctx, statsCacheKey).Bytes()
	if err == nil {
		var stats SiteStats
		if unmarshalErr := json.Unmarshal(data, &stats); unmarshalErr == nil {
			return &stats, nil
		}
		slog.Warn("stats cache: failed to unmarshal cached value, falling through to DB", "error", err)
	} else if !errors.Is(err, redis.Nil) {
		// Redis error (not a simple cache miss) — log and fall through to DB.
		slog.Warn("stats cache: redis error, falling back to DB", "error", err)
	}

	// Cache miss, unmarshal error, or Redis failure — query DB.
	stats, err := c.queryDB(ctx)
	if err != nil {
		return nil, err
	}

	// Store in cache (best-effort, don't fail the request on cache write errors).
	if cacheData, jsonErr := json.Marshal(stats); jsonErr == nil {
		if setErr := c.redis.Set(ctx, statsCacheKey, cacheData, c.ttl).Err(); setErr != nil {
			slog.Warn("stats cache: failed to write to Redis", "error", setErr)
		}
	}

	return stats, nil
}

// Warm pre-populates the cache by querying the DB and storing the result.
// This is intended to be called from background jobs (e.g. recalc_stats).
func (c *StatsCache) Warm(ctx context.Context) error {
	stats, err := c.queryDB(ctx)
	if err != nil {
		return fmt.Errorf("stats cache warm: %w", err)
	}

	data, err := json.Marshal(stats)
	if err != nil {
		return fmt.Errorf("stats cache warm marshal: %w", err)
	}

	if err := c.redis.Set(ctx, statsCacheKey, data, c.warmTTL).Err(); err != nil {
		return fmt.Errorf("stats cache warm redis set: %w", err)
	}

	return nil
}

func (c *StatsCache) queryDB(ctx context.Context) (*SiteStats, error) {
	var stats SiteStats
	err := c.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM users WHERE enabled = true),
			(SELECT COUNT(*) FROM torrents WHERE visible = true AND banned = false),
			(SELECT COUNT(*) FROM peers),
			(SELECT COUNT(*) FROM peers WHERE seeder = true),
			(SELECT COUNT(*) FROM peers WHERE seeder = false),
			(SELECT COUNT(*) FROM users WHERE enabled = true AND last_access > NOW() - INTERVAL '15 minutes')
	`).Scan(&stats.Users, &stats.Torrents, &stats.Peers, &stats.Seeders, &stats.Leechers, &stats.OnlineUsers)
	if err != nil {
		return nil, fmt.Errorf("query site stats: %w", err)
	}
	return &stats, nil
}
