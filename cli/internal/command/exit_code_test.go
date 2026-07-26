package command

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/williamokano/go-torrent-trader/cli/internal/config"
)

// Exit codes are the interface a scheduled job branches on. "Everything is 1"
// forces a cron wrapper to regex stderr, and while the only credential a site
// issues expires after an hour, telling "token expired" from "site down" is the
// single most useful signal the CLI can give.
func TestExitCodes(t *testing.T) {
	authFailure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"unauthorized","message":"invalid or expired token"}}`))
	}))
	defer authFailure.Close()

	forbidden := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer forbidden.Close()

	serverError := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer serverError.Close()

	// A closed listener gives a deterministic connection refusal.
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	deadURL := dead.URL
	dead.Close()

	tests := []struct {
		name string
		args []string
		want int
	}{
		{
			name: "success",
			args: []string{"version"},
			want: ExitOK,
		},
		{
			name: "unknown flag is a usage error",
			args: []string{"version", "--nope"},
			want: ExitUsage,
		},
		{
			name: "unknown output format is a usage error",
			args: []string{"version", "-o", "xml"},
			want: ExitUsage,
		},
		{
			name: "too many arguments is a usage error",
			args: []string{"whoami", "extra"},
			want: ExitUsage,
		},
		{
			name: "no credential is an auth error",
			args: []string{"whoami", "--url", authFailure.URL},
			want: ExitAuth,
		},
		{
			name: "a rejected token is an auth error",
			args: []string{"whoami", "--url", authFailure.URL, "--token", "stale"},
			want: ExitAuth,
		},
		{
			name: "403 is an auth error",
			args: []string{"whoami", "--url", forbidden.URL, "--token", "t"},
			want: ExitAuth,
		},
		{
			name: "a server error is a general failure, not a network one",
			args: []string{"whoami", "--url", serverError.URL, "--token", "t"},
			want: ExitError,
		},
		{
			name: "an unreachable site is a network error",
			args: []string{"whoami", "--url", deadURL, "--token", "t"},
			want: ExitNetwork,
		},
		{
			name: "a timeout is a network error",
			args: []string{"whoami", "--url", "http://192.0.2.1:9", "--token", "t", "--timeout", "50ms"},
			want: ExitNetwork,
		},
		{
			name: "a non-positive timeout is a usage error",
			args: []string{"whoami", "--url", authFailure.URL, "--token", "t", "--timeout", "0"},
			want: ExitUsage,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			isolate(t)
			var out, errOut bytes.Buffer
			if got := Execute("test", &out, &errOut, tc.args); got != tc.want {
				t.Errorf("exit code = %d, want %d (stderr: %s)", got, tc.want, errOut.String())
			}
		})
	}
}

// --timeout 0 means "no timeout" to net/http, which is the exact hang the flag
// exists to prevent, and many tools read 0 as "use the default".
func TestNonPositiveTimeoutIsRejected(t *testing.T) {
	isolate(t)

	for _, v := range []string{"0", "-5s"} {
		got := runCLI(t, "", "whoami", "--url", "https://tracker.example.com", "--token", "t", "--timeout", v)
		if got.err == nil {
			t.Fatalf("--timeout %s was accepted, want an error", v)
		}
		if !strings.Contains(got.err.Error(), "positive") {
			t.Errorf("--timeout %s error = %v, want it to say the value must be positive", v, got.err)
		}
	}
}

// A stored token belongs to the site it was stored for. Sending it to whatever
// --url happens to name hands the credential to whoever answers that host.
func TestStoredTokenIsNotSentToAnotherHost(t *testing.T) {
	isolate(t)

	var reached bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		_, _ = w.Write([]byte(`{"user":{"username":"whoever"}}`))
	}))
	defer other.Close()

	if got := runCLI(t, "", "profile", "set", "prod", "--url", "https://tracker.example.com"); got.err != nil {
		t.Fatalf("profile set error = %v", got.err)
	}
	if err := config.StoreCredential("prod", "secret-token"); err != nil {
		t.Fatalf("StoreCredential() error = %v", err)
	}

	got := runCLI(t, "", "whoami", "--url", other.URL)
	if got.err == nil {
		t.Fatal("whoami sent the stored token to a different host, want a refusal")
	}
	if reached {
		t.Error("the other host received a request carrying the stored token")
	}
	// The refusal has to explain how to proceed deliberately.
	for _, want := range []string{"--token", config.EnvToken} {
		if !strings.Contains(got.err.Error(), want) {
			t.Errorf("error %q does not mention %q", got.err, want)
		}
	}
}

// An explicit token is the caller saying where it should go, so it is not
// second-guessed.
func TestExplicitTokenIsSentToAnyHost(t *testing.T) {
	isolate(t)

	var gotAuth string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"user":{"username":"whoever"}}`))
	}))
	defer other.Close()

	if got := runCLI(t, "", "profile", "set", "prod", "--url", "https://tracker.example.com"); got.err != nil {
		t.Fatalf("profile set error = %v", got.err)
	}
	if err := config.StoreCredential("prod", "secret-token"); err != nil {
		t.Fatalf("StoreCredential() error = %v", err)
	}

	if got := runCLI(t, "", "whoami", "--url", other.URL, "--token", "explicit"); got.err != nil {
		t.Fatalf("whoami error = %v", got.err)
	}
	if gotAuth != "Bearer explicit" {
		t.Errorf("Authorization = %q, want the explicit token", gotAuth)
	}
}

// The same URL written with a trailing slash is the same site, and must not trip
// the host guard.
func TestStoredTokenSurvivesATrailingSlashDifference(t *testing.T) {
	isolate(t)

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"user":{"username":"alice"}}`))
	}))
	defer srv.Close()

	if got := runCLI(t, "", "profile", "set", "prod", "--url", srv.URL); got.err != nil {
		t.Fatalf("profile set error = %v", got.err)
	}
	if err := config.StoreCredential("prod", "secret-token"); err != nil {
		t.Fatalf("StoreCredential() error = %v", err)
	}

	if got := runCLI(t, "", "whoami", "--url", srv.URL+"/"); got.err != nil {
		t.Fatalf("whoami error = %v", got.err)
	}
	if gotAuth != "Bearer secret-token" {
		t.Errorf("Authorization = %q, want the stored token", gotAuth)
	}
}
