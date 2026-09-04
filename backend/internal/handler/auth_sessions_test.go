package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// #171. A member could log in from several devices and had no way to see them,
// let alone end one. The only remedy for "someone else is signed in as me" was a
// password change, which signs out every one of the member's own devices too and
// still tells them nothing about what happened.
//
// Driven through the real router rather than the handler, because half of what
// these endpoints promise is enforced by middleware: RequireAuth is what makes
// the caller's own session identifiable, and it is what a revoked token has to
// start failing at.

type sessionRow struct {
	ID         string `json:"id"`
	DeviceName string `json:"device_name"`
	IP         string `json:"ip"`
	Current    bool   `json:"current"`
}

// listSessions calls GET /auth/sessions with the given access token and returns
// the decoded rows along with the raw body, which some assertions search for
// tokens that must not be in it.
func listSessions(t *testing.T, router http.Handler, accessToken string) ([]sessionRow, string) {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/sessions", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /auth/sessions = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Sessions []sessionRow `json:"sessions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding session list: %v; body: %s", err, rec.Body.String())
	}
	return resp.Sessions, rec.Body.String()
}

// deleteWithToken issues an authenticated DELETE and returns the response.
func deleteWithToken(t *testing.T, router http.Handler, path, accessToken string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodDelete, path, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// authenticatedGet is the liveness probe for an access token: /auth/me is behind
// RequireAuth, so a 401 there means the session is really gone rather than
// merely absent from a listing.
func authenticatedGet(t *testing.T, router http.Handler, accessToken string) int {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec.Code
}

func TestSessionListShowsEveryDeviceAndMarksTheCurrentOne(t *testing.T) {
	_, _, router := setupRouter()
	first := registerForTokens(t, router, "multidevice")
	second := loginForTokens(t, router, "multidevice")

	rows, _ := listSessions(t, router, second.Access)
	if len(rows) != 2 {
		t.Fatalf("got %d sessions, want 2 — a member with two logins has two sessions", len(rows))
	}

	var current int
	for _, row := range rows {
		if row.Current {
			current++
		}
		if row.ID == "" {
			t.Error("a session with no id cannot be revoked, which is the point of the list")
		}
	}
	if current != 1 {
		t.Errorf("%d sessions marked current, want exactly 1 — the UI labels this row and "+
			"protects it from the panic button", current)
	}

	// And the caller's own token is the one marked, not just any of them.
	fromFirst, _ := listSessions(t, router, first.Access)
	for _, row := range fromFirst {
		for _, other := range rows {
			if row.ID == other.ID && row.Current == other.Current && row.Current {
				t.Error("both callers see the same row as current, so 'current' is not " +
					"tracking the caller at all")
			}
		}
	}
}

// The list is the answer to "is someone else signed in as me", so it must not
// itself be a way to become them.
func TestSessionListNeverReturnsATokenInAnyForm(t *testing.T) {
	_, _, router := setupRouter()
	tokens := registerForTokens(t, router, "notokens")

	rows, body := listSessions(t, router, tokens.Access)
	if strings.Contains(body, tokens.Access) || strings.Contains(body, tokens.Refresh) {
		t.Fatal("the session list contains a live token — an endpoint meant to help a " +
			"member evict an intruder would be handing out credentials")
	}
	for _, row := range rows {
		if row.ID == tokens.Access || row.ID == tokens.Refresh {
			t.Fatal("the session id is a token")
		}
	}
}

// The acceptance criterion: revoking another session kills it immediately, not
// whenever its access token happens to expire.
func TestRevokingOneSessionInvalidatesItsAccessTokenImmediately(t *testing.T) {
	_, _, router := setupRouter()
	victim := registerForTokens(t, router, "evictor")
	current := loginForTokens(t, router, "evictor")

	rows, _ := listSessions(t, router, current.Access)
	var victimID string
	for _, row := range rows {
		if !row.Current {
			victimID = row.ID
		}
	}
	if victimID == "" {
		t.Fatal("fixture: expected one session that is not the caller's")
	}

	if code := authenticatedGet(t, router, victim.Access); code != http.StatusOK {
		t.Fatalf("fixture: the other session should work before revocation, got %d", code)
	}

	rec := deleteWithToken(t, router, "/api/v1/auth/sessions/"+victimID, current.Access)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE session = %d, want 204; body: %s", rec.Code, rec.Body.String())
	}

	if code := authenticatedGet(t, router, victim.Access); code != http.StatusUnauthorized {
		t.Errorf("the revoked session's access token still works (%d) — revocation that "+
			"waits for the token to expire is not revocation", code)
	}
	if code := authenticatedGet(t, router, current.Access); code != http.StatusOK {
		t.Errorf("revoking another session logged the caller out too (%d)", code)
	}
}

// Revoking also has to reach the refresh token, or the session comes back the
// moment its owner calls /auth/refresh — the whole shape of #231.
func TestRevokingOneSessionAlsoKillsItsRefreshToken(t *testing.T) {
	_, _, router := setupRouter()
	victim := registerForTokens(t, router, "refreshdead")
	current := loginForTokens(t, router, "refreshdead")

	rows, _ := listSessions(t, router, current.Access)
	for _, row := range rows {
		if row.Current {
			continue
		}
		if rec := deleteWithToken(t, router, "/api/v1/auth/sessions/"+row.ID, current.Access); rec.Code != http.StatusNoContent {
			t.Fatalf("DELETE session = %d", rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh",
		strings.NewReader(`{"refresh_token":"`+victim.Refresh+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("the revoked session minted a new access token at /auth/refresh (%d) — "+
			"only its short-lived half was actually revoked", rec.Code)
	}
}

// The other acceptance criterion: the panic button leaves the caller signed in,
// because they still have a password to change.
func TestRevokingAllOtherSessionsLeavesTheCallerLoggedIn(t *testing.T) {
	_, _, router := setupRouter()
	old1 := registerForTokens(t, router, "panicbutton")
	old2 := loginForTokens(t, router, "panicbutton")
	current := loginForTokens(t, router, "panicbutton")

	rec := deleteWithToken(t, router, "/api/v1/auth/sessions", current.Access)
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE /auth/sessions = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Revoked int `json:"revoked"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if resp.Revoked != 2 {
		t.Errorf("revoked = %d, want 2", resp.Revoked)
	}

	if code := authenticatedGet(t, router, current.Access); code != http.StatusOK {
		t.Errorf("the caller was signed out by their own panic button (%d), leaving them "+
			"unable to change their password", code)
	}
	for i, tok := range []string{old1.Access, old2.Access} {
		if code := authenticatedGet(t, router, tok); code != http.StatusUnauthorized {
			t.Errorf("other session %d survived (%d)", i, code)
		}
	}

	rows, _ := listSessions(t, router, current.Access)
	if len(rows) != 1 {
		t.Errorf("%d sessions left, want 1", len(rows))
	}
}

// One member must not be able to end another's session by guessing or replaying
// an id. The lookup only ever walks the caller's own sessions, so the answer is
// the same 404 an expired id gets — nothing to distinguish, nothing to probe.
func TestASessionIDFromAnotherMemberIsNotRevocable(t *testing.T) {
	_, _, router := setupRouter()
	victim := registerForTokens(t, router, "victimuser")
	attacker := registerForTokens(t, router, "attackeruser")

	rows, _ := listSessions(t, router, victim.Access)
	if len(rows) != 1 {
		t.Fatalf("fixture: victim has %d sessions, want 1", len(rows))
	}

	rec := deleteWithToken(t, router, "/api/v1/auth/sessions/"+rows[0].ID, attacker.Access)
	if rec.Code != http.StatusNotFound {
		t.Errorf("revoking another member's session = %d, want 404", rec.Code)
	}
	if code := authenticatedGet(t, router, victim.Access); code != http.StatusOK {
		t.Errorf("the victim's session was ended by another member (%d)", code)
	}
}

// The attacker's own panic button must not reach across accounts either.
func TestRevokingAllSessionsOnlyTouchesTheCallersOwn(t *testing.T) {
	_, _, router := setupRouter()
	bystander := registerForTokens(t, router, "bystander")
	caller := registerForTokens(t, router, "caller")

	if rec := deleteWithToken(t, router, "/api/v1/auth/sessions", caller.Access); rec.Code != http.StatusOK {
		t.Fatalf("DELETE /auth/sessions = %d", rec.Code)
	}
	if code := authenticatedGet(t, router, bystander.Access); code != http.StatusOK {
		t.Errorf("another member's session was revoked (%d)", code)
	}
}

func TestRevokingAnUnknownSessionIs404(t *testing.T) {
	_, _, router := setupRouter()
	tokens := registerForTokens(t, router, "unknownid")

	rec := deleteWithToken(t, router, "/api/v1/auth/sessions/nosuchsession", tokens.Access)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
}

// Unlike /auth/logout, these endpoints act on sessions other than the caller's,
// so holding a stale credential is not enough to reach them.
func TestSessionEndpointsRequireAuthentication(t *testing.T) {
	_, _, router := setupRouter()

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/auth/sessions"},
		{http.MethodDelete, "/api/v1/auth/sessions"},
		{http.MethodDelete, "/api/v1/auth/sessions/anything"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without a token = %d, want 401", tc.method, tc.path, rec.Code)
		}
	}
}

// A session is only recognisable if it says something about the device. Nothing
// ever wrote DeviceName before this — the field existed and stayed empty — which
// would have left every row of the list showing an IP and a timestamp.
func TestSessionsAreLabelledFromTheUserAgentWhenTheClientDoesNotNameItself(t *testing.T) {
	_, _, router := setupRouter()

	body := `{"username":"labelled","email":"labelled@example.com","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Gecko/20100101 Firefox/128.0")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register = %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Tokens struct {
			Access string `json:"access_token"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding register response: %v", err)
	}

	rows, _ := listSessions(t, router, resp.Tokens.Access)
	if len(rows) != 1 {
		t.Fatalf("got %d sessions, want 1", len(rows))
	}
	if rows[0].DeviceName != "Firefox on Windows" {
		t.Errorf("device_name = %q, want %q", rows[0].DeviceName, "Firefox on Windows")
	}
}

// A client that names itself keeps that name.
func TestAClientCanNameItsOwnSession(t *testing.T) {
	_, _, router := setupRouter()
	registerForTokens(t, router, "namedclient")

	body := `{"username":"namedclient","password":"password123","device_name":"Work laptop"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh) Chrome/120.0 Safari/537.36")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login = %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Tokens struct {
			Access string `json:"access_token"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding login response: %v", err)
	}

	rows, _ := listSessions(t, router, resp.Tokens.Access)
	for _, row := range rows {
		if row.Current {
			if row.DeviceName != "Work laptop" {
				t.Errorf("device_name = %q, want the name the client gave", row.DeviceName)
			}
			return
		}
	}
	t.Fatal("no current session in the list")
}

// A session's id has to survive a token refresh. It is the handle the member
// points at, and a list whose rows were renamed every hour — which is what a
// hash of the rotating refresh token would do — could not be acted on.
func TestASessionKeepsItsIDAcrossARefresh(t *testing.T) {
	_, _, router := setupRouter()
	tokens := registerForTokens(t, router, "rotating")

	before, _ := listSessions(t, router, tokens.Access)
	if len(before) != 1 {
		t.Fatalf("fixture: %d sessions, want 1", len(before))
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh",
		strings.NewReader(`{"refresh_token":"`+tokens.Refresh+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh = %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Tokens struct {
			Access string `json:"access_token"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding refresh response: %v", err)
	}

	after, _ := listSessions(t, router, resp.Tokens.Access)
	if len(after) != 1 {
		t.Fatalf("%d sessions after refresh, want 1 — rotation should replace, not add", len(after))
	}
	if after[0].ID != before[0].ID {
		t.Errorf("session id changed across a refresh: %q -> %q", before[0].ID, after[0].ID)
	}
}
