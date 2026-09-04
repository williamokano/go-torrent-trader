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

// ListByUserID is what makes the session list possible: the per-user set had
// always been maintained so sessions could be revoked together, and nothing had
// ever read it back (#171).
func TestRedisSessionListByUserID(t *testing.T) {
	store, _ := newRedisSessions(t)

	for _, s := range []*service.Session{
		newSession(1, "acc-a", "ref-a"),
		newSession(1, "acc-b", "ref-b"),
		newSession(2, "acc-c", "ref-c"), // another member's session must not appear
	} {
		if err := store.Create(s); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	got, err := store.ListByUserID(1)
	if err != nil {
		t.Fatalf("ListByUserID: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d sessions for user 1, want 2", len(got))
	}
	for _, sess := range got {
		if sess.UserID != 1 {
			t.Errorf("ListByUserID(1) returned a session belonging to user %d", sess.UserID)
		}
	}
}

// The session a member most needs to see is the one whose access token has
// already expired — it stays usable at /auth/refresh for another twenty-nine
// days, and it is invisible to anything that looks sessions up by access token
// (#231). Listing must resolve through the refresh key, which outlives it.
func TestRedisSessionListIncludesSessionsWhoseAccessTokenHasExpired(t *testing.T) {
	store, mr := newRedisSessions(t)

	if err := store.Create(newSession(1, "acc-1", "ref-1")); err != nil {
		t.Fatalf("Create: %v", err)
	}

	mr.FastForward(2 * time.Hour) // past the access TTL, well inside the refresh TTL
	if store.GetByAccessToken("acc-1") != nil {
		t.Fatal("fixture: the access token should have expired")
	}
	if store.GetByRefreshToken("ref-1") == nil {
		t.Fatal("fixture: the refresh token must still resolve, or this proves nothing")
	}

	got, err := store.ListByUserID(1)
	if err != nil {
		t.Fatalf("ListByUserID: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1 — a session that can still mint access tokens "+
			"has to be visible to the member who wants it gone", len(got))
	}
}

// A member of the set whose keys have both expired is a session that no longer
// exists. It must not appear as an empty row — and it must not stay in the set
// either: nothing else ever removes it, so one dead member accumulates per
// login, forever, and this read pays a round trip for each of them.
func TestRedisSessionListSkipsAndPrunesFullyExpiredSessions(t *testing.T) {
	store, mr := newRedisSessions(t)

	if err := store.Create(newSession(1, "acc-1", "ref-1")); err != nil {
		t.Fatalf("Create: %v", err)
	}

	mr.FastForward(48 * time.Hour) // past both TTLs in this fixture

	got, err := store.ListByUserID(1)
	if err != nil {
		t.Fatalf("ListByUserID: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d sessions, want 0 — the set entry outlives the keys it points at", len(got))
	}
	if members, _ := mr.SMembers("session:user:1"); len(members) != 0 {
		t.Errorf("the user set still holds %v, so it grows by one entry per expired "+
			"session and never shrinks", members)
	}
}

// The timestamp the panel shows as "active X ago" has to be the session's last
// activity. The session JSON lives under two keys and the listing reads the
// refresh one, so a touch that wrote only the access key left every row
// reporting when the session was created (#171 review).
func TestRedisSessionListReflectsTouchLastActive(t *testing.T) {
	store, _ := newRedisSessions(t)

	sess := newSession(1, "acc-1", "ref-1")
	sess.CreatedAt = time.Now().Add(-3 * time.Hour)
	sess.LastActive = sess.CreatedAt
	if err := store.Create(sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	store.TouchLastActive("acc-1")

	got, err := store.ListByUserID(1)
	if err != nil {
		t.Fatalf("ListByUserID: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
	if !got[0].LastActive.After(got[0].CreatedAt) {
		t.Errorf("listed LastActive = %v, same as CreatedAt (%v) — the list is showing "+
			"when the session was created, not when it was last used",
			got[0].LastActive, got[0].CreatedAt)
	}
}

// ...but a touch must never bring a revoked session back. Both keys are written
// with SETXX precisely so that a write cannot recreate a key revocation deleted.
func TestRedisSessionTouchDoesNotResurrectARevokedSession(t *testing.T) {
	store, _ := newRedisSessions(t)

	if err := store.Create(newSession(1, "acc-1", "ref-1")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	store.DeleteByRefreshToken("ref-1")

	store.TouchLastActive("acc-1") // the racing request that was already in flight

	if store.GetByRefreshToken("ref-1") != nil {
		t.Error("the refresh key came back after revocation, handing the member's " +
			"revoked session another twenty-nine days")
	}
	if store.GetByAccessToken("acc-1") != nil {
		t.Error("the access key came back after revocation")
	}
}

// A session stored before Session.ID existed carries an empty one, and stays
// usable for the rest of its thirty days. Against the real store, not only an
// in-memory double: the fallback id is derived from the refresh token, which is
// the field the Redis listing resolves by.
func TestRedisSessionListAddressesSessionsPredatingTheIDField(t *testing.T) {
	store, _ := newRedisSessions(t)

	legacy := newSession(1, "acc-legacy", "ref-legacy")
	legacy.ID = "" // as it unmarshals from a session written before the field
	if err := store.Create(legacy); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := store.ListByUserID(1)
	if err != nil {
		t.Fatalf("ListByUserID: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
	if id := service.SessionID(got[0]); id == "" {
		t.Error("a session with no id and no derived id cannot be revoked at all")
	}
}

// A user with no sessions is an empty list, not a nil-pointer panic.
func TestRedisSessionListForUnknownUserIsEmpty(t *testing.T) {
	store, _ := newRedisSessions(t)

	got, err := store.ListByUserID(999)
	if err != nil {
		t.Fatalf("ListByUserID: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d sessions for a user who has never logged in", len(got))
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
