package service_test

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// The Redis session store is the persistence behind multi-device sessions
// (BE-1.2). These run against miniredis, so they exercise the real pipelines,
// the per-user token set, and TTL behaviour rather than a mock.

func newRedisSessions(t *testing.T) (*service.RedisSessionStore, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	store := service.NewRedisSessionStore(rdb, time.Hour, 24*time.Hour)
	return store, mr
}

func newSession(userID int64, access, refresh string) *service.Session {
	return &service.Session{
		UserID:       userID,
		AccessToken:  access,
		RefreshToken: refresh,
		DeviceName:   "test-device",
	}
}

func TestRedisSessionCreateAndLookup(t *testing.T) {
	store, _ := newRedisSessions(t)

	if err := store.Create(newSession(1, "acc-1", "ref-1")); err != nil {
		t.Fatalf("Create: %v", err)
	}

	byAccess := store.GetByAccessToken("acc-1")
	if byAccess == nil || byAccess.UserID != 1 {
		t.Fatalf("GetByAccessToken = %v, want the session for user 1", byAccess)
	}

	byRefresh := store.GetByRefreshToken("ref-1")
	if byRefresh == nil || byRefresh.AccessToken != "acc-1" {
		t.Fatalf("GetByRefreshToken = %v, want the session with access acc-1", byRefresh)
	}
}

func TestRedisSessionLookupMissingReturnsNil(t *testing.T) {
	store, _ := newRedisSessions(t)

	if store.GetByAccessToken("nope") != nil {
		t.Error("GetByAccessToken returned a session for an unknown token")
	}
	if store.GetByRefreshToken("nope") != nil {
		t.Error("GetByRefreshToken returned a session for an unknown token")
	}
}

// An expired access key must read as "no session" — this is the store relying
// on Redis TTL rather than checking timestamps itself.
func TestRedisSessionAccessTokenExpires(t *testing.T) {
	store, mr := newRedisSessions(t)

	if err := store.Create(newSession(1, "acc-1", "ref-1")); err != nil {
		t.Fatalf("Create: %v", err)
	}

	mr.FastForward(2 * time.Hour) // past the 1h access TTL

	if store.GetByAccessToken("acc-1") != nil {
		t.Error("access token is still valid after its TTL elapsed")
	}
	// The refresh token has a longer TTL and must still resolve, which is what
	// lets a client refresh an expired access token.
	if store.GetByRefreshToken("ref-1") == nil {
		t.Error("refresh token expired too early — a client could not refresh")
	}
}

func TestRedisSessionDelete(t *testing.T) {
	store, mr := newRedisSessions(t)

	if err := store.Create(newSession(1, "acc-1", "ref-1")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	store.Delete("acc-1")

	if store.GetByAccessToken("acc-1") != nil {
		t.Error("session still present after Delete")
	}
	if store.GetByRefreshToken("ref-1") != nil {
		t.Error("refresh token survived Delete")
	}
	// The token must also be removed from the user's session set, or a later
	// DeleteByUserID would iterate a dangling entry.
	if members, _ := mr.SMembers("session:user:1"); len(members) != 0 {
		t.Errorf("user session set still holds %v after Delete", members)
	}
}

// Deleting an unknown token must be a safe no-op, not a panic.
func TestRedisSessionDeleteUnknownIsNoop(t *testing.T) {
	store, _ := newRedisSessions(t)
	store.Delete("does-not-exist") // must not panic
}

// Rotate is the refresh flow: the old tokens die, a new pair is issued, and the
// user's session set tracks the swap.
func TestRedisSessionRotate(t *testing.T) {
	store, _ := newRedisSessions(t)

	if err := store.Create(newSession(1, "acc-old", "ref-old")); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := store.Rotate("ref-old", newSession(1, "acc-new", "ref-new")); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	if store.GetByAccessToken("acc-old") != nil {
		t.Error("old access token still valid after Rotate")
	}
	if store.GetByRefreshToken("ref-old") != nil {
		t.Error("old refresh token still valid after Rotate")
	}
	if store.GetByAccessToken("acc-new") == nil {
		t.Error("new access token not usable after Rotate")
	}
}

// Rotating a refresh token that no longer exists must still create the new
// session — a double-submit of the refresh request should not be fatal.
func TestRedisSessionRotateUnknownOldTokenStillCreates(t *testing.T) {
	store, _ := newRedisSessions(t)

	if err := store.Rotate("ref-gone", newSession(1, "acc-new", "ref-new")); err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if store.GetByAccessToken("acc-new") == nil {
		t.Error("new session was not created when the old refresh token was absent")
	}
}

// DeleteByUserID is "log out everywhere": every device's session goes.
func TestRedisSessionDeleteByUserID(t *testing.T) {
	store, mr := newRedisSessions(t)

	for _, s := range []*service.Session{
		newSession(1, "acc-a", "ref-a"),
		newSession(1, "acc-b", "ref-b"),
		newSession(2, "acc-c", "ref-c"), // a different user must be untouched
	} {
		if err := store.Create(s); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	store.DeleteByUserID(1)

	if store.GetByAccessToken("acc-a") != nil || store.GetByAccessToken("acc-b") != nil {
		t.Error("user 1 still has live sessions after DeleteByUserID")
	}
	if store.GetByRefreshToken("ref-a") != nil {
		t.Error("user 1's refresh token survived DeleteByUserID")
	}
	if store.GetByAccessToken("acc-c") == nil {
		t.Error("user 2's session was deleted — DeleteByUserID must be scoped to one user")
	}
	if members, _ := mr.SMembers("session:user:1"); len(members) != 0 {
		t.Errorf("user 1 session set still holds %v", members)
	}
}

// DeleteByUserIDExcept is "log out my other devices" — the current session must
// survive while every other one for that user dies.
func TestRedisSessionDeleteByUserIDExceptKeepsCurrent(t *testing.T) {
	store, _ := newRedisSessions(t)

	for _, s := range []*service.Session{
		newSession(1, "acc-keep", "ref-keep"),
		newSession(1, "acc-drop1", "ref-drop1"),
		newSession(1, "acc-drop2", "ref-drop2"),
	} {
		if err := store.Create(s); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	store.DeleteByUserIDExcept(1, "acc-keep")

	if store.GetByAccessToken("acc-keep") == nil {
		t.Error("the kept session was deleted")
	}
	if store.GetByAccessToken("acc-drop1") != nil || store.GetByAccessToken("acc-drop2") != nil {
		t.Error("other sessions survived DeleteByUserIDExcept")
	}
}

// TouchLastActive updates the timestamp without resetting the TTL — a busy
// session must not become immortal, nor be logged out early.
func TestRedisSessionTouchLastActivePreservesTTL(t *testing.T) {
	store, mr := newRedisSessions(t)

	if err := store.Create(newSession(1, "acc-1", "ref-1")); err != nil {
		t.Fatalf("Create: %v", err)
	}

	mr.FastForward(30 * time.Minute) // half the access TTL
	before := store.GetByAccessToken("acc-1").LastActive

	store.TouchLastActive("acc-1")

	sess := store.GetByAccessToken("acc-1")
	if sess == nil {
		t.Fatal("session gone after TouchLastActive")
	}
	if !sess.LastActive.After(before) {
		t.Error("LastActive was not advanced")
	}

	// The remaining TTL should be roughly the original minus the elapsed 30m —
	// nowhere near a full reset to 1h.
	ttl := mr.TTL("session:access:acc-1")
	if ttl > 35*time.Minute {
		t.Errorf("TTL = %v after touch, want it preserved near 30m, not reset to the full hour", ttl)
	}
}

// Touching an unknown token is a no-op.
func TestRedisSessionTouchUnknownIsNoop(t *testing.T) {
	store, _ := newRedisSessions(t)
	store.TouchLastActive("nope") // must not panic
}
