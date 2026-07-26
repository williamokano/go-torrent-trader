package command

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/williamokano/go-torrent-trader/cli/internal/config"
)

// isolate points the CLI at a scratch config directory and clears the
// environment overrides, so a developer's own exported TT_* values cannot change
// what these tests observe.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv(config.EnvConfigDir, t.TempDir())
	t.Setenv(config.EnvProfile, "")
	t.Setenv(config.EnvURL, "")
	t.Setenv(config.EnvToken, "")
}

type result struct {
	stdout string
	stderr string
	err    error
}

func runCLI(t *testing.T, stdin string, args ...string) result {
	t.Helper()
	root := NewRoot("test")
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)

	err := root.Execute()
	return result{stdout: out.String(), stderr: errOut.String(), err: err}
}

func TestVersionTable(t *testing.T) {
	isolate(t)

	got := runCLI(t, "", "version")
	if got.err != nil {
		t.Fatalf("version error = %v", got.err)
	}
	if !strings.Contains(got.stdout, "test") {
		t.Errorf("stdout = %q, want the version string", got.stdout)
	}
	if !strings.Contains(got.stdout, "VERSION") {
		t.Errorf("stdout = %q, want a table header", got.stdout)
	}
}

func TestVersionJSON(t *testing.T) {
	isolate(t)

	got := runCLI(t, "", "version", "-o", "json")
	if got.err != nil {
		t.Fatalf("version error = %v", got.err)
	}
	if !strings.Contains(got.stdout, `"version": "test"`) {
		t.Errorf("stdout = %q, want JSON with the version", got.stdout)
	}
}

func TestProfileLifecycle(t *testing.T) {
	isolate(t)

	// Creating the first profile also selects it, so the CLI is usable without a
	// second command.
	if got := runCLI(t, "", "profile", "set", "prod", "--url", "https://tracker.example.com/"); got.err != nil {
		t.Fatalf("profile set error = %v", got.err)
	}

	listed := runCLI(t, "", "profile", "list")
	if listed.err != nil {
		t.Fatalf("profile list error = %v", listed.err)
	}
	if !strings.Contains(listed.stdout, "prod") {
		t.Errorf("list = %q, want the profile", listed.stdout)
	}
	// The trailing slash must have been trimmed on the way in.
	if !strings.Contains(listed.stdout, "https://tracker.example.com") ||
		strings.Contains(listed.stdout, "example.com/ ") {
		t.Errorf("list = %q, want the URL without a trailing slash", listed.stdout)
	}
	if !strings.Contains(listed.stdout, "*") {
		t.Errorf("list = %q, want the first profile marked current", listed.stdout)
	}

	if got := runCLI(t, "", "profile", "set", "staging", "--url", "https://staging.example.com"); got.err != nil {
		t.Fatalf("second profile set error = %v", got.err)
	}
	if got := runCLI(t, "", "profile", "use", "staging"); got.err != nil {
		t.Fatalf("profile use error = %v", got.err)
	}

	afterUse := runCLI(t, "", "profile", "list", "-o", "json")
	if afterUse.err != nil {
		t.Fatalf("profile list error = %v", afterUse.err)
	}
	if !strings.Contains(afterUse.stdout, `"name": "staging"`) {
		t.Errorf("list = %q, want staging listed", afterUse.stdout)
	}

	if got := runCLI(t, "", "profile", "remove", "prod"); got.err != nil {
		t.Fatalf("profile remove error = %v", got.err)
	}
	afterRemove := runCLI(t, "", "profile", "list")
	if afterRemove.err != nil {
		t.Fatalf("profile list error = %v", afterRemove.err)
	}
	if strings.Contains(afterRemove.stdout, "prod") {
		t.Errorf("list = %q, want prod gone", afterRemove.stdout)
	}
}

func TestProfileSetRequiresAURL(t *testing.T) {
	isolate(t)

	if got := runCLI(t, "", "profile", "set", "prod"); got.err == nil {
		t.Fatal("profile set without --url succeeded, want an error")
	}
}

func TestProfileUseRejectsAnUnknownProfile(t *testing.T) {
	isolate(t)

	got := runCLI(t, "", "profile", "use", "ghost")
	if got.err == nil {
		t.Fatal("profile use on an unknown profile succeeded, want an error")
	}
	if !strings.Contains(got.err.Error(), "ghost") {
		t.Errorf("error = %v, want it to name the profile", got.err)
	}
}

func TestProfileRemoveRejectsAnUnknownProfile(t *testing.T) {
	isolate(t)

	if got := runCLI(t, "", "profile", "remove", "ghost"); got.err == nil {
		t.Fatal("profile remove on an unknown profile succeeded, want an error")
	}
}

func TestProfileSetTokenStoresAndListReportsIt(t *testing.T) {
	isolate(t)

	if got := runCLI(t, "", "profile", "set", "prod", "--url", "https://tracker.example.com"); got.err != nil {
		t.Fatalf("profile set error = %v", got.err)
	}
	stored := runCLI(t, "token-abc\n", "auth", "set-token", "prod", "--stdin")
	if stored.err != nil {
		t.Fatalf("profile set-token error = %v", stored.err)
	}

	// Confirming the token landed must not involve printing it.
	if strings.Contains(stored.stdout, "token-abc") {
		t.Error("set-token echoed the token back to stdout")
	}

	listed := runCLI(t, "", "profile", "list")
	if listed.err != nil {
		t.Fatalf("profile list error = %v", listed.err)
	}
	if !strings.Contains(listed.stdout, "yes") {
		t.Errorf("list = %q, want the token column to report yes", listed.stdout)
	}
	if strings.Contains(listed.stdout, "token-abc") {
		t.Error("profile list printed the stored token")
	}

	got, err := config.LoadCredential("prod")
	if err != nil {
		t.Fatalf("LoadCredential() error = %v", err)
	}
	if got != "token-abc" {
		t.Errorf("stored token = %q, want token-abc", got)
	}
}

func TestProfileSetTokenRejectsAnEmptyToken(t *testing.T) {
	isolate(t)

	if got := runCLI(t, "\n", "auth", "set-token", "prod", "--stdin"); got.err == nil {
		t.Fatal("set-token with an empty token succeeded, want an error")
	}
}

func TestProfileClearToken(t *testing.T) {
	isolate(t)

	if got := runCLI(t, "", "profile", "set", "prod", "--url", "https://tracker.example.com"); got.err != nil {
		t.Fatalf("profile set error = %v", got.err)
	}
	if got := runCLI(t, "token-abc\n", "auth", "set-token", "prod", "--stdin"); got.err != nil {
		t.Fatalf("profile set-token error = %v", got.err)
	}
	if got := runCLI(t, "", "auth", "clear-token", "prod"); got.err != nil {
		t.Fatalf("profile clear-token error = %v", got.err)
	}

	got, err := config.LoadCredential("prod")
	if err != nil {
		t.Fatalf("LoadCredential() error = %v", err)
	}
	if got != "" {
		t.Errorf("token = %q, want empty after clear-token", got)
	}
}

// Removing a profile must take its token with it, or a live credential is left
// behind in a file nothing references.
func TestProfileRemoveAlsoDeletesTheToken(t *testing.T) {
	isolate(t)

	if got := runCLI(t, "", "profile", "set", "prod", "--url", "https://tracker.example.com"); got.err != nil {
		t.Fatalf("profile set error = %v", got.err)
	}
	if got := runCLI(t, "token-abc\n", "auth", "set-token", "prod", "--stdin"); got.err != nil {
		t.Fatalf("profile set-token error = %v", got.err)
	}
	if got := runCLI(t, "", "profile", "remove", "prod"); got.err != nil {
		t.Fatalf("profile remove error = %v", got.err)
	}

	got, err := config.LoadCredential("prod")
	if err != nil {
		t.Fatalf("LoadCredential() error = %v", err)
	}
	if got != "" {
		t.Errorf("token = %q, want it removed with the profile", got)
	}
}

func TestWhoami(t *testing.T) {
	isolate(t)

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/me" {
			t.Errorf("path = %q, want /api/v1/auth/me", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user":{
			"id": 7,
			"username": "alice",
			"email": "alice@example.com",
			"uploaded": 1073741824,
			"downloaded": 536870912,
			"ratio": 2,
			"invites": 3,
			"warned": false,
			"passkey": "a1b2************c5d6"
		}}`))
	}))
	defer srv.Close()

	got := runCLI(t, "", "whoami", "--url", srv.URL, "--token", "token-abc")
	if got.err != nil {
		t.Fatalf("whoami error = %v", got.err)
	}
	if gotAuth != "Bearer token-abc" {
		t.Errorf("Authorization = %q, want Bearer token-abc", gotAuth)
	}
	if !strings.Contains(got.stdout, "alice") {
		t.Errorf("stdout = %q, want the username", got.stdout)
	}
	if !strings.Contains(got.stdout, "1.00 GiB") {
		t.Errorf("stdout = %q, want a human byte count", got.stdout)
	}
	if !strings.Contains(got.stdout, "2.00") {
		t.Errorf("stdout = %q, want the ratio", got.stdout)
	}
	// Even a masked passkey stays out of the default table.
	if strings.Contains(got.stdout, "a1b2") {
		t.Error("whoami table included the passkey")
	}
}

// An infinite ratio arrives as the -1 sentinel and must not render as "-1.00".
func TestWhoamiRendersAnInfiniteRatio(t *testing.T) {
	isolate(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"user":{"username":"alice","uploaded":1024,"downloaded":0,"ratio":-1}}`))
	}))
	defer srv.Close()

	got := runCLI(t, "", "whoami", "--url", srv.URL, "--token", "token-abc")
	if got.err != nil {
		t.Fatalf("whoami error = %v", got.err)
	}
	if !strings.Contains(got.stdout, "Inf") {
		t.Errorf("stdout = %q, want Inf for the -1 sentinel", got.stdout)
	}
	if strings.Contains(got.stdout, "-1.00") {
		t.Errorf("stdout = %q, want the sentinel translated", got.stdout)
	}
}

// --output json must show every field the API returned, including the ones the
// table omits, so scripts are not limited to the summary.
func TestWhoamiJSONIncludesFullDetail(t *testing.T) {
	isolate(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"user":{"username":"alice","email":"alice@example.com","passkey":"a1b2****c5d6"}}`))
	}))
	defer srv.Close()

	got := runCLI(t, "", "whoami", "-o", "json", "--url", srv.URL, "--token", "token-abc")
	if got.err != nil {
		t.Fatalf("whoami error = %v", got.err)
	}
	for _, want := range []string{"alice@example.com", "a1b2****c5d6"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout = %q, want it to contain %q", got.stdout, want)
		}
	}
}

// The request must not be sent at all without a credential, and the error has to
// say how to supply one.
func TestWhoamiWithoutATokenIsAnActionableError(t *testing.T) {
	isolate(t)

	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		_, _ = w.Write([]byte(`{"user":{"username":"public"}}`))
	}))
	defer srv.Close()

	got := runCLI(t, "", "whoami", "--url", srv.URL)
	if got.err == nil {
		t.Fatal("whoami without a token succeeded, want an error")
	}
	if called {
		t.Error("whoami sent an anonymous request instead of refusing")
	}
	for _, want := range []string{config.EnvToken, "--token", "set-token"} {
		if !strings.Contains(got.err.Error(), want) {
			t.Errorf("error %q does not mention %q", got.err, want)
		}
	}
}

func TestWhoamiSurfacesAnAPIError(t *testing.T) {
	isolate(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"unauthorized","message":"invalid or expired token"}}`))
	}))
	defer srv.Close()

	got := runCLI(t, "", "whoami", "--url", srv.URL, "--token", "stale")
	if got.err == nil {
		t.Fatal("whoami succeeded against a 401, want an error")
	}
	if !strings.Contains(got.err.Error(), "invalid or expired token") {
		t.Errorf("error = %v, want the API message", got.err)
	}
}

// TT_TOKEN and TT_URL are the CI path and must work with no config file at all.
func TestWhoamiFromEnvironmentOnly(t *testing.T) {
	isolate(t)

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"user":{"username":"alice"}}`))
	}))
	defer srv.Close()

	t.Setenv(config.EnvURL, srv.URL)
	t.Setenv(config.EnvToken, "env-token")

	got := runCLI(t, "", "whoami")
	if got.err != nil {
		t.Fatalf("whoami error = %v", got.err)
	}
	if gotAuth != "Bearer env-token" {
		t.Errorf("Authorization = %q, want the environment token", gotAuth)
	}
}

func TestExecuteReportsFailureExitCode(t *testing.T) {
	isolate(t)

	var out, errOut bytes.Buffer
	code := Execute("test", &out, &errOut, []string{"profile", "use", "ghost"})
	if code != ExitUsage {
		t.Errorf("exit code = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errOut.String(), "tt: ") {
		t.Errorf("stderr = %q, want the tt: prefix", errOut.String())
	}
}

func TestExecuteReportsSuccessExitCode(t *testing.T) {
	isolate(t)

	var out, errOut bytes.Buffer
	code := Execute("test", &out, &errOut, []string{"version"})
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

// A failing API call is not a usage mistake and must not answer with usage text,
// which would bury the actual error.
func TestAPIErrorDoesNotPrintUsage(t *testing.T) {
	isolate(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	var out, errOut bytes.Buffer
	Execute("test", &out, &errOut, []string{"whoami", "--url", srv.URL, "--token", "t"})
	if strings.Contains(out.String()+errOut.String(), "Usage:") {
		t.Errorf("output contained usage text: %q", out.String()+errOut.String())
	}
}

func TestUnknownOutputFormatIsRejected(t *testing.T) {
	isolate(t)

	got := runCLI(t, "", "version", "-o", "xml")
	if got.err == nil {
		t.Fatal("an unknown --output value succeeded, want an error")
	}
}

func TestProfileCompletionOffersConfiguredNames(t *testing.T) {
	isolate(t)

	for _, name := range []string{"prod", "staging"} {
		if got := runCLI(t, "", "profile", "set", name, "--url", "https://"+name+".example.com"); got.err != nil {
			t.Fatalf("profile set %s error = %v", name, got.err)
		}
	}

	names, directive := completeProfileNames(nil, nil, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want ShellCompDirectiveNoFileComp", directive)
	}
	if len(names) != 2 || names[0] != "prod" || names[1] != "staging" {
		t.Errorf("names = %v, want [prod staging] sorted", names)
	}
}
