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
	if byAccess.UserID != userID || byAccess.DeviceName != "laptop" ||
		byAccess.IP != "203.0.113.7" || byAccess.AccessToken != "access-1" {
		t.Errorf("GetByAccessToken = %+v, want the stored session", byAccess)
	}
	// Authority comes from the account, not from what the session was created
	// with: testSession claims group 3 with moderator rights, the seeded user's
	// group says otherwise.
	if want := livePermissions(t, userID); byAccess.Permissions != want || byAccess.GroupID != want.GroupID {
		t.Errorf("permissions = %+v (group %d), want the account's current %+v",
			byAccess.Permissions, byAccess.GroupID, want)
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

// livePermissions is what a session for userID should carry right now: the
// permissions of the user's current group, with their current username.
func livePermissions(t *testing.T, userID int64) model.Permissions {
	t.Helper()
	var (
		groupID  int64
		username string
	)
	if err := testDB.QueryRow(`SELECT group_id, username FROM users WHERE id = $1`, userID).
		Scan(&groupID, &username); err != nil {
		t.Fatalf("reading user: %v", err)
	}
	group, err := NewGroupRepo(testDB).GetByID(context.Background(), groupID)
	if err != nil {
		t.Fatalf("reading group: %v", err)
	}
	perms := model.PermissionsFromGroup(group)
	perms.Username = username
	return perms
}

func groupBySlug(t *testing.T, slug string) int64 {
	t.Helper()
	var id int64
	if err := testDB.QueryRow(`SELECT id FROM groups WHERE slug = $1`, slug).Scan(&id); err != nil {
		t.Fatalf("group %q: %v", slug, err)
	}
	return id
}

// #291: a ban takes effect on the member's very next request, through either
// token, whichever code path disabled the account — the admin edit, the
// warning ladder, or a direct UPDATE as here.
func TestSessionStore_DisabledAccountHasNoLiveSession(t *testing.T) {
	store, userID := newSessionStore(t)
	if err := store.Create(testSession(userID, "ban-a", "ban-r", time.Hour, 30*24*time.Hour)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := testDB.Exec(`UPDATE users SET enabled = false WHERE id = $1`, userID); err != nil {
		t.Fatalf("disabling user: %v", err)
	}

	if sessionOf(store.GetByAccessToken("ban-a")) != nil {
		t.Error("a disabled member's access token still works")
	}
	if sessionOf(store.GetByRefreshToken("ban-r")) != nil {
		t.Error("a disabled member can still refresh")
	}
}

// #291: a demotion takes effect at once. The old store copied the login-time
// snapshot forward on every refresh, so a demoted admin stayed one for as long
// as their client kept refreshing.
func TestSessionStore_PermissionsFollowTheMembersCurrentGroup(t *testing.T) {
	store, userID := newSessionStore(t)
	admin := groupBySlug(t, "administrator")
	if _, err := testDB.Exec(`UPDATE users SET group_id = $1 WHERE id = $2`, admin, userID); err != nil {
		t.Fatalf("promoting user: %v", err)
	}
	if err := store.Create(testSession(userID, "demote-a", "demote-r", time.Hour, time.Hour)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !sessionOf(store.GetByAccessToken("demote-a")).Permissions.IsAdmin {
		t.Fatal("setup: the admin's session is not an admin's")
	}

	user := groupBySlug(t, "user")
	if _, err := testDB.Exec(`UPDATE users SET group_id = $1 WHERE id = $2`, user, userID); err != nil {
		t.Fatalf("demoting user: %v", err)
	}
	for name, sess := range map[string]*service.Session{
		"access":  sessionOf(store.GetByAccessToken("demote-a")),
		"refresh": sessionOf(store.GetByRefreshToken("demote-r")),
	} {
		if sess == nil {
			t.Fatalf("%s: the demoted member's session is gone; it should stay, with less authority", name)
		}
		if sess.Permissions.IsAdmin || sess.GroupID != user {
			t.Errorf("%s: permissions after demotion = %+v (group %d), want the user class", name, sess.Permissions, sess.GroupID)
		}
	}
}

// Editing a class reaches every member in it at once, and so does a rename.
func TestSessionStore_PermissionsFollowEditsToTheClassAndAccount(t *testing.T) {
	store, userID := newSessionStore(t)
	if err := store.Create(testSession(userID, "edit-a", "edit-r", time.Hour, time.Hour)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	before := sessionOf(store.GetByAccessToken("edit-a")).Permissions

	if _, err := testDB.Exec(`UPDATE groups SET can_upload = NOT can_upload
		WHERE id = (SELECT group_id FROM users WHERE id = $1)`, userID); err != nil {
		t.Fatalf("editing the class: %v", err)
	}
	if _, err := testDB.Exec(`UPDATE users SET username = 'renamed' WHERE id = $1`, userID); err != nil {
		t.Fatalf("renaming: %v", err)
	}

	after := sessionOf(store.GetByAccessToken("edit-a")).Permissions
	if after.CanUpload == before.CanUpload {
		t.Error("an edit to the member's class did not reach their session")
	}
	if after.Username != "renamed" {
		t.Errorf("username = %q after a rename, want renamed", after.Username)
	}
}

// Every permission column is read into its own field. The seeded classes set
// most flags alike, so a swapped pair in the SELECT or the Scan would pass the
// other tests; this class alternates them, so it would not.
func TestSessionStore_ReadsEveryPermissionFromItsOwnColumn(t *testing.T) {
	store, userID := newSessionStore(t)
	var groupID int64
	if err := testDB.QueryRow(`INSERT INTO groups
		(name, slug, level, can_upload, can_download, can_invite, can_comment, can_forum,
		 is_admin, is_moderator, is_immune, can_self_approve)
		VALUES ('Checkerboard', 'checkerboard', 37, true, false, true, false, true,
		        false, true, false, true)
		RETURNING id`).Scan(&groupID); err != nil {
		t.Fatalf("creating class: %v", err)
	}
	if _, err := testDB.Exec(`UPDATE users SET group_id = $1 WHERE id = $2`, groupID, userID); err != nil {
		t.Fatalf("moving user: %v", err)
	}
	if err := store.Create(testSession(userID, "check-a", "check-r", time.Hour, time.Hour)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	want := model.Permissions{
		GroupID: groupID, GroupName: "Checkerboard", Username: "sessionuser", Level: 37,
		CanUpload: true, CanDownload: false, CanInvite: true, CanComment: false, CanForum: true,
		IsAdmin: false, IsModerator: true, IsImmune: false, CanSelfApprove: true,
	}
	for name, sess := range map[string]*service.Session{
		"access":  sessionOf(store.GetByAccessToken("check-a")),
		"refresh": sessionOf(store.GetByRefreshToken("check-r")),
	} {
		if sess == nil {
			t.Fatalf("%s: no session", name)
		}
		if sess.Permissions != want || sess.GroupID != groupID {
			t.Errorf("%s: permissions = %+v (group %d), want %+v", name, sess.Permissions, sess.GroupID, want)
		}
	}
}
