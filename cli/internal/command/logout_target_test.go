package command

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/williamokano/go-torrent-trader/cli/internal/config"
)

// `tt auth logout` sends a credential tt stored, so it must obey the same
// profile-URL binding every other command does. It did not: revokeSession went
// through resolveSite, which has no such check, so a stale or hostile TT_URL made
// logout POST the 30-day refresh token to whoever answered, print success, exit 0,
// and then delete the only local copy — leaving a live session nobody could revoke.
//
// The assertion is on what the other server *received*, not on the error, because
// the error alone would pass even if the request had already gone out.
func TestLogoutNeverSendsTheStoredCredentialToAnotherSite(t *testing.T) {
	var mu sync.Mutex
	var sawPaths []string
	var sawBodies []string
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, 4096)
		n, _ := r.Body.Read(body)
		mu.Lock()
		sawPaths = append(sawPaths, r.URL.Path)
		sawBodies = append(sawBodies, string(body[:n])+" auth="+r.Header.Get("Authorization"))
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer elsewhere.Close()

	isolate(t)
	_, srv := newFakeSite(t)
	setupProfile(t, srv.URL)

	if got := runCLI(t, "hunter2\n", "auth", "login", "prod",
		"--username", "alice", "--password-stdin"); got.err != nil {
		t.Fatalf("auth login error = %v", got.err)
	}

	// The profile still points at the real site; only this invocation is redirected.
	t.Setenv(config.EnvURL, elsewhere.URL)
	got := runCLI(t, "", "auth", "logout", "prod")

	mu.Lock()
	paths, bodies := append([]string{}, sawPaths...), append([]string{}, sawBodies...)
	mu.Unlock()

	if len(paths) != 0 {
		t.Errorf("the other site received %v (%v) — the stored refresh token left the "+
			"profile's own origin", paths, bodies)
	}
	if got.err == nil {
		t.Fatal("auth logout succeeded against a different site, want a refusal")
	}
	if !errors.Is(got.err, config.ErrTokenHostMismatch) {
		t.Errorf("err = %v, want ErrTokenHostMismatch", got.err)
	}
	if code := exitCode(got.err); code != ExitAuth {
		t.Errorf("exit code = %d, want %d (auth)", code, ExitAuth)
	}

	// And the credential must survive: deleting it here is what made the original
	// bug unrecoverable, since the live session then had no local copy to revoke.
	stored, err := config.LoadCredentialRecord("prod")
	if err != nil {
		t.Fatalf("LoadCredentialRecord() error = %v", err)
	}
	if stored.RefreshToken == "" {
		t.Error("the refused logout deleted the credential anyway, leaving a live " +
			"session with nothing left to revoke it with")
	}
}

// Logging out against the profile's own site still works, so the guard above is
// not passing merely because logout is broken.
func TestLogoutStillWorksAgainstItsOwnSite(t *testing.T) {
	isolate(t)
	site, srv := newFakeSite(t)
	setupProfile(t, srv.URL)

	if got := runCLI(t, "hunter2\n", "auth", "login", "prod",
		"--username", "alice", "--password-stdin"); got.err != nil {
		t.Fatalf("auth login error = %v", got.err)
	}
	if got := runCLI(t, "", "auth", "logout", "prod"); got.err != nil {
		t.Fatalf("auth logout error = %v", got.err)
	}

	site.mu.Lock()
	calls := site.logoutCalls
	site.mu.Unlock()
	if calls != 1 {
		t.Errorf("logoutCalls = %d, want 1 — the session must be revoked server-side", calls)
	}
}

// A renewal whose local persist fails must not throw away the token it just
// obtained. The server has already rotated the pair by that point, so the copy on
// disk is dead either way; returning an error as well would fail the command the
// operator actually ran and lose a working credential in the process.
//
// Driven by making the config directory unwritable after login, so the refresh
// succeeds and only the write fails.
func TestARenewalThatCannotBeSavedStillCompletesTheCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("changes directory permissions")
	}
	isolate(t)
	site, srv := newFakeSite(t)
	setupProfile(t, srv.URL)

	// A 1-second access token. It must be *positive*: Tokens.ExpiresAt returns the
	// zero time for expires_in <= 0, and ExpiresWithin reads a zero expiry as
	// "never expires" — so a negative TTL silently disables the renewal this test
	// exists to exercise.
	site.mu.Lock()
	site.accessTTL = 1
	site.mu.Unlock()

	if got := runCLI(t, "hunter2\n", "auth", "login", "prod",
		"--username", "alice", "--password-stdin"); got.err != nil {
		t.Fatalf("auth login error = %v", got.err)
	}

	dir, err := config.Dir()
	if err != nil {
		t.Fatalf("config.Dir() error = %v", err)
	}
	// Readable so Load still works, but not writable, so only the persist fails.
	if err := chmodReadOnly(dir); err != nil {
		t.Fatalf("making config dir read-only: %v", err)
	}
	t.Cleanup(func() { _ = chmodPrivate(dir) })

	got := runCLI(t, "", "whoami")

	if got.err != nil {
		t.Errorf("whoami error = %v, want the command to complete on a fresh token "+
			"even though it could not be saved", got.err)
	}
	if !strings.Contains(got.stderr, "could not save it") {
		t.Errorf("stderr = %q, want a warning that the renewed token was not stored", got.stderr)
	}
	site.mu.Lock()
	refreshes, meAuth := site.refreshCalls, site.lastMeAuth
	site.mu.Unlock()
	if refreshes == 0 {
		t.Error("no refresh happened, so this test proves nothing about the persist path")
	}
	if meAuth == "" {
		t.Error("the request went out with no credential")
	}
}

// The refresher must send the refresh token it most recently received, not the one
// captured when it was built. Replaying a rotated token is two guaranteed-useless
// round trips today, and reads as token theft to any server with reuse detection.
func TestASecondRenewalUsesTheRotatedRefreshToken(t *testing.T) {
	isolate(t)
	site, srv := newFakeSite(t)
	setupProfile(t, srv.URL)

	// Positive, for the reason above: -1 means "no expiry" and this test then
	// passes without a single refresh, proving nothing about rotation.
	site.mu.Lock()
	site.accessTTL = 1
	site.mu.Unlock()

	if got := runCLI(t, "hunter2\n", "auth", "login", "prod",
		"--username", "alice", "--password-stdin"); got.err != nil {
		t.Fatalf("auth login error = %v", got.err)
	}

	// Two commands in a row. The fake site rotates validRefresh on every refresh
	// and rejects anything else, so a replayed token fails the second call.
	for i, name := range []string{"first", "second"} {
		if got := runCLI(t, "", "whoami"); got.err != nil {
			t.Fatalf("%s whoami (call %d) error = %v — a rotated refresh token was replayed",
				name, i+1, got.err)
		}
	}

	// Without this the test passes when nothing refreshes at all.
	site.mu.Lock()
	refreshes := site.refreshCalls
	site.mu.Unlock()
	if refreshes < 2 {
		t.Errorf("refreshCalls = %d, want at least 2 — the renewals this test is about "+
			"did not happen, so it proves nothing", refreshes)
	}
}

func chmodReadOnly(dir string) error { return os.Chmod(dir, 0o500) }
func chmodPrivate(dir string) error  { return os.Chmod(dir, 0o700) }

// Neither set-token nor clear-token revokes anything, so using either over a login
// record leaves a live 30-day session with no local copy — the exact state
// `tt auth logout` exists to prevent. Not refused, but never silent.
func TestReplacingOrClearingALoginSessionWarnsItIsNotRevoked(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		in   string
	}{
		{name: "set-token", args: []string{"auth", "set-token", "prod", "--stdin"}, in: "pasted-api-key\n"},
		{name: "clear-token", args: []string{"auth", "clear-token", "prod"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolate(t)
			_, srv := newFakeSite(t)
			setupProfile(t, srv.URL)

			if got := runCLI(t, "hunter2\n", "auth", "login", "prod",
				"--username", "alice", "--password-stdin"); got.err != nil {
				t.Fatalf("auth login error = %v", got.err)
			}

			got := runCLI(t, tc.in, tc.args...)
			if got.err != nil {
				t.Fatalf("%v error = %v", tc.args, got.err)
			}
			if !strings.Contains(got.stderr, "does not revoke it") {
				t.Errorf("stderr = %q, want a warning that the session stays valid", got.stderr)
			}
		})
	}
}

// A pasted API key has no refresh token, so replacing it must stay quiet — a
// warning on every set-token would train people to ignore the one that matters.
func TestReplacingAPastedKeyDoesNotWarn(t *testing.T) {
	isolate(t)
	_, srv := newFakeSite(t)
	setupProfile(t, srv.URL)

	if got := runCLI(t, "first-key\n", "auth", "set-token", "prod", "--stdin"); got.err != nil {
		t.Fatalf("first set-token error = %v", got.err)
	}
	got := runCLI(t, "second-key\n", "auth", "set-token", "prod", "--stdin")
	if got.err != nil {
		t.Fatalf("second set-token error = %v", got.err)
	}
	if strings.Contains(got.stderr, "does not revoke") {
		t.Errorf("stderr = %q, want no warning for a credential that cannot refresh", got.stderr)
	}
}
