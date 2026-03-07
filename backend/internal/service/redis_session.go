package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis key patterns:
//   session:access:{token}   -> JSON-encoded Session (TTL = AccessTokenTTL)
//   session:refresh:{token}  -> JSON-encoded Session (TTL = RefreshTokenTTL)
//   session:user:{userID}    -> Redis Set of access tokens for that user (no TTL, cleaned on delete)

const (
	keyPrefixAccess  = "session:access:"
	keyPrefixRefresh = "session:refresh:"
	keyPrefixUser    = "session:user:"
)

// RedisSessionStore implements SessionStore backed by Redis.
type RedisSessionStore struct {
	client          *redis.Client
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
}

// NewRedisSessionStore creates a Redis-backed session store.
func NewRedisSessionStore(redisURL string, accessTokenTTL, refreshTokenTTL time.Duration) (*RedisSessionStore, error) {
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

	return &RedisSessionStore{
		client:          client,
		accessTokenTTL:  accessTokenTTL,
		refreshTokenTTL: refreshTokenTTL,
	}, nil
}

// Close closes the underlying Redis client.
func (r *RedisSessionStore) Close() error {
	return r.client.Close()
}

// Create stores a new session in Redis.
func (r *RedisSessionStore) Create(session *Session) error {
	ctx := context.Background()
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	pipe := r.client.Pipeline()
	pipe.Set(ctx, keyPrefixAccess+session.AccessToken, data, r.accessTokenTTL)
	pipe.Set(ctx, keyPrefixRefresh+session.RefreshToken, data, r.refreshTokenTTL)
	pipe.SAdd(ctx, userKey(session.UserID), session.AccessToken)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis create session: %w", err)
	}

	return nil
}

// GetByAccessToken retrieves a session by its access token.
// Returns nil if not found or expired (Redis handles TTL expiry).
func (r *RedisSessionStore) GetByAccessToken(token string) *Session {
	ctx := context.Background()
	data, err := r.client.Get(ctx, keyPrefixAccess+token).Bytes()
	if err != nil {
		return nil
	}

	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		slog.Error("failed to unmarshal session from redis", "error", err)
		return nil
	}

	return &sess
}

// GetByRefreshToken retrieves a session by its refresh token.
// Returns nil if not found or expired.
func (r *RedisSessionStore) GetByRefreshToken(token string) *Session {
	ctx := context.Background()
	data, err := r.client.Get(ctx, keyPrefixRefresh+token).Bytes()
	if err != nil {
		return nil
	}

	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		slog.Error("failed to unmarshal session from redis", "error", err)
		return nil
	}

	return &sess
}

// Delete removes a session by access token.
func (r *RedisSessionStore) Delete(accessToken string) {
	ctx := context.Background()

	// Fetch session first to get refresh token and user ID for cleanup.
	sess := r.GetByAccessToken(accessToken)
	if sess == nil {
		return
	}

	pipe := r.client.Pipeline()
	pipe.Del(ctx, keyPrefixAccess+accessToken)
	pipe.Del(ctx, keyPrefixRefresh+sess.RefreshToken)
	pipe.SRem(ctx, userKey(sess.UserID), accessToken)

	if _, err := pipe.Exec(ctx); err != nil {
		slog.Error("redis delete session failed", "error", err)
	}
}

// Rotate invalidates the old session and creates a new one with fresh tokens.
func (r *RedisSessionStore) Rotate(oldRefreshToken string, newSession *Session) error {
	ctx := context.Background()

	// Remove old session keys.
	oldSess := r.GetByRefreshToken(oldRefreshToken)
	if oldSess != nil {
		pipe := r.client.Pipeline()
		pipe.Del(ctx, keyPrefixAccess+oldSess.AccessToken)
		pipe.Del(ctx, keyPrefixRefresh+oldRefreshToken)
		pipe.SRem(ctx, userKey(oldSess.UserID), oldSess.AccessToken)
		if _, err := pipe.Exec(ctx); err != nil {
			slog.Error("redis rotate: failed to remove old session", "error", err)
		}
	}

	return r.Create(newSession)
}

// DeleteByUserID removes all sessions for a given user ID.
func (r *RedisSessionStore) DeleteByUserID(userID int64) {
	r.deleteUserSessions(userID, "")
}

// DeleteByUserIDExcept removes all sessions for a given user ID except the one
// matching the provided access token.
func (r *RedisSessionStore) DeleteByUserIDExcept(userID int64, keepAccessToken string) {
	r.deleteUserSessions(userID, keepAccessToken)
}

// TouchLastActive updates the session's LastActive timestamp in Redis.
func (r *RedisSessionStore) TouchLastActive(accessToken string) {
	ctx := context.Background()
	data, err := r.client.Get(ctx, keyPrefixAccess+accessToken).Bytes()
	if err != nil {
		return
	}

	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return
	}

	sess.LastActive = time.Now()
	updated, err := json.Marshal(&sess)
	if err != nil {
		return
	}

	// Preserve the remaining TTL on the key.
	ttl := r.client.TTL(ctx, keyPrefixAccess+accessToken).Val()
	if ttl <= 0 {
		ttl = r.accessTokenTTL
	}
	if err := r.client.Set(ctx, keyPrefixAccess+accessToken, updated, ttl).Err(); err != nil {
		slog.Error("redis touch last active failed", "error", err)
	}
}

// deleteUserSessions removes all sessions for a user, optionally keeping one.
func (r *RedisSessionStore) deleteUserSessions(userID int64, keepAccessToken string) {
	ctx := context.Background()
	uKey := userKey(userID)

	accessTokens, err := r.client.SMembers(ctx, uKey).Result()
	if err != nil {
		slog.Error("redis: failed to get user sessions", "user_id", userID, "error", err)
		return
	}

	pipe := r.client.Pipeline()
	for _, at := range accessTokens {
		if at == keepAccessToken {
			continue
		}

		// Look up the session to find the refresh token.
		data, err := r.client.Get(ctx, keyPrefixAccess+at).Bytes()
		if err == nil {
			var sess Session
			if err := json.Unmarshal(data, &sess); err == nil {
				pipe.Del(ctx, keyPrefixRefresh+sess.RefreshToken)
			}
		}

		pipe.Del(ctx, keyPrefixAccess+at)
		pipe.SRem(ctx, uKey, at)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		slog.Error("redis: failed to delete user sessions", "user_id", userID, "error", err)
	}
}

func userKey(userID int64) string {
	return fmt.Sprintf("%s%d", keyPrefixUser, userID)
}
