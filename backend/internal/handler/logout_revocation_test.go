package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// #231. A session lives one hour on its access token and thirty days on its refresh
// token, and it could only be revoked by its access token — the route was behind
// RequireAuth, which rejects an expired one with a 401 before the handler runs, and
// the store looked the session up by access key and gave up when that missed.
//
// So for the remaining twenty-nine days a session could be *used* — `/auth/refresh`
// mints a new access token from it — but not cancelled. A client that deleted its
// local copy on logout, which is the correct thing for a client to do, stranded a
// live credential nobody could see or revoke.
//
// Driven through the real router, because the route's middleware was half the bug.
// Asserted on what the store holds afterwards rather than on the status code: the
// question is whether the credential is dead, and a 204 over a surviving session is
// exactly the defect.
func TestLogoutRevokesASessionWhoseAccessTokenIsGone(t *testing.T) {
	_, sessions, router := setupRouter()
	tokens := registerForTokens(t, router, "expireduser")

	// The state an hour in: the access token no longer resolves, the refresh token
	// still does. Simulated by dropping just the access half, which is what Redis
	// TTL expiry does.
	sess := sessions.GetByAccessToken(tokens.Access)
	if sess == nil {
		t.Fatal("fixture: the new session should resolve by access token")
	}
	dropAccessHalf(t, sessions, sess)

	if sessions.GetByRefreshToken(tokens.Refresh) == nil {
		t.Fatal("fixture: the refresh token must still resolve, or this proves nothing")
	}

	// The header carries the now-dead access token, exactly as a real client would
	// send it, and the body carries the refresh token.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout",
		strings.NewReader(`{"refresh_token":"`+tokens.Refresh+`"}`))
	req.Header.Set("Authorization", "Bearer "+tokens.Access)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", rec.Code, rec.Body.String())
	}
	if sessions.GetByRefreshToken(tokens.Refresh) != nil {
		t.Error("the refresh token still resolves after logout, so the session is " +
			"still usable at /auth/refresh and nobody can revoke it")
	}
}

// And the refresh token alone is enough, with no Authorization header at all — which
// is the state a client is in after it has discarded an access token it knows is
// expired.
func TestLogoutAcceptsTheRefreshTokenWithNoAuthorizationHeader(t *testing.T) {
	_, sessions, router := setupRouter()
	tokens := registerForTokens(t, router, "refreshonly")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout",
		strings.NewReader(`{"refresh_token":"`+tokens.Refresh+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", rec.Code, rec.Body.String())
	}
	if sessions.GetByRefreshToken(tokens.Refresh) != nil {
		t.Error("the refresh token survived")
	}
	if sessions.GetByAccessToken(tokens.Access) != nil {
		t.Error("the access token survived a revocation by refresh token; both halves " +
			"must go or the session lives on the one nobody looked at")
	}
}

// Revoking by access token must also remove the refresh half. This was already true,
// and it is the assertion that keeps it true now that two paths reach the same store.
func TestLogoutByAccessTokenRemovesBothHalves(t *testing.T) {
	_, sessions, router := setupRouter()
	tokens := registerForTokens(t, router, "bothhalves")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+tokens.Access)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", rec.Code, rec.Body.String())
	}
	if sessions.GetByAccessToken(tokens.Access) != nil {
		t.Error("the access token survived")
	}
	if sessions.GetByRefreshToken(tokens.Refresh) != nil {
		t.Error("the refresh token survived a logout by access token")
	}
}

// The route moved from RequireAuth to OptionalAuth, so the handler is now the only
// thing between an anonymous request and a 204. This is the assertion that the move
// did not open the endpoint up.
func TestLogoutWithNoUsableCredentialIsUnauthorized(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "no body", body: ""},
		{name: "empty object", body: `{}`},
		{name: "empty refresh token", body: `{"refresh_token":""}`},
		{name: "refresh token that was never issued", body: `{"refresh_token":"not-a-real-token"}`},
		{name: "malformed body", body: `{not json`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, router := setupRouter()

			var req *http.Request
			if tc.body == "" {
				req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
			} else {
				req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", strings.NewReader(tc.body))
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401; body: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// A revoked refresh token must not still mint access tokens, which is the capability
// the whole issue is about.
func TestARevokedRefreshTokenCannotBeRefreshed(t *testing.T) {
	_, _, router := setupRouter()
	tokens := registerForTokens(t, router, "norefresh")

	logout := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout",
		strings.NewReader(`{"refresh_token":"`+tokens.Refresh+`"}`))
	logout.Header.Set("Content-Type", "application/json")
	logoutRec := httptest.NewRecorder()
	router.ServeHTTP(logoutRec, logout)
	if logoutRec.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, want 204", logoutRec.Code)
	}

	refresh := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh",
		strings.NewReader(`{"refresh_token":"`+tokens.Refresh+`"}`))
	refresh.Header.Set("Content-Type", "application/json")
	refreshRec := httptest.NewRecorder()
	router.ServeHTTP(refreshRec, refresh)

	if refreshRec.Code == http.StatusOK {
		t.Errorf("a revoked refresh token still minted tokens: %s", refreshRec.Body.String())
	}
}

// Revoking one session must leave the user's others alone.
func TestLogoutLeavesTheUsersOtherSessionsAlone(t *testing.T) {
	_, sessions, router := setupRouter()
	phone := registerForTokens(t, router, "twodevices")
	laptop := loginForTokens(t, router, "twodevices")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout",
		strings.NewReader(`{"refresh_token":"`+phone.Refresh+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", rec.Code, rec.Body.String())
	}
	if sessions.GetByRefreshToken(laptop.Refresh) == nil {
		t.Error("logging out one session revoked another")
	}
}

// --- helpers ---------------------------------------------------------------

type tokenPair struct{ Access, Refresh string }

func registerForTokens(t *testing.T, router http.Handler, username string) tokenPair {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"username": username,
		"email":    username + "@example.com",
		"password": "password123",
	})
	return postForTokens(t, router, "/api/v1/auth/register", body)
}

func loginForTokens(t *testing.T, router http.Handler, username string) tokenPair {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"username": username,
		"password": "password123",
	})
	return postForTokens(t, router, "/api/v1/auth/login", body)
}

func postForTokens(t *testing.T, router http.Handler, path string, body []byte) tokenPair {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var resp struct {
		Tokens struct {
			Access  string `json:"access_token"`
			Refresh string `json:"refresh_token"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("%s returned %d: %s", path, rec.Code, rec.Body.String())
	}
	if resp.Tokens.Access == "" || resp.Tokens.Refresh == "" {
		t.Fatalf("%s returned no token pair (%d): %s", path, rec.Code, rec.Body.String())
	}
	return tokenPair{Access: resp.Tokens.Access, Refresh: resp.Tokens.Refresh}
}

// dropAccessHalf leaves the session reachable only by its refresh token, which is what
// the access key's one-hour TTL produces in Redis.
func dropAccessHalf(t *testing.T, sessions service.SessionStore, sess *service.Session) {
	t.Helper()
	// Copied before Delete, which removes both halves.
	revived := *sess
	revived.AccessToken = "" // the half that has expired
	sessions.Delete(sess.AccessToken)

	// RefreshExpiresAt has to come across: the store treats a zero value as already
	// expired, so dropping it would make the fixture pass for the wrong reason —
	// the refresh token would be gone because it "expired", not because logout
	// revoked it.
	if err := sessions.Create(&revived); err != nil {
		t.Fatalf("re-seeding the refresh half: %v", err)
	}
}
