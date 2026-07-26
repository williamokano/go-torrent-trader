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
//   session:user:{userID}    -> Redis Set of *refresh* tokens for that user (no TTL, cleaned on delete)
//
// The user set is indexed by refresh token, not access token, and that is
// load-bearing. The access key carries the 1-hour TTL and the refresh key the
// 30-day one, so indexing by access token made a session unreachable the moment its
// access token expired — while its refresh key stayed alive and fully usable at
// /auth/refresh for the rest of the month. That is what #231 was: no way to revoke,
// and "log out all devices" silently skipping exactly the sessions most in need of
// it. Indexing by the long-lived half keeps every session reachable for as long as
// it can actually be used, and the session JSON stored under the refresh key
// carries AccessToken, so the short-lived key is always derivable from it.

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

// NewRedisSessionStore creates a Redis-backed session store using the provided
// Redis client. The caller is responsible for closing the client.
func NewRedisSessionStore(client *redis.Client, accessTokenTTL, refreshTokenTTL time.Duration) *RedisSessionStore {
	return &RedisSessionStore{
		client:          client,
		accessTokenTTL:  accessTokenTTL,
		refreshTokenTTL: refreshTokenTTL,
	}
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
	pipe.SAdd(ctx, userKey(session.UserID), session.RefreshToken)

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

	r.deleteSession(ctx, sess)
}

// DeleteByRefreshToken removes a session identified by its refresh token.
//
// The revocation path that works once the access token has expired, which is the
// window the access-token-only path could not reach. A caller holding the refresh
// token can already mint access tokens from it, so letting it revoke the session is
// strictly a reduction in what that token can do.
func (r *RedisSessionStore) DeleteByRefreshToken(refreshToken string) {
	sess := r.GetByRefreshToken(refreshToken)
	if sess == nil {
		return
	}
	r.deleteSession(context.Background(), sess)
}

// deleteSession removes both keys and the user-set entry for one session.
func (r *RedisSessionStore) deleteSession(ctx context.Context, sess *Session) {
	pipe := r.client.Pipeline()
	pipe.Del(ctx, keyPrefixAccess+sess.AccessToken)
	pipe.Del(ctx, keyPrefixRefresh+sess.RefreshToken)
	pipe.SRem(ctx, userKey(sess.UserID), sess.RefreshToken)
	// Transitional: sets written before this changed hold access tokens. Removing
	// both spellings drains them without a migration step.
	pipe.SRem(ctx, userKey(sess.UserID), sess.AccessToken)

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
		pipe.SRem(ctx, userKey(oldSess.UserID), oldRefreshToken)
		pipe.SRem(ctx, userKey(oldSess.UserID), oldSess.AccessToken) // transitional, see Create
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
//
// Members are refresh tokens (see Create). Resolving each through the refresh key is
// what makes "log out all devices" actually log out all devices: reading the session
// through the *access* key meant a session whose access token had expired resolved to
// nothing, so its refresh key was never deleted and the set member was dropped —
// removing the only pointer to a credential that stayed valid for weeks.
func (r *RedisSessionStore) deleteUserSessions(userID int64, keepAccessToken string) {
	ctx := context.Background()
	uKey := userKey(userID)

	members, err := r.client.SMembers(ctx, uKey).Result()
	if err != nil {
		slog.Error("redis: failed to get user sessions", "user_id", userID, "error", err)
		return
	}

	// The caller names the session to keep by access token, so resolve it to the
	// refresh token the set is indexed by.
	keepRefreshToken := ""
	if keepAccessToken != "" {
		if keep := r.GetByAccessToken(keepAccessToken); keep != nil {
			keepRefreshToken = keep.RefreshToken
		}
	}

	pipe := r.client.Pipeline()
	for _, member := range members {
		if member == keepAccessToken || (keepRefreshToken != "" && member == keepRefreshToken) {
			continue
		}

		// A member is a refresh token. Sets written before that changed hold access
		// tokens, so fall back to reading it as one — that drains the old spelling
		// instead of leaving it behind forever.
		if sess := r.GetByRefreshToken(member); sess != nil {
			pipe.Del(ctx, keyPrefixRefresh+member)
			pipe.Del(ctx, keyPrefixAccess+sess.AccessToken)
		} else if sess := r.GetByAccessToken(member); sess != nil {
			pipe.Del(ctx, keyPrefixAccess+member)
			pipe.Del(ctx, keyPrefixRefresh+sess.RefreshToken)
		} else {
			// Neither key resolves: both have expired, so there is nothing left to
			// revoke and only the set entry to tidy up. Delete both spellings
			// blindly, since an unresolvable member gives no way to derive the other.
			pipe.Del(ctx, keyPrefixRefresh+member)
			pipe.Del(ctx, keyPrefixAccess+member)
		}
		pipe.SRem(ctx, uKey, member)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		slog.Error("redis: failed to delete user sessions", "user_id", userID, "error", err)
	}
}

func userKey(userID int64) string {
	return fmt.Sprintf("%s%d", keyPrefixUser, userID)
}
