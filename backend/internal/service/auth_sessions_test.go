package service

import (
	"fmt"
	"testing"
	"time"
)

// Unit-level cover for the three operations behind /auth/sessions (#171). The
// handler tests drive these through the router; these pin the parts a status
// code cannot show — the ordering of the list, which row is marked current, and
// that a member's reach stops at their own account.

// sessionsFor builds an AuthService over an in-memory store holding the given
// sessions. Nothing here touches the user repository, so the rest is nil.
func sessionsFor(t *testing.T, sessions ...*Session) (*AuthService, *memorySessionStore) {
	t.Helper()

	store := newTestSessionStore()
	for _, sess := range sessions {
		if err := store.Create(sess); err != nil {
			t.Fatalf("seeding session: %v", err)
		}
	}
	return NewAuthService(nil, store, nil, nil, "", nil), store
}

func testSession(userID int64, id, access, refresh string, lastActive time.Time) *Session {
	return &Session{
		ID:               id,
		UserID:           userID,
		AccessToken:      access,
		RefreshToken:     refresh,
		DeviceName:       "Firefox on Linux",
		IP:               "203.0.113.7",
		CreatedAt:        lastActive.Add(-time.Hour),
		LastActive:       lastActive,
		ExpiresAt:        time.Now().Add(time.Hour),
		RefreshExpiresAt: time.Now().Add(720 * time.Hour),
	}
}

func TestListSessionsOrdersByLastActiveAndMarksTheCaller(t *testing.T) {
	now := time.Now()
	auth, _ := sessionsFor(t,
		testSession(1, "stale", "acc-stale", "ref-stale", now.Add(-2*time.Hour)),
		testSession(1, "here", "acc-here", "ref-here", now),
		testSession(1, "recent", "acc-recent", "ref-recent", now.Add(-time.Minute)),
		testSession(2, "other", "acc-other", "ref-other", now),
	)

	got, err := auth.ListSessions(1, "acc-here")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("got %d sessions, want 3 — another member's session must not appear", len(got))
	}
	want := []string{"here", "recent", "stale"}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("position %d = %q, want %q (most recently active first)", i, got[i].ID, id)
		}
	}
	if !got[0].Current {
		t.Error("the caller's own session is not marked current, so the UI cannot label it")
	}
	for _, row := range got[1:] {
		if row.Current {
			t.Errorf("session %q is marked current but is not the caller's", row.ID)
		}
	}
}

// A caller with no usable access token — there is no such caller today, but the
// signature allows it — must not have some arbitrary row marked as theirs.
func TestListSessionsMarksNothingCurrentWithoutAnAccessToken(t *testing.T) {
	auth, _ := sessionsFor(t, testSession(1, "a", "acc-a", "ref-a", time.Now()))

	rows, err := auth.ListSessions(1, "")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	for _, row := range rows {
		if row.Current {
			t.Error("a row was marked current for a caller that named no session")
		}
	}
}

func TestListSessionsCarriesNoCredential(t *testing.T) {
	auth, _ := sessionsFor(t, testSession(1, "a", "acc-a", "ref-a", time.Now()))

	got, err := auth.ListSessions(1, "acc-a")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
	if got[0].ID == "acc-a" || got[0].ID == "ref-a" {
		t.Error("the session id is one of the session's tokens")
	}
}

// A session stored before Session.ID existed still has to be listable and
// revocable — it stays usable for up to thirty days after the upgrade.
func TestSessionsPredatingTheIDFieldAreStillAddressable(t *testing.T) {
	legacy := testSession(1, "", "acc-legacy", "ref-legacy", time.Now())
	auth, store := sessionsFor(t, legacy)

	got, err := auth.ListSessions(1, "")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
	if got[0].ID == "" {
		t.Fatal("a session with no id cannot be revoked")
	}

	if err := auth.RevokeSession(1, got[0].ID); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}
	if store.GetByRefreshToken("ref-legacy") != nil {
		t.Error("the session survived revocation")
	}
}

func TestRevokeSessionRemovesBothHalves(t *testing.T) {
	auth, store := sessionsFor(t,
		testSession(1, "target", "acc-target", "ref-target", time.Now()),
		testSession(1, "keep", "acc-keep", "ref-keep", time.Now()),
	)

	if err := auth.RevokeSession(1, "target"); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}

	if store.GetByAccessToken("acc-target") != nil {
		t.Error("the access token survived")
	}
	if store.GetByRefreshToken("ref-target") != nil {
		t.Error("the refresh token survived — the session can still mint access tokens")
	}
	if store.GetByAccessToken("acc-keep") == nil {
		t.Error("an unrelated session of the same member was revoked")
	}
}

func TestRevokeSessionRefusesAnotherMembersSession(t *testing.T) {
	auth, store := sessionsFor(t, testSession(2, "victim", "acc-victim", "ref-victim", time.Now()))

	if err := auth.RevokeSession(1, "victim"); err != ErrSessionNotFound {
		t.Errorf("err = %v, want ErrSessionNotFound", err)
	}
	if store.GetByAccessToken("acc-victim") == nil {
		t.Error("another member's session was revoked")
	}
}

func TestRevokeSessionOnAnUnknownOrEmptyIDIsNotFound(t *testing.T) {
	auth, _ := sessionsFor(t, testSession(1, "a", "acc-a", "ref-a", time.Now()))

	for _, id := range []string{"", "nosuchsession"} {
		if err := auth.RevokeSession(1, id); err != ErrSessionNotFound {
			t.Errorf("RevokeSession(%q) = %v, want ErrSessionNotFound", id, err)
		}
	}
}

func TestRevokeOtherSessionsKeepsTheCallerAndCountsTheRest(t *testing.T) {
	auth, store := sessionsFor(t,
		testSession(1, "here", "acc-here", "ref-here", time.Now()),
		testSession(1, "phone", "acc-phone", "ref-phone", time.Now()),
		testSession(1, "laptop", "acc-laptop", "ref-laptop", time.Now()),
		testSession(2, "stranger", "acc-stranger", "ref-stranger", time.Now()),
	)

	got, err := auth.RevokeOtherSessions(1, "acc-here")
	if err != nil {
		t.Fatalf("RevokeOtherSessions: %v", err)
	}
	if got != 2 {
		t.Errorf("revoked = %d, want 2", got)
	}
	if store.GetByAccessToken("acc-here") == nil {
		t.Error("the caller was signed out by their own panic button, leaving them " +
			"unable to change their password")
	}
	if store.GetByAccessToken("acc-phone") != nil || store.GetByAccessToken("acc-laptop") != nil {
		t.Error("another of the member's sessions survived")
	}
	if store.GetByAccessToken("acc-stranger") == nil {
		t.Error("a different member's session was revoked")
	}
}

// A store whose delete silently misses — which is what a session rotating at
// /auth/refresh in the window between the listing and the delete looks like —
// must not produce a 204 over a session that is still alive.
type missingDeleteStore struct {
	*memorySessionStore
	misses int // how many DeleteByRefreshToken calls to swallow
}

func (s *missingDeleteStore) DeleteByRefreshToken(refreshToken string) {
	if s.misses > 0 {
		s.misses--
		return
	}
	s.memorySessionStore.DeleteByRefreshToken(refreshToken)
}

func TestRevokeSessionRetriesWhenTheDeleteMisses(t *testing.T) {
	inner := newTestSessionStore()
	if err := inner.Create(testSession(1, "target", "acc-t", "ref-t", time.Now())); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	store := &missingDeleteStore{memorySessionStore: inner, misses: 1}
	auth := NewAuthService(nil, store, nil, nil, "", nil)

	if err := auth.RevokeSession(1, "target"); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}
	if inner.GetByRefreshToken("ref-t") != nil {
		t.Error("the session survived a revoke that reported success")
	}
}

func TestRevokeSessionReportsAFailureRatherThanA204(t *testing.T) {
	inner := newTestSessionStore()
	if err := inner.Create(testSession(1, "target", "acc-t", "ref-t", time.Now())); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	// Every attempt misses, as a session refreshing in a tight loop or a
	// degraded store would.
	store := &missingDeleteStore{memorySessionStore: inner, misses: 100}
	auth := NewAuthService(nil, store, nil, nil, "", nil)

	err := auth.RevokeSession(1, "target")
	if err == nil {
		t.Fatal("RevokeSession reported success over a session that is still live")
	}
	if err == ErrSessionNotFound {
		t.Errorf("err = %v; a surviving session is not a missing one", err)
	}
}

// The panic button must fail closed. The store's own DeleteByUserIDExcept is
// told which session to keep by access token and, when it cannot resolve one,
// keeps nothing and deletes everything — including the caller's. Revoking by
// name instead means an unidentifiable caller revokes nothing at all.
func TestRevokeOtherSessionsRefusesWhenTheCallerCannotBeIdentified(t *testing.T) {
	auth, store := sessionsFor(t,
		testSession(1, "here", "acc-here", "ref-here", time.Now()),
		testSession(1, "phone", "acc-phone", "ref-phone", time.Now()),
	)

	revoked, err := auth.RevokeOtherSessions(1, "acc-not-a-session")
	if err == nil {
		t.Fatal("RevokeOtherSessions proceeded without identifying the calling session")
	}
	if revoked != 0 {
		t.Errorf("revoked = %d, want 0", revoked)
	}
	if store.GetByAccessToken("acc-here") == nil || store.GetByAccessToken("acc-phone") == nil {
		t.Error("sessions were revoked despite the caller being unidentifiable — this is " +
			"the path that signs a member out of every device mid-panic")
	}
}

func TestRevokeOtherSessionsRetriesADeleteThatMisses(t *testing.T) {
	inner := newTestSessionStore()
	for _, sess := range []*Session{
		testSession(1, "here", "acc-here", "ref-here", time.Now()),
		testSession(1, "phone", "acc-phone", "ref-phone", time.Now()),
	} {
		if err := inner.Create(sess); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}
	store := &missingDeleteStore{memorySessionStore: inner, misses: 1}
	auth := NewAuthService(nil, store, nil, nil, "", nil)

	revoked, err := auth.RevokeOtherSessions(1, "acc-here")
	if err != nil {
		t.Fatalf("RevokeOtherSessions: %v", err)
	}
	if revoked != 1 {
		t.Errorf("revoked = %d, want 1 — a session counted once, however many "+
			"passes it took", revoked)
	}
	if inner.GetByRefreshToken("ref-phone") != nil {
		t.Error("the session survived a panic button that reported success")
	}
	if inner.GetByAccessToken("acc-here") == nil {
		t.Error("the caller was signed out")
	}
}

// countingStore records how often the session list is read.
type countingStore struct {
	*memorySessionStore
	lists int
}

func (s *countingStore) ListByUserID(userID int64) ([]*Session, error) {
	s.lists++
	return s.memorySessionStore.ListByUserID(userID)
}

// Revoking by name must not mean re-reading every session once per session.
// The list is the expensive call — one round trip per member of the user's set
// against Redis — so a member with many devices would pay for it squared.
func TestRevokeOtherSessionsReadsTheListABoundedNumberOfTimes(t *testing.T) {
	inner := newTestSessionStore()
	if err := inner.Create(testSession(1, "here", "acc-here", "ref-here", time.Now())); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	for i := 0; i < 8; i++ {
		id := fmt.Sprintf("other-%d", i)
		if err := inner.Create(testSession(1, id, "acc-"+id, "ref-"+id, time.Now())); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}
	store := &countingStore{memorySessionStore: inner}
	auth := NewAuthService(nil, store, nil, nil, "", nil)

	revoked, err := auth.RevokeOtherSessions(1, "acc-here")
	if err != nil {
		t.Fatalf("RevokeOtherSessions: %v", err)
	}
	if revoked != 8 {
		t.Errorf("revoked = %d, want 8", revoked)
	}
	if store.lists > revokeAttempts {
		t.Errorf("read the session list %d times to revoke 8 sessions; it should be "+
			"bounded by the retry count (%d), not by how many devices a member has",
			store.lists, revokeAttempts)
	}
}

func TestRevokeOtherSessionsWithNothingElseToRevokeIsZero(t *testing.T) {
	auth, _ := sessionsFor(t, testSession(1, "here", "acc-here", "ref-here", time.Now()))

	got, err := auth.RevokeOtherSessions(1, "acc-here")
	if err != nil {
		t.Fatalf("RevokeOtherSessions: %v", err)
	}
	if got != 0 {
		t.Errorf("revoked = %d, want 0", got)
	}
}
