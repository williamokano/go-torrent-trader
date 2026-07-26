package command

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/williamokano/go-torrent-trader/cli/internal/client"
	"github.com/williamokano/go-torrent-trader/cli/internal/config"
)

// EnvPassword lets a script supply a password without a terminal.
const EnvPassword = "TT_PASSWORD"

// refreshSkew renews a token slightly before it expires, so a request is not
// sent with a credential that will have died by the time it lands.
const refreshSkew = 30 * time.Second

func newAuthLoginCmd(g *globals) *cobra.Command {
	var username string
	var passwordStdin bool

	cmd := &cobra.Command{
		Use:   "login [profile]",
		Short: "Log in to a site and store the resulting token",
		Long: "Exchanges a username and password for a token pair via\n" +
			"/api/v1/auth/login, and stores it for the profile.\n\n" +
			"The password is used once and never written anywhere. The access token\n" +
			"it returns expires after an hour, so the refresh token is stored\n" +
			"alongside it and tt renews the access token by itself — that is what\n" +
			"makes the CLI usable from cron rather than only at a terminal.\n\n" +
			"For automation, prefer 'tt auth set-token' with a scoped API key once\n" +
			"those exist (#170): a key can be revoked on its own and carries only the\n" +
			"permissions it was granted, where a login carries all of yours.",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeProfileNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := config.Load()
			if err != nil {
				return err
			}
			name := g.profileName(f, firstArg(args))

			resolved, err := g.resolveSite(f, name)
			if err != nil {
				return err
			}

			// One buffered reader for the whole command. Wrapping stdin twice
			// would let the first read buffer the password and throw it away,
			// which breaks every piped invocation.
			in := bufio.NewReader(cmd.InOrStdin())

			if username == "" {
				if username, err = promptLine(cmd, in, "Username: "); err != nil {
					return err
				}
			}
			if username == "" {
				return usageError{errors.New("username is required")}
			}

			password, err := readPasswordInput(cmd, in, passwordStdin)
			if err != nil {
				return err
			}
			if password == "" {
				return usageError{errors.New("password is required")}
			}

			tokens, err := client.Login(cmd.Context(), resolved.URL, username, password,
				client.WithUserAgent(g.userAgent()), client.WithHTTPClient(g.httpClient()))
			if err != nil {
				// A rejected password is an auth failure, not a usage mistake, so
				// a scheduled wrapper can tell it from a malformed command.
				return authError{err}
			}

			if err := config.StoreCredentialRecord(name, config.Credential{
				Token:        tokens.AccessToken,
				RefreshToken: tokens.RefreshToken,
				ExpiresAt:    tokens.ExpiresAt(time.Now()),
			}); err != nil {
				return err
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Logged in to %s as %s (profile %q)\n",
				resolved.URL, username, name)
			return err
		},
	}

	cmd.Flags().StringVarP(&username, "username", "u", "", "Username, prompted for when omitted")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false,
		"Read the password from stdin instead of prompting. Prefer this or "+EnvPassword+" over any flag, which would land in shell history")
	return cmd
}

func newAuthLogoutCmd(g *globals) *cobra.Command {
	var local bool

	cmd := &cobra.Command{
		Use:   "logout [profile]",
		Short: "Invalidate the session and delete the stored credential",
		Long: "Calls /api/v1/auth/logout so the session is actually dead on the server,\n" +
			"then removes the local credential.\n\n" +
			"Deleting the local file alone would leave a refresh token valid for its\n" +
			"full lifetime on the server — a credential nobody can see and nobody\n" +
			"revoked. Use --local to skip the server call when the site is\n" +
			"unreachable and you only want the file gone.",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeProfileNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := config.Load()
			if err != nil {
				return err
			}
			name := g.profileName(f, firstArg(args))

			stored, err := config.LoadCredentialRecord(name)
			if err != nil {
				return err
			}
			if stored.Token == "" {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "No stored credential for profile %q\n", name)
				return err
			}

			var serverErr error
			if !local {
				serverErr = g.revokeSession(cmd.Context(), f, name, stored)
			}

			// Delete locally whatever the server said. Keeping the credential
			// after the user asked to log out is the worse failure, and a
			// revocation that did not happen is reported rather than hidden.
			if err := config.DeleteCredential(name); err != nil {
				return err
			}

			if serverErr != nil {
				_, printErr := fmt.Fprintf(cmd.ErrOrStderr(),
					"Removed the local credential for %q, but the session could not be invalidated on the server: %v\n"+
						"It stays valid until it expires. Revoke it from the site if that matters.\n", name, serverErr)
				return printErr
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Logged out of profile %q\n", name)
			return err
		},
	}

	cmd.Flags().BoolVar(&local, "local", false,
		"Only delete the local credential, leaving the session valid on the server")
	return cmd
}

// revokeSession asks the site to invalidate a session, renewing the access token
// first when it has already expired.
//
// The renewal is not an optimisation. The server revokes a session by looking it
// up from its access token and gives up if that lookup fails, so logging out
// with an expired one revokes nothing — while this command still deletes the
// local file, leaving the 30-day refresh token alive with no copy left to revoke
// it with. Logging out the morning after logging in is the ordinary case, so
// that must not be the ordinary outcome.
func (g *globals) revokeSession(ctx context.Context, f *config.File, name string, stored config.Credential) error {
	resolved, err := g.resolveSite(f, name)
	if err != nil {
		return err
	}

	opts := []client.Option{
		client.WithUserAgent(g.userAgent()),
		client.WithHTTPClient(g.httpClient()),
	}
	token := stored.Token
	if refresher := refresherFor(name, resolved.URL, stored,
		client.WithUserAgent(g.userAgent()), client.WithHTTPClient(g.httpClient())); refresher != nil {
		opts = append(opts, client.WithRefresher(refresher))
		if stored.ExpiresWithin(refreshSkew) {
			// A failed renewal is not reported here: it usually means the whole
			// session is already gone, and there is nothing left to revoke. Send
			// the stale token anyway and let the server's answer decide what the
			// caller is told.
			if fresh, refreshErr := refresher(ctx); refreshErr == nil {
				token = fresh
			}
		}
	}

	c, err := client.NewAuthenticated(resolved.URL, token, opts...)
	if err != nil {
		return err
	}
	return c.Logout(ctx)
}

// refresherFor returns a Refresher that renews a profile's token and persists
// the new pair, or nil when the credential cannot refresh itself.
//
// A pasted API key has no refresh token and never needs one, so it gets nil and
// a 401 surfaces immediately rather than after a pointless round trip.
func refresherFor(profileName, baseURL string, stored config.Credential, opts ...client.Option) client.Refresher {
	if !stored.CanRefresh() {
		return nil
	}
	return func(ctx context.Context) (string, error) {
		tokens, err := client.Refresh(ctx, baseURL, stored.RefreshToken, opts...)
		if err != nil {
			return "", err
		}
		// The refresh token rotates, so failing to persist would leave the old
		// one on disk and the next invocation would have to log in again.
		if err := config.StoreCredentialRecord(profileName, config.Credential{
			Token:        tokens.AccessToken,
			RefreshToken: tokens.RefreshToken,
			ExpiresAt:    tokens.ExpiresAt(time.Now()),
		}); err != nil {
			return "", err
		}
		return tokens.AccessToken, nil
	}
}

// promptLine reads a visible line, used for the username.
func promptLine(cmd *cobra.Command, in *bufio.Reader, prompt string) (string, error) {
	if _, err := fmt.Fprint(cmd.ErrOrStderr(), prompt); err != nil {
		return "", err
	}
	return readBufferedLine(in)
}

// readPasswordInput reads a password without echoing it, or from stdin.
func readPasswordInput(cmd *cobra.Command, in *bufio.Reader, fromStdin bool) (string, error) {
	if env := os.Getenv(EnvPassword); env != "" && !fromStdin {
		return strings.TrimSpace(env), nil
	}
	if !fromStdin {
		if f, ok := cmd.InOrStdin().(*os.File); ok && isTerminal(f) {
			if _, err := fmt.Fprint(cmd.ErrOrStderr(), "Password: "); err != nil {
				return "", err
			}
			raw, err := readPassword(f)
			if _, printErr := fmt.Fprintln(cmd.ErrOrStderr()); printErr != nil {
				return "", printErr
			}
			if err != nil {
				return "", fmt.Errorf("reading password: %w", err)
			}
			return strings.TrimSpace(string(raw)), nil
		}
	}
	return readBufferedLine(in)
}
