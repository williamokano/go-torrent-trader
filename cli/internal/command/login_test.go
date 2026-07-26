package command

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/cli/internal/config"
)

// fakeSite is a stand-in for the auth endpoints tt depends on.
type fakeSite struct {
	mu sync.Mutex

	// validAccess is the token /auth/me currently accepts.
	validAccess string
	// validRefresh is the refresh token /auth/refresh currently accepts.
	validRefresh string

	username, password string

	loginCalls   int
	refreshCalls int
	logoutCalls  int
	meCalls      int
	// lastMeAuth is the Authorization header the last /auth/me request carried.
	lastMeAuth string
	// accessTTL is what login and refresh report as expires_in.
	accessTTL int64
	// logoutStatus lets a test make revocation fail.
	logoutStatus int
}

func newFakeSite(t *testing.T) (*fakeSite, *httptest.Server) {
	t.Helper()
	s := &fakeSite{
		validAccess:  "access-1",
		validRefresh: "refresh-1",
		username:     "alice",
		password:     "hunter2",
		accessTTL:    3600,
		logoutStatus: http.StatusNoContent,
	}
	srv := httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(srv.Close)
	return s, srv
}

func (s *fakeSite) handle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch r.URL.Path {
	case "/api/v1/auth/login":
		s.loginCalls++
		var body struct{ Username, Password string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Username != s.username || body.Password != s.password {
			writeAPIError(w, http.StatusUnauthorized, "unauthorized", "invalid credentials")
			return
		}
		s.writeTokens(w)

	case "/api/v1/auth/refresh":
		s.refreshCalls++
		var body struct {
			RefreshToken string `json:"refresh_token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.RefreshToken != s.validRefresh {
			writeAPIError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired refresh token")
			return
		}
		// Both tokens rotate, which is why the CLI has to persist the new pair.
		s.validAccess = fmt.Sprintf("access-%d", s.refreshCalls+1)
		s.validRefresh = fmt.Sprintf("refresh-%d", s.refreshCalls+1)
		s.writeTokens(w)

	case "/api/v1/auth/logout":
		s.logoutCalls++
		if r.Header.Get("Authorization") != "Bearer "+s.validAccess {
			writeAPIError(w, http.StatusUnauthorized, "unauthorized", "invalid token")
			return
		}
		if s.logoutStatus != http.StatusNoContent {
			w.WriteHeader(s.logoutStatus)
			return
		}
		s.validAccess, s.validRefresh = "", ""
		w.WriteHeader(http.StatusNoContent)

	case "/api/v1/auth/me":
		s.meCalls++
		s.lastMeAuth = r.Header.Get("Authorization")
		if s.validAccess == "" || r.Header.Get("Authorization") != "Bearer "+s.validAccess {
			writeAPIError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired token")
			return
		}
		_, _ = w.Write([]byte(`{"user":{"username":"alice"}}`))

	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (s *fakeSite) writeTokens(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"user":{"username":%q},"tokens":{"access_token":%q,"refresh_token":%q,"expires_in":%d}}`,
		s.username, s.validAccess, s.validRefresh, s.accessTTL)
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"error":{"code":%q,"message":%q}}`, code, message)
}

func (s *fakeSite) counts() (login, refresh, logout, me int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loginCalls, s.refreshCalls, s.logoutCalls, s.meCalls
}

func setupProfile(t *testing.T, url string) {
	t.Helper()
	if got := runCLI(t, "", "profile", "set", "prod", "--url", url); got.err != nil {
		t.Fatalf("profile set error = %v", got.err)
	}
}

func TestLoginStoresTheTokenPair(t *testing.T) {
	isolate(t)
	site, srv := newFakeSite(t)
	setupProfile(t, srv.URL)

	got := runCLI(t, "hunter2\n", "auth", "login", "prod", "--username", "alice", "--password-stdin")
	if got.err != nil {
		t.Fatalf("auth login error = %v", got.err)
	}
	// The password must never be echoed back.
	if strings.Contains(got.stdout+got.stderr, "hunter2") {
		t.Error("login echoed the password")
	}

	stored, err := config.LoadCredentialRecord("prod")
	if err != nil {
		t.Fatalf("LoadCredentialRecord() error = %v", err)
	}
	if stored.Token != "access-1" {
		t.Errorf("access token = %q, want access-1", stored.Token)
	}
	// Without the refresh token the credential dies in an hour, which is the
	// whole reason login exists rather than just documenting curl.
	if stored.RefreshToken != "refresh-1" {
		t.Errorf("refresh token = %q, want refresh-1", stored.RefreshToken)
	}
	if stored.ExpiresAt.IsZero() {
		t.Error("expires_at is zero, so tt cannot renew before spending a request")
	}
	if login, _, _, _ := site.counts(); login != 1 {
		t.Errorf("login calls = %d, want 1", login)
	}
}

func TestLoginRejectsBadCredentialsAsAnAuthFailure(t *testing.T) {
	isolate(t)
	_, srv := newFakeSite(t)
	setupProfile(t, srv.URL)

	t.Setenv(EnvPassword, "wrong-password")
	code := executeArgs(t, "auth", "login", "prod", "--username", "alice")
	if code != ExitAuth {
		t.Errorf("exit code = %d, want %d for a rejected password", code, ExitAuth)
	}

	stored, err := config.LoadCredentialRecord("prod")
	if err != nil {
		t.Fatalf("LoadCredentialRecord() error = %v", err)
	}
	if stored.Token != "" {
		t.Errorf("a failed login stored %q, want nothing", stored.Token)
	}
}

// The point of storing a refresh token: an expired access token renews itself
// instead of failing a scheduled job.
func TestExpiredTokenIsRefreshedTransparently(t *testing.T) {
	isolate(t)
	site, srv := newFakeSite(t)
	setupProfile(t, srv.URL)

	// A credential whose access token the server no longer accepts, but whose
	// refresh token is still good — exactly the state a cron job wakes up in.
	if err := config.StoreCredentialRecord("prod", config.Credential{
		Token:        "stale-access",
		RefreshToken: "refresh-1",
		ExpiresAt:    time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("StoreCredentialRecord() error = %v", err)
	}

	got := runCLI(t, "", "whoami")
	if got.err != nil {
		t.Fatalf("whoami error = %v", got.err)
	}
	if !strings.Contains(got.stdout, "alice") {
		t.Errorf("stdout = %q, want the profile", got.stdout)
	}

	_, refresh, _, _ := site.counts()
	if refresh != 1 {
		t.Errorf("refresh calls = %d, want exactly 1", refresh)
	}

	// The rotated pair must be persisted, or the next invocation replays a
	// refresh token the server has already retired.
	stored, err := config.LoadCredentialRecord("prod")
	if err != nil {
		t.Fatalf("LoadCredentialRecord() error = %v", err)
	}
	if stored.Token != "access-2" {
		t.Errorf("stored access token = %q, want the rotated access-2", stored.Token)
	}
	if stored.RefreshToken != "refresh-2" {
		t.Errorf("stored refresh token = %q, want the rotated refresh-2", stored.RefreshToken)
	}
}

// A token that expired without the CLI knowing (zero expires_at) must still
// recover, via the 401 rather than the clock.
func TestRefreshRecoversFromA401WhenExpiryIsUnknown(t *testing.T) {
	isolate(t)
	site, srv := newFakeSite(t)
	setupProfile(t, srv.URL)

	if err := config.StoreCredentialRecord("prod", config.Credential{
		Token:        "stale-access",
		RefreshToken: "refresh-1",
		// No ExpiresAt: nothing tells the CLI this is dead until the server does.
	}); err != nil {
		t.Fatalf("StoreCredentialRecord() error = %v", err)
	}

	if got := runCLI(t, "", "whoami"); got.err != nil {
		t.Fatalf("whoami error = %v", got.err)
	}
	if _, refresh, _, me := site.counts(); refresh != 1 || me != 2 {
		t.Errorf("refresh=%d me=%d, want one refresh and a retried request", refresh, me)
	}
}

// A dead refresh token must surface the original auth failure rather than
// retrying forever.
func TestAnUnusableRefreshTokenReportsAnAuthFailure(t *testing.T) {
	isolate(t)
	site, srv := newFakeSite(t)
	setupProfile(t, srv.URL)

	if err := config.StoreCredentialRecord("prod", config.Credential{
		Token:        "stale-access",
		RefreshToken: "refresh-that-is-also-dead",
	}); err != nil {
		t.Fatalf("StoreCredentialRecord() error = %v", err)
	}

	code := executeArgs(t, "whoami")
	if code != ExitAuth {
		t.Errorf("exit code = %d, want %d", code, ExitAuth)
	}
	if _, refresh, _, _ := site.counts(); refresh != 1 {
		t.Errorf("refresh calls = %d, want exactly 1 — no retry loop", refresh)
	}
}

// A pasted API key has no refresh token; it must not cost a pointless round trip.
func TestAKeyOnlyCredentialIsNeverRefreshed(t *testing.T) {
	isolate(t)
	site, srv := newFakeSite(t)
	setupProfile(t, srv.URL)

	if err := config.StoreCredential("prod", "some-api-key"); err != nil {
		t.Fatalf("StoreCredential() error = %v", err)
	}

	if code := executeArgs(t, "whoami"); code != ExitAuth {
		t.Errorf("exit code = %d, want %d", code, ExitAuth)
	}
	if _, refresh, _, _ := site.counts(); refresh != 0 {
		t.Errorf("refresh calls = %d, want 0 for a credential that cannot refresh", refresh)
	}
}

// An explicit --token belongs to the caller. Refreshing it would replace what
// they passed with a stored profile's session.
func TestAnExplicitTokenIsNeverRefreshed(t *testing.T) {
	isolate(t)
	site, srv := newFakeSite(t)
	setupProfile(t, srv.URL)

	if err := config.StoreCredentialRecord("prod", config.Credential{
		Token:        "stored-access",
		RefreshToken: "refresh-1",
	}); err != nil {
		t.Fatalf("StoreCredentialRecord() error = %v", err)
	}

	if code := executeArgs(t, "whoami", "--token", "caller-supplied"); code != ExitAuth {
		t.Errorf("exit code = %d, want %d", code, ExitAuth)
	}
	if _, refresh, _, _ := site.counts(); refresh != 0 {
		t.Errorf("refresh calls = %d, want 0 for a caller-supplied token", refresh)
	}
	// The stored credential must be untouched.
	stored, err := config.LoadCredentialRecord("prod")
	if err != nil {
		t.Fatalf("LoadCredentialRecord() error = %v", err)
	}
	if stored.Token != "stored-access" {
		t.Errorf("stored token = %q, want it untouched", stored.Token)
	}
}

// Logging out must kill the session on the server, not merely forget it here.
// A locally-deleted refresh token stays valid for its full lifetime otherwise —
// a live credential nobody can see and nobody revoked.
func TestLogoutRevokesServerSideThenDeletesLocally(t *testing.T) {
	isolate(t)
	site, srv := newFakeSite(t)
	setupProfile(t, srv.URL)

	if got := runCLI(t, "hunter2\n", "auth", "login", "prod", "--username", "alice", "--password-stdin"); got.err != nil {
		t.Fatalf("auth login error = %v", got.err)
	}
	if got := runCLI(t, "", "auth", "logout", "prod"); got.err != nil {
		t.Fatalf("auth logout error = %v", got.err)
	}

	if _, _, logout, _ := site.counts(); logout != 1 {
		t.Errorf("logout calls = %d, want 1", logout)
	}
	stored, err := config.LoadCredentialRecord("prod")
	if err != nil {
		t.Fatalf("LoadCredentialRecord() error = %v", err)
	}
	if stored.Token != "" || stored.RefreshToken != "" {
		t.Errorf("credential = %+v, want it removed", stored)
	}
}

// Logging out the morning after logging in must still revoke the session.
//
// The server finds a session from its access token and gives up when that lookup
// fails, so revoking with an expired one is a no-op that still answers 401.
// Because logout deletes the local file regardless, letting that stand would
// strand a refresh token that stays valid for thirty days with no copy left to
// revoke it with — the precise thing this command exists to prevent.
func TestLogoutRenewsAnExpiredTokenBeforeRevoking(t *testing.T) {
	isolate(t)
	site, srv := newFakeSite(t)
	setupProfile(t, srv.URL)

	if err := config.StoreCredentialRecord("prod", config.Credential{
		Token:        "stale-access",
		RefreshToken: "refresh-1",
		ExpiresAt:    time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatalf("StoreCredentialRecord() error = %v", err)
	}

	got := runCLI(t, "", "auth", "logout", "prod")
	if got.err != nil {
		t.Fatalf("auth logout error = %v", got.err)
	}
	if got.stderr != "" {
		t.Errorf("stderr = %q, want no revocation warning", got.stderr)
	}

	// Renewed up front, from the expiry already on disk, rather than by spending
	// a request to be told what the clock had already said. The 401 path would
	// also get there, but only after a round trip known in advance to fail.
	if _, refresh, logout, _ := site.counts(); refresh != 1 || logout != 1 {
		t.Errorf("refresh=%d logout=%d, want one renewal and a single logout", refresh, logout)
	}

	// The site clears both tokens only on a logout it accepted, so this is the
	// assertion that the session is genuinely dead rather than merely forgotten.
	site.mu.Lock()
	access, refresh := site.validAccess, site.validRefresh
	site.mu.Unlock()
	if access != "" || refresh != "" {
		t.Errorf("site still holds access=%q refresh=%q, want the session revoked", access, refresh)
	}

	stored, err := config.LoadCredentialRecord("prod")
	if err != nil {
		t.Fatalf("LoadCredentialRecord() error = %v", err)
	}
	if stored.Token != "" || stored.RefreshToken != "" {
		t.Errorf("credential = %+v, want it removed", stored)
	}
}

// If revocation fails, the local credential still goes — but the user must be
// told the session is still alive, not left believing they logged out.
//
// And "told" has to mean a non-zero exit code, not only a line on stderr. This
// previously returned nil: a deprovisioning script saw success while a live 30-day
// session stayed on the server, and the only way to notice was to grep stderr —
// which is exactly what this CLI's exit-code table exists to avoid.
func TestLogoutReportsAFailedRevocation(t *testing.T) {
	isolate(t)
	site, srv := newFakeSite(t)
	setupProfile(t, srv.URL)

	if got := runCLI(t, "hunter2\n", "auth", "login", "prod", "--username", "alice", "--password-stdin"); got.err != nil {
		t.Fatalf("auth login error = %v", got.err)
	}
	site.mu.Lock()
	site.logoutStatus = http.StatusInternalServerError
	site.mu.Unlock()

	got := runCLI(t, "", "auth", "logout", "prod")
	if got.err == nil {
		t.Fatal("auth logout succeeded, want a failure — a script must be able to " +
			"detect that a live session was left on the server")
	}
	if code := exitCode(got.err); code != ExitError {
		t.Errorf("exit code = %d, want %d", code, ExitError)
	}
	if !strings.Contains(got.stderr, "could not be invalidated") {
		t.Errorf("stderr = %q, want a warning that the session is still valid", got.stderr)
	}
	stored, err := config.LoadCredentialRecord("prod")
	if err != nil {
		t.Fatalf("LoadCredentialRecord() error = %v", err)
	}
	if stored.Token != "" {
		t.Error("the local credential survived a logout the user asked for")
	}
}

// --local is the escape hatch for an unreachable site.
func TestLogoutLocalSkipsTheServer(t *testing.T) {
	isolate(t)
	site, srv := newFakeSite(t)
	setupProfile(t, srv.URL)

	if got := runCLI(t, "hunter2\n", "auth", "login", "prod", "--username", "alice", "--password-stdin"); got.err != nil {
		t.Fatalf("auth login error = %v", got.err)
	}
	if got := runCLI(t, "", "auth", "logout", "prod", "--local"); got.err != nil {
		t.Fatalf("auth logout --local error = %v", got.err)
	}

	if _, _, logout, _ := site.counts(); logout != 0 {
		t.Errorf("logout calls = %d, want 0 with --local", logout)
	}
	stored, err := config.LoadCredentialRecord("prod")
	if err != nil {
		t.Fatalf("LoadCredentialRecord() error = %v", err)
	}
	if stored.Token != "" {
		t.Error("the local credential survived --local logout")
	}
}

func TestLogoutWithNothingStoredSaysSo(t *testing.T) {
	isolate(t)
	_, srv := newFakeSite(t)
	setupProfile(t, srv.URL)

	got := runCLI(t, "", "auth", "logout", "prod")
	if got.err != nil {
		t.Fatalf("auth logout error = %v", got.err)
	}
	if !strings.Contains(got.stdout, "No stored credential") {
		t.Errorf("stdout = %q, want it to say there was nothing to do", got.stdout)
	}
}

// TT_PASSWORD is the unattended path and must not require a terminal.
func TestLoginReadsThePasswordFromTheEnvironment(t *testing.T) {
	isolate(t)
	_, srv := newFakeSite(t)
	setupProfile(t, srv.URL)
	t.Setenv(EnvPassword, "hunter2")

	if got := runCLI(t, "", "auth", "login", "prod", "--username", "alice"); got.err != nil {
		t.Fatalf("auth login error = %v", got.err)
	}
	stored, err := config.LoadCredential("prod")
	if err != nil {
		t.Fatalf("LoadCredential() error = %v", err)
	}
	if stored == "" {
		t.Error("no token stored after an environment-password login")
	}
}

// The username is prompted for when not supplied, reading from stdin.
func TestLoginPromptsForTheUsername(t *testing.T) {
	isolate(t)
	_, srv := newFakeSite(t)
	setupProfile(t, srv.URL)

	got := runCLI(t, "alice\nhunter2\n", "auth", "login", "prod")
	if got.err != nil {
		t.Fatalf("auth login error = %v", got.err)
	}
	if !strings.Contains(got.stderr, "Username:") {
		t.Errorf("stderr = %q, want a username prompt", got.stderr)
	}
	stored, err := config.LoadCredential("prod")
	if err != nil {
		t.Fatalf("LoadCredential() error = %v", err)
	}
	if stored == "" {
		t.Error("no token stored after a prompted login")
	}
}

// Both values piped on one stream must both arrive. Wrapping stdin in a second
// bufio.Reader for the password made the first reader swallow it, so a fully
// piped login failed with "password is required" and nothing said why.
func TestLoginReadsUsernameAndPasswordFromOnePipe(t *testing.T) {
	isolate(t)
	site, srv := newFakeSite(t)
	setupProfile(t, srv.URL)

	got := runCLI(t, "alice\nhunter2\n", "auth", "login", "prod")
	if got.err != nil {
		t.Fatalf("auth login error = %v", got.err)
	}
	if login, _, _, _ := site.counts(); login != 1 {
		t.Errorf("login calls = %d, want 1", login)
	}
	stored, err := config.LoadCredential("prod")
	if err != nil {
		t.Fatalf("LoadCredential() error = %v", err)
	}
	if stored != "access-1" {
		t.Errorf("stored token = %q, want access-1", stored)
	}
}

// login honours TT_PROFILE like every other credential command.
func TestLoginHonoursProfileEnvironment(t *testing.T) {
	isolate(t)
	_, srv := newFakeSite(t)
	setupProfile(t, srv.URL)
	if got := runCLI(t, "", "profile", "set", "staging", "--url", srv.URL); got.err != nil {
		t.Fatalf("profile set error = %v", got.err)
	}
	t.Setenv(config.EnvProfile, "staging")
	t.Setenv(EnvPassword, "hunter2")

	if got := runCLI(t, "", "auth", "login", "--username", "alice"); got.err != nil {
		t.Fatalf("auth login error = %v", got.err)
	}

	staging, err := config.LoadCredential("staging")
	if err != nil {
		t.Fatalf("LoadCredential(staging) error = %v", err)
	}
	if staging == "" {
		t.Error("no token stored against the profile TT_PROFILE selected")
	}
	prod, err := config.LoadCredential("prod")
	if err != nil {
		t.Fatalf("LoadCredential(prod) error = %v", err)
	}
	if prod != "" {
		t.Errorf("prod token = %q, want the untargeted profile untouched", prod)
	}
}

func executeArgs(t *testing.T, args ...string) int {
	t.Helper()
	var out, errOut strings.Builder
	return Execute("test", &out, &errOut, args)
}
