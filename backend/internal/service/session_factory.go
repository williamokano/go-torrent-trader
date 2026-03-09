package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// NewSessionStore creates a SessionStore based on the provided configuration.
// Returns a RedisSessionStore for type "redis".
func NewSessionStore(cfg SessionStoreConfig) (SessionStore, error) {
	switch cfg.Type {
	case "redis":
		return newRedisSessionStoreFromURL(cfg.RedisURL, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	default:
		return nil, fmt.Errorf("unknown session store type %q: must be \"redis\"", cfg.Type)
	}
}

// NewSessionStoreWithClient creates a SessionStore using a pre-existing Redis
// client. This avoids creating a duplicate connection when the caller already
// has a shared client.
func NewSessionStoreWithClient(client *redis.Client, accessTokenTTL, refreshTokenTTL time.Duration) SessionStore {
	return NewRedisSessionStore(client, accessTokenTTL, refreshTokenTTL)
}

// newRedisSessionStoreFromURL creates a Redis client from a URL, pings it, and
// returns a RedisSessionStore. Used by the factory when no shared client exists.
func newRedisSessionStoreFromURL(redisURL string, accessTokenTTL, refreshTokenTTL time.Duration) (*RedisSessionStore, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		if closeErr := client.Close(); closeErr != nil {
			slog.Error("failed to close redis client after ping failure", "error", closeErr)
		}
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	return NewRedisSessionStore(client, accessTokenTTL, refreshTokenTTL), nil
}
