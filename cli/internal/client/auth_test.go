package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLoginReturnsTheTokenPair(t *testing.T) {
	var gotBody map[string]string
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"tokens":{"access_token":"a","refresh_token":"r","expires_in":3600}}`))
	}))
	defer srv.Close()

	tokens, err := Login(context.Background(), srv.URL, "alice", "hunter2")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if tokens.AccessToken != "a" || tokens.RefreshToken != "r" || tokens.ExpiresIn != 3600 {
		t.Errorf("tokens = %+v, want the pair from the response", tokens)
	}
	if gotBody["username"] != "alice" || gotBody["password"] != "hunter2" {
		t.Errorf("body = %v, want the credentials", gotBody)
	}
	// Logging in is the one call that must not carry a bearer token.
	if gotAuth != "" {
		t.Errorf("Authorization = %q, want none on login", gotAuth)
	}
}

func TestLoginSurfacesRejectedCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"unauthorized","message":"invalid credentials"}}`))
	}))
	defer srv.Close()

	_, err := Login(context.Background(), srv.URL, "alice", "wrong")

	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Login() error = %v, want a 401 APIError", err)
	}
}

// A 200 with no token is a broken site, and treating it as success would store
// an empty credential and fail confusingly later.
func TestLoginRejectsAResponseWithNoToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tokens":{}}`))
	}))
	defer srv.Close()

	if _, err := Login(context.Background(), srv.URL, "alice", "hunter2"); !errors.Is(err, ErrNoTokens) {
		t.Fatalf("Login() error = %v, want ErrNoTokens", err)
	}
}

func TestRefreshReturnsTheRotatedPair(t *testing.T) {
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"tokens":{"access_token":"a2","refresh_token":"r2","expires_in":60}}`))
	}))
	defer srv.Close()

	tokens, err := Refresh(context.Background(), srv.URL, "r1")
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if gotBody["refresh_token"] != "r1" {
		t.Errorf("body = %v, want the refresh token", gotBody)
	}
	if tokens.AccessToken != "a2" || tokens.RefreshToken != "r2" {
		t.Errorf("tokens = %+v, want the rotated pair", tokens)
	}
}

func TestRefreshRejectsAResponseWithNoToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	if _, err := Refresh(context.Background(), srv.URL, "r1"); !errors.Is(err, ErrNoTokens) {
		t.Fatalf("Refresh() error = %v, want ErrNoTokens", err)
	}
}

func TestLoginAndRefreshRejectABadBaseURL(t *testing.T) {
	if _, err := Login(context.Background(), "not-a-url", "a", "b"); err == nil {
		t.Error("Login() accepted an unusable base URL")
	}
	if _, err := Refresh(context.Background(), "not-a-url", "r"); err == nil {
		t.Error("Refresh() accepted an unusable base URL")
	}
}

func TestLogoutPostsWithTheBearerToken(t *testing.T) {
	var gotAuth, gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotMethod, gotPath = r.Header.Get("Authorization"), r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c, err := NewAuthenticated(srv.URL, "token-abc")
	if err != nil {
		t.Fatalf("NewAuthenticated() error = %v", err)
	}
	if err := c.Logout(context.Background()); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if gotAuth != "Bearer token-abc" {
		t.Errorf("Authorization = %q, want the token", gotAuth)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/auth/logout" {
		t.Errorf("%s %s, want POST /api/v1/auth/logout", gotMethod, gotPath)
	}
}

func TestExpiresAt(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)

	got := Tokens{ExpiresIn: 3600}.ExpiresAt(now)
	if want := now.Add(time.Hour); !got.Equal(want) {
		t.Errorf("ExpiresAt() = %v, want %v", got, want)
	}
	// A site that omits expires_in leaves the expiry unknown rather than
	// claiming the token died at the epoch, which would refresh on every call.
	if got := (Tokens{}).ExpiresAt(now); !got.IsZero() {
		t.Errorf("ExpiresAt() with no expires_in = %v, want the zero time", got)
	}
}

// The refresher retries exactly once and replays the same request, including a
// body — otherwise a retried POST would send nothing.
func TestDoRetriesOnceAfterRefreshing(t *testing.T) {
	var bodies []string
	var auths []string
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		buf := make([]byte, r.ContentLength)
		if r.ContentLength > 0 {
			_, _ = r.Body.Read(buf)
		}
		bodies = append(bodies, string(buf))
		auths = append(auths, r.Header.Get("Authorization"))
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	refreshed := 0
	c, err := NewAuthenticated(srv.URL, "stale", WithRefresher(func(context.Context) (string, error) {
		refreshed++
		return "good", nil
	}))
	if err != nil {
		t.Fatalf("NewAuthenticated() error = %v", err)
	}

	var out map[string]bool
	if err := c.Post(context.Background(), "/api/v1/thing", map[string]string{"k": "v"}, &out); err != nil {
		t.Fatalf("Post() error = %v", err)
	}

	if calls != 2 || refreshed != 1 {
		t.Fatalf("calls=%d refreshed=%d, want 2 and 1", calls, refreshed)
	}
	if bodies[0] != bodies[1] || bodies[1] != `{"k":"v"}` {
		t.Errorf("bodies = %q, want the same payload replayed", bodies)
	}
	if auths[0] != "Bearer stale" || auths[1] != "Bearer good" {
		t.Errorf("auths = %q, want the retry to carry the refreshed token", auths)
	}
	if !out["ok"] {
		t.Error("the retried response was not decoded")
	}
}

// mustReturn runs call and fails the test if it has not finished in time.
//
// The bug these retry tests guard against is a loop, and a looping Do never
// returns at all: it either spins on the CPU or hammers the server forever. A
// plain call would therefore hang the whole package until go test's global
// timeout kills it with no indication of which test was stuck — which is exactly
// what happened when this retry was mutated from an if into a for. Bounding it
// here turns that regression back into a named failure.
func mustReturn(t *testing.T, call func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- call() }()
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("Do() never returned: the 401 retry is looping")
		return nil
	}
}

// A refresher that cannot renew must let the original 401 through rather than
// masking it with its own error.
func TestDoSurfacesThe401WhenRefreshFails(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"unauthorized","message":"invalid or expired token"}}`))
	}))
	defer srv.Close()

	c, err := NewAuthenticated(srv.URL, "stale", WithRefresher(func(context.Context) (string, error) {
		return "", errors.New("refresh token is dead too")
	}))
	if err != nil {
		t.Fatalf("NewAuthenticated() error = %v", err)
	}

	err = mustReturn(t, func() error { return c.Get(context.Background(), "/api/v1/auth/me", nil) })

	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Get() error = %v, want the original 401", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want no retry when refreshing failed", calls)
	}
}

// A persistent 401 must not loop.
func TestDoRetriesAtMostOnce(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	refreshed := 0
	c, err := NewAuthenticated(srv.URL, "stale", WithRefresher(func(context.Context) (string, error) {
		refreshed++
		return "still-no-good", nil
	}))
	if err != nil {
		t.Fatalf("NewAuthenticated() error = %v", err)
	}

	if err := mustReturn(t, func() error { return c.Get(context.Background(), "/api/v1/auth/me", nil) }); err == nil {
		t.Fatal("Get() succeeded against a permanent 401")
	}
	if calls != 2 || refreshed != 1 {
		t.Errorf("calls=%d refreshed=%d, want exactly one retry", calls, refreshed)
	}
}

// Without a refresher a 401 is returned as-is.
func TestDoDoesNotRetryWithoutARefresher(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c, err := NewAuthenticated(srv.URL, "stale")
	if err != nil {
		t.Fatalf("NewAuthenticated() error = %v", err)
	}
	if err := c.Get(context.Background(), "/api/v1/auth/me", nil); err == nil {
		t.Fatal("Get() succeeded against a 401")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

// Only 401 triggers a refresh; a 403 means the token is valid but not permitted,
// and renewing it would change nothing.
func TestDoDoesNotRefreshOnForbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	refreshed := 0
	c, err := NewAuthenticated(srv.URL, "fine", WithRefresher(func(context.Context) (string, error) {
		refreshed++
		return "new", nil
	}))
	if err != nil {
		t.Fatalf("NewAuthenticated() error = %v", err)
	}
	if err := c.Get(context.Background(), "/api/v1/admin/x", nil); err == nil {
		t.Fatal("Get() succeeded against a 403")
	}
	if refreshed != 0 {
		t.Errorf("refreshed = %d, want 0 for a 403", refreshed)
	}
}
