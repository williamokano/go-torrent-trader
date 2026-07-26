package command

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/williamokano/go-torrent-trader/cli/internal/config"
)

// The no-echo prompt is the security-relevant half of readToken and would
// otherwise go untested, because exercising it for real needs a pseudo-terminal.
// The terminal check and the password read are indirected so both can be driven.
func TestReadTokenPromptsWithoutEchoOnATerminal(t *testing.T) {
	var prompted bool
	restore := stubTerminal(t, true, func(*os.File) ([]byte, error) {
		prompted = true
		return []byte("typed-token\n"), nil
	})
	defer restore()

	cmd := &cobra.Command{}
	var errOut strings.Builder
	cmd.SetErr(&errOut)
	// InOrStdin must be an *os.File for the terminal branch to be considered.
	cmd.SetIn(os.Stdin)

	got, err := readToken(cmd, false)
	if err != nil {
		t.Fatalf("readToken() error = %v", err)
	}
	if !prompted {
		t.Fatal("readToken did not use the no-echo password read")
	}
	if got != "typed-token" {
		t.Errorf("token = %q, want typed-token", got)
	}
	// The prompt belongs on stderr so `tt auth set-token > file` still shows it.
	if !strings.Contains(errOut.String(), "Token:") {
		t.Errorf("stderr = %q, want the prompt", errOut.String())
	}
}

// The token must never be echoed back to the user's screen.
func TestReadTokenNeverEchoesTheSecret(t *testing.T) {
	restore := stubTerminal(t, true, func(*os.File) ([]byte, error) {
		return []byte("super-secret"), nil
	})
	defer restore()

	cmd := &cobra.Command{}
	var out, errOut strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetIn(os.Stdin)

	if _, err := readToken(cmd, false); err != nil {
		t.Fatalf("readToken() error = %v", err)
	}
	if strings.Contains(out.String()+errOut.String(), "super-secret") {
		t.Error("readToken echoed the token")
	}
}

func TestReadTokenReportsAFailedRead(t *testing.T) {
	restore := stubTerminal(t, true, func(*os.File) ([]byte, error) {
		return nil, errors.New("tty exploded")
	})
	defer restore()

	cmd := &cobra.Command{}
	cmd.SetErr(&strings.Builder{})
	cmd.SetIn(os.Stdin)

	if _, err := readToken(cmd, false); err == nil {
		t.Fatal("readToken() succeeded despite a failing read, want an error")
	}
}

// --stdin must bypass the prompt even when stdin is a terminal, so a script that
// says it is piping is never left waiting on a prompt nobody will see.
func TestReadTokenStdinFlagBypassesThePrompt(t *testing.T) {
	restore := stubTerminal(t, true, func(*os.File) ([]byte, error) {
		t.Error("readToken prompted despite --stdin")
		return nil, nil
	})
	defer restore()

	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("piped-token\n"))

	got, err := readToken(cmd, true)
	if err != nil {
		t.Fatalf("readToken() error = %v", err)
	}
	if got != "piped-token" {
		t.Errorf("token = %q, want piped-token", got)
	}
}

func stubTerminal(t *testing.T, terminal bool, read func(*os.File) ([]byte, error)) func() {
	t.Helper()
	origIsTerminal, origRead := isTerminal, readPassword
	isTerminal = func(*os.File) bool { return terminal }
	readPassword = read
	return func() { isTerminal, readPassword = origIsTerminal, origRead }
}

// The token column must be able to say "no". Hardcoding it to "yes" passed every
// other test, and an operator who believes a profile is credentialled finds out
// when the scheduled job 401s.
func TestProfileListReportsAMissingToken(t *testing.T) {
	isolate(t)

	if got := runCLI(t, "", "profile", "set", "prod", "--url", "https://tracker.example.com"); got.err != nil {
		t.Fatalf("profile set error = %v", got.err)
	}
	if got := runCLI(t, "", "profile", "set", "staging", "--url", "https://staging.example.com"); got.err != nil {
		t.Fatalf("profile set error = %v", got.err)
	}
	if err := config.StoreCredential("prod", "token-abc"); err != nil {
		t.Fatalf("StoreCredential() error = %v", err)
	}

	got := runCLI(t, "", "profile", "list", "-o", "json")
	if got.err != nil {
		t.Fatalf("profile list error = %v", got.err)
	}
	if !strings.Contains(got.stdout, `"has_token": true`) {
		t.Errorf("list = %q, want prod reported as holding a token", got.stdout)
	}
	if !strings.Contains(got.stdout, `"has_token": false`) {
		t.Errorf("list = %q, want staging reported as holding none", got.stdout)
	}
}

// `profile remove` should not claim it deleted a token that never existed.
func TestProfileRemoveDoesNotClaimAnAbsentToken(t *testing.T) {
	isolate(t)

	if got := runCLI(t, "", "profile", "set", "prod", "--url", "https://tracker.example.com"); got.err != nil {
		t.Fatalf("profile set error = %v", got.err)
	}

	got := runCLI(t, "", "profile", "remove", "prod")
	if got.err != nil {
		t.Fatalf("profile remove error = %v", got.err)
	}
	if strings.Contains(got.stdout, "stored token") {
		t.Errorf("output = %q, want no claim about a token that was never stored", got.stdout)
	}
}

// A profile URL that cannot work must be rejected when it is set, not at first
// use — the error then points at the command that was actually wrong.
func TestProfileSetValidatesTheURL(t *testing.T) {
	isolate(t)

	for _, bad := range []string{"tracker.example.com", "ftp://tracker.example.com", "https://tracker.example.com/?x=1"} {
		if got := runCLI(t, "", "profile", "set", "prod", "--url", bad); got.err == nil {
			t.Errorf("profile set --url %q succeeded, want an error", bad)
		}
	}
}

func TestProfileSetRejectsAnEmptyName(t *testing.T) {
	isolate(t)

	if got := runCLI(t, "", "profile", "set", "", "--url", "https://tracker.example.com"); got.err == nil {
		t.Fatal("profile set with an empty name succeeded, want an error")
	}
}
