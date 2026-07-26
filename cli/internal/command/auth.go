package command

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/williamokano/go-torrent-trader/cli/internal/config"
)

// newAuthCmd groups everything that touches a credential.
//
// Credentials live under `tt auth`, not `tt profile`, because `tt profile` is
// about which sites exist and these commands are about who you are. The split
// also leaves an obvious home for server-side key management when scoped API
// tokens land (#170): `tt auth create-key` sits naturally beside
// `tt auth set-token`, whereas `tt profile create-key` would not.
func newAuthCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage the credential used for a site",
		Long: "Tokens are stored in credentials.yaml with mode 0600, separately from\n" +
			"the profile definitions in config.yaml, so sharing a config never means\n" +
			"sharing a credential.",
	}
	cmd.AddCommand(
		newAuthLoginCmd(g),
		newAuthLogoutCmd(g),
		newAuthSetTokenCmd(g),
		newAuthClearTokenCmd(g),
	)
	return cmd
}

func newAuthSetTokenCmd(g *globals) *cobra.Command {
	var fromStdin bool
	cmd := &cobra.Command{
		Use:   "set-token [profile]",
		Short: "Store the bearer token for a profile",
		Long: "Reads a token from the terminal without echoing it, or from stdin with\n" +
			"--stdin, and stores it in credentials.yaml with mode 0600.\n\n" +
			"The token is whatever the site issued. Today that is a session access\n" +
			"token, obtainable with:\n\n" +
			"  curl -sX POST https://tracker.example.com/api/v1/auth/login \\\n" +
			"    -H 'Content-Type: application/json' \\\n" +
			"    -d '{\"username\":\"you\",\"password\":\"...\"}' \\\n" +
			"    | jq -r .tokens.access_token\n\n" +
			"That token expires in an hour, which makes it fine for interactive use\n" +
			"and poor for cron. Long-lived scoped API keys are issue #170; when they\n" +
			"exist this command stores one with no change.",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeProfileNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := config.Load()
			if err != nil {
				return err
			}
			name := g.profileName(f, firstArg(args))

			token, err := readToken(cmd, fromStdin)
			if err != nil {
				return err
			}
			if token == "" {
				return usageError{errors.New("no token supplied")}
			}
			warnIfDiscardingARefreshableSession(cmd, name)
			if err := config.StoreCredential(name, token); err != nil {
				return err
			}

			path, err := config.CredentialsPath()
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Stored token for profile %q in %s\n", name, path)
			return err
		},
	}
	cmd.Flags().BoolVar(&fromStdin, "stdin", false,
		"Read the token from stdin instead of prompting, for scripts and CI")
	return cmd
}

func newAuthClearTokenCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:               "clear-token [profile]",
		Short:             "Delete the stored token for a profile",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeProfileNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := config.Load()
			if err != nil {
				return err
			}
			name := g.profileName(f, firstArg(args))
			warnIfDiscardingARefreshableSession(cmd, name)
			if err := config.DeleteCredential(name); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Cleared stored token for profile %q\n", name)
			return err
		},
	}
}

// readToken reads a secret without putting it in argv, where it would be visible
// to every other process on the box and saved into shell history.
func readToken(cmd *cobra.Command, fromStdin bool) (string, error) {
	in := cmd.InOrStdin()

	if !fromStdin {
		if f, ok := in.(*os.File); ok && isTerminal(f) {
			if _, err := fmt.Fprint(cmd.ErrOrStderr(), "Token: "); err != nil {
				return "", err
			}
			raw, err := readPassword(f)
			if _, printErr := fmt.Fprintln(cmd.ErrOrStderr()); printErr != nil {
				return "", printErr
			}
			if err != nil {
				return "", fmt.Errorf("reading token: %w", err)
			}
			return strings.TrimSpace(string(raw)), nil
		}
	}

	// Not a terminal, or --stdin: read one line. Piping is the CI path.
	return readLine(in)
}

// readLine reads a single line, treating EOF as the end of the value so a
// here-string with no trailing newline works.
func readLine(in io.Reader) (string, error) {
	return readBufferedLine(bufio.NewReader(in))
}

// readBufferedLine reads one line from an already-buffered reader.
//
// Callers that read more than once must share the reader: a second
// bufio.NewReader over the same stream would find the first one had already
// buffered — and discarded — the input it needs.
func readBufferedLine(in *bufio.Reader) (string, error) {
	line, err := in.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("reading from stdin: %w", err)
	}
	return strings.TrimSpace(line), nil
}

// Indirected so the no-echo path can be exercised without a pseudo-terminal.
var (
	isTerminal   = func(f *os.File) bool { return term.IsTerminal(int(f.Fd())) }
	readPassword = func(f *os.File) ([]byte, error) { return term.ReadPassword(int(f.Fd())) }
)

func firstArg(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}

// warnIfDiscardingARefreshableSession notes that overwriting or deleting a stored
// credential leaves a live server-side session behind.
//
// Neither set-token nor clear-token revokes anything, so replacing a login record
// with a pasted API key — or clearing it — leaves precisely the state
// `tt auth logout` was written to prevent: a 30-day refresh token nobody can see
// and nobody revoked. Not refused, because the operator may well be deliberately
// swapping credentials and may not have the site reachable; but it must not be
// silent, since the whole argument for logout is that an unrevoked session is the
// worse failure.
func warnIfDiscardingARefreshableSession(cmd *cobra.Command, name string) {
	stored, err := config.LoadCredentialRecord(name)
	if err != nil || !stored.CanRefresh() {
		return
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
		"tt: warning: profile %q holds a login session with a refresh token, and this "+
			"does not revoke it.\ntt: the session stays valid on the server until it "+
			"expires — run 'tt auth logout %s' first to end it.\n", name, name)
}
