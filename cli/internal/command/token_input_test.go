package command

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/williamokano/go-torrent-trader/cli/internal/config"
)

// Piping a token in without --stdin must work too: stdin is not a terminal, so
// there is nothing to prompt on and reading the line is the only sensible answer.
func TestReadTokenFromAPipeWithoutTheStdinFlag(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("piped-token\n"))

	got, err := readToken(cmd, false)
	if err != nil {
		t.Fatalf("readToken() error = %v", err)
	}
	if got != "piped-token" {
		t.Errorf("token = %q, want piped-token", got)
	}
}

// A token pasted with surrounding whitespace, or a file with no trailing
// newline, must still yield the bare token — a stray \n in an Authorization
// header is a confusing 401.
func TestReadTokenTrimsWhitespace(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "trailing newline", in: "token-abc\n", want: "token-abc"},
		{name: "no trailing newline", in: "token-abc", want: "token-abc"},
		{name: "surrounding spaces", in: "  token-abc  \n", want: "token-abc"},
		{name: "carriage return", in: "token-abc\r\n", want: "token-abc"},
		{name: "empty", in: "\n", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.SetIn(strings.NewReader(tc.in))

			got, err := readToken(cmd, true)
			if err != nil {
				t.Fatalf("readToken() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("token = %q, want %q", got, tc.want)
			}
		})
	}
}

// Profile-name resolution must be identical everywhere. The credential commands
// resolving it themselves is how set-token came to ignore TT_PROFILE.
func TestProfileNameResolution(t *testing.T) {
	tests := []struct {
		name    string
		arg     string
		flag    string
		env     string
		current string
		want    string
	}{
		{name: "positional argument wins", arg: "from-arg", flag: "from-flag", env: "from-env", current: "cur", want: "from-arg"},
		{name: "flag beats the environment", flag: "from-flag", env: "from-env", current: "cur", want: "from-flag"},
		{name: "environment beats the current profile", env: "from-env", current: "cur", want: "from-env"},
		{name: "current profile is the fallback", current: "cur", want: "cur"},
		{name: "nothing at all yields the default", want: config.DefaultProfileName},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			isolate(t)
			t.Setenv(config.EnvProfile, tc.env)

			g := &globals{profile: tc.flag}
			f := &config.File{CurrentProfile: tc.current}
			if got := g.profileName(f, tc.arg); got != tc.want {
				t.Errorf("profileName() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TT_PROFILE selecting a profile that is not the current one must be honoured by
// the credential commands. Getting this wrong overwrites the wrong site's token
// and reports success.
func TestSetTokenHonoursProfileEnvironment(t *testing.T) {
	isolate(t)

	for _, name := range []string{"prod", "staging"} {
		if got := runCLI(t, "", "profile", "set", name, "--url", "https://"+name+".example.com"); got.err != nil {
			t.Fatalf("profile set %s error = %v", name, got.err)
		}
	}
	// "prod" was created first, so it is current. Select "staging" by environment.
	if err := config.StoreCredential("prod", "prod-token"); err != nil {
		t.Fatalf("seeding prod token: %v", err)
	}

	t.Setenv(config.EnvProfile, "staging")
	if got := runCLI(t, "staging-token\n", "auth", "set-token", "--stdin"); got.err != nil {
		t.Fatalf("auth set-token error = %v", got.err)
	}

	staging, err := config.LoadCredential("staging")
	if err != nil {
		t.Fatalf("LoadCredential(staging) error = %v", err)
	}
	if staging != "staging-token" {
		t.Errorf("staging token = %q, want the token stored against TT_PROFILE", staging)
	}
	prod, err := config.LoadCredential("prod")
	if err != nil {
		t.Fatalf("LoadCredential(prod) error = %v", err)
	}
	if prod != "prod-token" {
		t.Errorf("prod token = %q, want it untouched", prod)
	}
}

// clear-token is destructive, so it must resolve the profile the same way.
func TestClearTokenHonoursProfileEnvironment(t *testing.T) {
	isolate(t)

	for _, name := range []string{"prod", "staging"} {
		if got := runCLI(t, "", "profile", "set", name, "--url", "https://"+name+".example.com"); got.err != nil {
			t.Fatalf("profile set %s error = %v", name, got.err)
		}
		if err := config.StoreCredential(name, name+"-token"); err != nil {
			t.Fatalf("seeding %s token: %v", name, err)
		}
	}

	t.Setenv(config.EnvProfile, "staging")
	if got := runCLI(t, "", "auth", "clear-token"); got.err != nil {
		t.Fatalf("auth clear-token error = %v", got.err)
	}

	staging, err := config.LoadCredential("staging")
	if err != nil {
		t.Fatalf("LoadCredential(staging) error = %v", err)
	}
	if staging != "" {
		t.Errorf("staging token = %q, want it cleared", staging)
	}
	prod, err := config.LoadCredential("prod")
	if err != nil {
		t.Fatalf("LoadCredential(prod) error = %v", err)
	}
	if prod != "prod-token" {
		t.Errorf("prod token = %q, want the untargeted profile untouched", prod)
	}
}

// set-token with no profile named must target the current profile, not always
// "default", or storing a token for the selected site would silently miss.
func TestSetTokenDefaultsToTheCurrentProfile(t *testing.T) {
	isolate(t)

	if got := runCLI(t, "", "profile", "set", "prod", "--url", "https://tracker.example.com"); got.err != nil {
		t.Fatalf("profile set error = %v", got.err)
	}
	if got := runCLI(t, "token-abc\n", "auth", "set-token", "--stdin"); got.err != nil {
		t.Fatalf("profile set-token error = %v", got.err)
	}

	stored, err := config.LoadCredential("prod")
	if err != nil {
		t.Fatalf("LoadCredential() error = %v", err)
	}
	if stored != "token-abc" {
		t.Errorf("prod token = %q, want the token stored against the current profile", stored)
	}
}

// --timeout has to reach the HTTP client, or a cron job can still hang.
func TestTimeoutFlagIsApplied(t *testing.T) {
	isolate(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write([]byte(`{"user":{"username":"alice"}}`))
	}))
	defer srv.Close()

	got := runCLI(t, "", "whoami", "--url", srv.URL, "--token", "t", "--timeout", "30ms")
	if got.err == nil {
		t.Fatal("whoami succeeded against a slow server with a 30ms timeout, want an error")
	}
}
