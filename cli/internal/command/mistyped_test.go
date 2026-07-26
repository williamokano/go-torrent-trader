package command

import (
	"bytes"
	"strings"
	"testing"
)

// The exit codes are documented as an interface, and a mistyped subcommand
// returned 0 — the single most likely operator error reported as success. A cron
// wrapper written as `tt profile lst || alert` never fires.
//
// Cobra's legacyArgs is the reason: it errors for an unknown subcommand of the
// root and returns nil for an unknown subcommand of anything else, so the
// grouping commands accepted whatever was typed and printed help.
func TestAMistypedSubcommandIsAUsageError(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "under a group", args: []string{"profile", "lst"}, want: `unknown command "lst" for "tt profile"`},
		{name: "under another group", args: []string{"auth", "bogus"}, want: `unknown command "bogus" for "tt auth"`},
		// This one errored already, but as ExitError rather than ExitUsage — so a
		// mistyped *command* and a mistyped *flag* were classified differently.
		{name: "at the top level", args: []string{"bogus"}, want: `unknown command "bogus" for "tt"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolate(t)
			var out, errOut bytes.Buffer

			code := Execute("test", &out, &errOut, tc.args)

			if code != ExitUsage {
				t.Errorf("exit code = %d, want %d (usage); stdout had %d bytes",
					code, ExitUsage, out.Len())
			}
			if got := errOut.String(); !strings.Contains(got, tc.want) {
				t.Errorf("stderr = %q, want it to contain %q", got, tc.want)
			}
		})
	}
}

// The suggestion cobra appends to its own unknown-command error has to survive,
// since the point of failing on a typo is helping the operator fix it.
func TestAMistypedCommandStillSuggestsTheRealOne(t *testing.T) {
	isolate(t)
	var out, errOut bytes.Buffer

	Execute("test", &out, &errOut, []string{"profil"})

	if got := errOut.String(); !strings.Contains(got, "Did you mean this?") ||
		!strings.Contains(got, "profile") {
		t.Errorf("stderr = %q, want a suggestion of \"profile\"", got)
	}
}

// Invoking a group with no subcommand is not an error — it is how you discover
// what the group offers. This is the behaviour the fix above had to preserve,
// and asserting it is what stops the fix turning every `tt profile` into a failure.
func TestAGroupWithNoSubcommandPrintsHelpAndSucceeds(t *testing.T) {
	for _, group := range []string{"profile", "auth"} {
		t.Run(group, func(t *testing.T) {
			isolate(t)
			var out, errOut bytes.Buffer

			code := Execute("test", &out, &errOut, []string{group})

			if code != ExitOK {
				t.Errorf("exit code = %d, want 0; stderr: %s", code, errOut.String())
			}
			if !strings.Contains(out.String(), "Usage:") {
				t.Errorf("stdout = %q, want help text", out.String())
			}
		})
	}
}

// A bare `tt` must still print help rather than failing, since the root is a
// grouping command too and got the same treatment.
func TestBareInvocationPrintsHelpAndSucceeds(t *testing.T) {
	isolate(t)
	var out, errOut bytes.Buffer

	if code := Execute("test", &out, &errOut, nil); code != ExitOK {
		t.Errorf("exit code = %d, want 0; stderr: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Errorf("stdout = %q, want help text", out.String())
	}
}

// A token going out over plaintext to a remote host deserves a line on stderr.
// stderr specifically, so `-o json | jq` is unaffected.
func TestPlaintextWarningFiresOnlyForRemoteHosts(t *testing.T) {
	for _, tc := range []struct {
		url  string
		warn bool
	}{
		{url: "https://tracker.example.com", warn: false},
		{url: "http://localhost:8080", warn: false},
		{url: "http://127.0.0.1:8080", warn: false},
		{url: "http://[::1]:8080", warn: false},
		{url: "http://tracker.example.com", warn: true},
		{url: "http://192.168.1.10:8080", warn: true},
	} {
		t.Run(tc.url, func(t *testing.T) {
			var buf bytes.Buffer
			warnIfPlaintext(&buf, tc.url)
			warned := strings.Contains(buf.String(), "cleartext")
			if warned != tc.warn {
				t.Errorf("warned = %v for %q, want %v (output %q)",
					warned, tc.url, tc.warn, buf.String())
			}
		})
	}
}
