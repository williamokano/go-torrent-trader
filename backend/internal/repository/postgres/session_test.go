package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

func newSessionStore(t *testing.T) (*SessionStore, int64) {
	t.Helper()
	db := requireDB(t)
	resetTestData(t, db)
	return NewSessionStore(db), seedUser(t, db, "sessionuser")
}

// testSession builds a session like AuthService does: an access token that
// expires after accessTTL and a refresh token after refreshTTL.
func testSession(userID int64, access, refresh string, accessTTL, refreshTTL time.Duration) *service.Session {
	now := time.Now()
	return &service.Session{
		UserID:           userID,
		GroupID:          3,
		Permissions:      model.Permissions{GroupID: 3, Username: "sessionuser", CanUpload: true, IsModerator: true},
		AccessToken:      access,
		RefreshToken:     refresh,
		DeviceName:       "laptop",
		IP:               "203.0.113.7",
		CreatedAt:        now,
		LastActive:       now,
		ExpiresAt:        now.Add(accessTTL),
		RefreshExpiresAt: now.Add(refreshTTL),
	}
}

func TestSessionStore_CreateAndLookUpByEitherToken(t *testing.T) {
	store, userID := newSessionStore(t)
	sess := testSession(userID, "access-1", "refresh-1", time.Hour, 30*24*time.Hour)
	if err := store.Create(sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	byAccess := sessionOf(store.GetByAccessToken("access-1"))
	if byAccess == nil {
		t.Fatal("GetByAccessToken found nothing")
	}
	if byAccess.UserID != userID || byAccess.GroupID != 3 || byAccess.DeviceName != "laptop" ||
		byAccess.IP != "203.0.113.7" || byAccess.AccessToken != "access-1" {
		t.Errorf("GetByAccessToken = %+v, want the stored session", byAccess)
	}
	if byAccess.Permissions != sess.Permissions {
		t.Errorf("permissions = %+v, want %+v", byAccess.Permissions, sess.Permissions)
	}

	byRefresh := sessionOf(store.GetByRefreshToken("refresh-1"))
	if byRefresh == nil || byRefresh.UserID != userID || byRefresh.RefreshToken != "refresh-1" {
		t.Fatalf("GetByRefreshToken = %+v, want the stored session", byRefresh)
	}

	if sessionOf(store.GetByAccessToken("refresh-1")) != nil || sessionOf(store.GetByRefreshToken("access-1")) != nil {
		t.Error("a token was accepted as the other kind")
	}
	if sessionOf(store.GetByAccessToken("nope")) != nil {
		t.Error("an unknown token resolved to a session")
	}
}

// The two halves expire separately: an hour-old access token is dead, while
// the month-long refresh token still mints a new pair.
func TestSessionStore_ExpiredAccessTokenLiveRefreshToken(t *testing.T) {
	store, userID := newSessionStore(t)
	if err := store.Create(testSession(userID, "access-old", "refresh-live", -time.Minute, time.Hour)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sessionOf(store.GetByAccessToken("access-old")) != nil {
		t.Error("an expired access token still resolves")
	}
	if sessionOf(store.GetByRefreshToken("refresh-live")) == nil {
		t.Error("a live refresh token stopped resolving when its access token expired")
	}
}

// A database read, or a restored backup, must not hand out working tokens.
func TestSessionStore_StoresNoRawToken(t *testing.T) {
	store, userID := newSessionStore(t)
	if err := store.Create(testSession(userID, "rawaccesstoken", "rawrefreshtoken", time.Hour, time.Hour)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	rows, err := testDB.Query(`SELECT * FROM sessions`)
	if err != nil {
		t.Fatalf("reading sessions: %v", err)
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatalf("columns: %v", err)
	}
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatalf("scan: %v", err)
		}
		for i, v := range values {
			var b []byte
			switch x := v.(type) {
			case []byte:
				b = x
			case string:
				b = []byte(x)
			default:
				continue
			}
			for _, token := range []string{"rawaccesstoken", "rawrefreshtoken"} {
				if bytes.Contains(b, []byte(token)) {
					t.Errorf("column %s holds the raw token %q", cols[i], token)
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
}

func TestSessionStore_Rotate(t *testing.T) {
	store, userID := newSessionStore(t)
	if err := store.Create(testSession(userID, "access-a", "refresh-a", time.Hour, time.Hour)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Rotate("refresh-a", testSession(userID, "access-b", "refresh-b", time.Hour, time.Hour)); err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if sessionOf(store.GetByAccessToken("access-a")) != nil || sessionOf(store.GetByRefreshToken("refresh-a")) != nil {
		t.Error("the rotated-out token pair still works")
	}
	if sessionOf(store.GetByAccessToken("access-b")) == nil || sessionOf(store.GetByRefreshToken("refresh-b")) == nil {
		t.Error("the new token pair does not work")
	}

	// A refresh token is single-use: rotating from it again creates nothing.
	err := store.Rotate("refresh-a", testSession(userID, "access-c", "refresh-c", time.Hour, time.Hour))
	if !errors.Is(err, service.ErrInvalidToken) {
		t.Fatalf("second rotation from the same token returned %v, want ErrInvalidToken", err)
	}
	if sessionOf(store.GetByAccessToken("access-c")) != nil {
		t.Error("a reused refresh token minted a new session")
	}
}

// Two refreshes racing with one token: exactly one may win. Under READ
// COMMITTED the loser's DELETE waits on the winner's row lock, then finds
// nothing to delete — which is what Rotate's row-count check turns into a
// refusal instead of a second token pair.
func TestSessionStore_ConcurrentRotationHasOneWinner(t *testing.T) {
	store, userID := newSessionStore(t)
	if err := store.Create(testSession(userID, "race-a", "race-r", time.Hour, time.Hour)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	const racers = 8
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		wins int
	)
	start := make(chan struct{})
	for i := range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			next := testSession(userID, fmt.Sprintf("race-a-%d", i), fmt.Sprintf("race-r-%d", i), time.Hour, time.Hour)
			err := store.Rotate("race-r", next)
			switch {
			case err == nil:
				mu.Lock()
				wins++
				mu.Unlock()
			case !errors.Is(err, service.ErrInvalidToken):
				t.Errorf("racer %d: %v", i, err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if wins != 1 {
		t.Fatalf("%d racers rotated the same refresh token, want exactly 1", wins)
	}
	var live int
	if err := testDB.QueryRow(`SELECT count(*) FROM sessions WHERE user_id = $1`, userID).Scan(&live); err != nil {
		t.Fatalf("counting sessions: %v", err)
	}
	if live != 1 {
		t.Fatalf("%d sessions after the race, want 1", live)
	}
}

// "Log out all devices" landing between a refresh's lookup and its Rotate
// must win: the refresh may not bring the session back.
func TestSessionStore_RotateCannotResurrectARevokedSession(t *testing.T) {
	store, userID := newSessionStore(t)
	if err := store.Create(testSession(userID, "rev-a", "rev-r", time.Hour, time.Hour)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sessionOf(store.GetByRefreshToken("rev-r")) == nil {
		t.Fatal("setup: session not found")
	}
	store.DeleteByUserID(userID)

	err := store.Rotate("rev-r", testSession(userID, "rev-a2", "rev-r2", time.Hour, time.Hour))
	if !errors.Is(err, service.ErrInvalidToken) {
		t.Fatalf("Rotate after revocation returned %v, want ErrInvalidToken", err)
	}
	if sessionOf(store.GetByAccessToken("rev-a2")) != nil {
		t.Error("a revoked session was brought back by a refresh in flight")
	}
}

// A store that cannot answer must say so: (nil, nil) would read as "no such
// session" and become a 401, and the frontend logs members out on a 401.
func TestSessionStore_ReportsStoreFailureAsAnError(t *testing.T) {
	requireDB(t)
	db, err := sql.Open("pgx", "postgres://nobody@127.0.0.1:1/none?connect_timeout=1")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := NewSessionStore(db)

	for name, lookup := range map[string]func(string) (*service.Session, error){
		"access":  store.GetByAccessToken,
		"refresh": store.GetByRefreshToken,
	} {
		sess, err := lookup("any")
		if sess != nil || !errors.Is(err, service.ErrSessionStoreUnavailable) {
			t.Errorf("%s lookup on a dead database = (%v, %v), want ErrSessionStoreUnavailable", name, sess, err)
		}
	}
}

// #231: once the access token has expired, the refresh token is the only handle
// on a session that can still be refreshed for weeks. Revocation must work
// through it, and "log out all devices" must reach it.
func TestSessionStore_RevokesSessionsWhoseAccessTokenExpired(t *testing.T) {
	store, userID := newSessionStore(t)
	if err := store.Create(testSession(userID, "a1", "r1", -time.Minute, time.Hour)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	store.DeleteByRefreshToken("r1")
	if sessionOf(store.GetByRefreshToken("r1")) != nil {
		t.Error("DeleteByRefreshToken left an expired-access session refreshable")
	}

	if err := store.Create(testSession(userID, "a2", "r2", -time.Minute, time.Hour)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	store.DeleteByUserID(userID)
	if sessionOf(store.GetByRefreshToken("r2")) != nil {
		t.Error("DeleteByUserID missed a session whose access token had expired")
	}
}

func TestSessionStore_DeleteAndDeleteAllExcept(t *testing.T) {
	store, userID := newSessionStore(t)
	other := seedUser(t, testDB, "someoneelse")
	for _, s := range []*service.Session{
		testSession(userID, "keep-a", "keep-r", time.Hour, time.Hour),
		testSession(userID, "drop-a", "drop-r", time.Hour, time.Hour),
		testSession(userID, "old-a", "old-r", -time.Minute, time.Hour),
		testSession(other, "other-a", "other-r", time.Hour, time.Hour),
	} {
		if err := store.Create(s); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	store.DeleteByUserIDExcept(userID, "keep-a")
	if sessionOf(store.GetByAccessToken("keep-a")) == nil {
		t.Error("the kept session was revoked")
	}
	if sessionOf(store.GetByAccessToken("drop-a")) != nil || sessionOf(store.GetByRefreshToken("old-r")) != nil {
		t.Error("another session of the user survived")
	}
	if sessionOf(store.GetByAccessToken("other-a")) == nil {
		t.Error("another user's session was revoked")
	}

	store.Delete("keep-a")
	if sessionOf(store.GetByRefreshToken("keep-r")) != nil {
		t.Error("Delete left the session refreshable")
	}
}

func TestSessionStore_TouchLastActiveIsThrottled(t *testing.T) {
	store, userID := newSessionStore(t)
	sess := testSession(userID, "touch-a", "touch-r", time.Hour, time.Hour)
	sess.LastActive = time.Now().Add(-10 * time.Minute)
	if err := store.Create(sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	store.TouchLastActive("touch-a")
	first := sessionOf(store.GetByAccessToken("touch-a")).LastActive
	if time.Since(first) > time.Minute {
		t.Fatalf("last active = %v, want it moved to now", first)
	}

	// Inside the interval a second touch writes nothing.
	store.TouchLastActive("touch-a")
	if second := sessionOf(store.GetByAccessToken("touch-a")).LastActive; !second.Equal(first) {
		t.Errorf("a touch inside the throttle interval wrote again: %v then %v", first, second)
	}
}

func TestSessionStore_PurgeExpired(t *testing.T) {
	store, userID := newSessionStore(t)
	if err := store.Create(testSession(userID, "dead-a", "dead-r", -2*time.Hour, -time.Hour)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Create(testSession(userID, "live-a", "live-r", -time.Hour, time.Hour)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	n, err := store.PurgeExpired(context.Background())
	if err != nil {
		t.Fatalf("PurgeExpired: %v", err)
	}
	if n != 1 {
		t.Errorf("purged %d sessions, want 1", n)
	}
	if sessionOf(store.GetByRefreshToken("live-r")) == nil {
		t.Error("a session with a live refresh token was purged")
	}
}

// Deleting the user takes their sessions with them.
func TestSessionStore_SessionsGoWithTheirUser(t *testing.T) {
	store, userID := newSessionStore(t)
	if err := store.Create(testSession(userID, "cascade-a", "cascade-r", time.Hour, time.Hour)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := testDB.Exec(`DELETE FROM users WHERE id = $1`, userID); err != nil {
		t.Fatalf("deleting user: %v", err)
	}
	if sessionOf(store.GetByAccessToken("cascade-a")) != nil {
		t.Error("a deleted user's session still resolves")
	}
}

// sessionOf unwraps a lookup for assertions about whether a session came back;
// a store error fails the test loudly instead of reading as "no session".
func sessionOf(sess *service.Session, err error) *service.Session {
	if err != nil {
		panic(fmt.Sprintf("session lookup failed: %v", err))
	}
	return sess
}
