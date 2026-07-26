// Package command builds the tt command tree.
//
// The tree lives here rather than in cmd/tt so it can be exercised in tests with
// its output captured, which is also why every command writes through
// cmd.OutOrStdout() instead of calling fmt.Print directly.
package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/williamokano/go-torrent-trader/cli/internal/client"
	"github.com/williamokano/go-torrent-trader/cli/internal/config"
	"github.com/williamokano/go-torrent-trader/cli/internal/output"
)

// Exit codes. These are an interface: a cron job's whole reason for calling a
// CLI is to branch on the outcome, and "everything is 1" forces it to regex
// stderr. Distinguishing auth from network matters most while the only
// credential a site issues expires after an hour.
const (
	ExitOK      = 0
	ExitError   = 1 // anything not covered below
	ExitUsage   = 2 // bad flags, bad arguments, unknown command
	ExitAuth    = 3 // no credential, or the site rejected it (401/403)
	ExitNetwork = 4 // unreachable, timed out, TLS failure
)

// globals holds the values of the persistent flags shared by every command.
type globals struct {
	profile string
	url     string
	token   string
	format  string
	timeout time.Duration
	// version is threaded through to the User-Agent so an operator reading
	// access logs can tell which CLI build produced a request.
	version string
}

// printer builds a Printer writing to the command's output stream.
func (g *globals) printer(cmd *cobra.Command) (output.Printer, error) {
	f, err := output.ParseFormat(g.format)
	if err != nil {
		return output.Printer{}, usageError{err}
	}
	return output.New(cmd.OutOrStdout(), f), nil
}

// resolve computes the effective site and credential for this invocation.
func (g *globals) resolve() (config.Resolved, error) {
	f, err := config.Load()
	if err != nil {
		return config.Resolved{}, err
	}
	return config.Resolve(f, config.Override{
		Profile: g.profile,
		URL:     g.url,
		Token:   g.token,
	})
}

// profileName resolves which profile a local command operates on.
//
// It goes through config.ProfileName so that flag, TT_PROFILE and the current
// profile are consulted in exactly the order Resolve uses. Duplicating that
// order is how the credential commands came to ignore TT_PROFILE while every
// other command honoured it.
func (g *globals) profileName(f *config.File, arg string) string {
	if arg != "" {
		return arg
	}
	return config.ProfileName(f, g.profile)
}

// warnIfPlaintext notes on stderr that a credential is about to cross an
// unencrypted connection.
//
// Not an error: http is legitimate against localhost during development, and
// against a host reachable only over a private link. But a bearer token that is
// also the site's full-account credential going out in cleartext to a remote host
// is worth one line, and silence reads as approval. Loopback is exempt because
// warning there would train people to ignore it.
//
// stderr so it never corrupts `-o json` piped into jq.
func warnIfPlaintext(w io.Writer, rawURL string) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "http" {
		return
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return
	}
	_, _ = fmt.Fprintf(w, "tt: warning: %s is not https, so the token is sent in cleartext\n", rawURL)
}

// userAgent identifies this build in the site's access logs.
func (g *globals) userAgent() string { return "tt/" + g.version }

// httpClient builds the transport for one invocation.
func (g *globals) httpClient() *http.Client {
	return &http.Client{Timeout: g.timeout}
}

// checkTimeout rejects a non-positive timeout.
//
// http.Client treats one as "no timeout", which is the exact hang the flag
// exists to prevent. Many tools read 0 as "use the default", so refusing beats
// silently doing the opposite of what a reader expects.
func (g *globals) checkTimeout() error {
	if g.timeout <= 0 {
		return usageError{fmt.Errorf("--timeout must be positive, got %s", g.timeout)}
	}
	return nil
}

// resolveSite resolves just the URL for a named profile, for commands that
// establish a credential rather than use one.
func (g *globals) resolveSite(f *config.File, name string) (config.Resolved, error) {
	if err := g.checkTimeout(); err != nil {
		return config.Resolved{}, err
	}
	return config.ResolveSite(f, name, g.url)
}

// authClient builds a client for an endpoint that requires authentication.
//
// It returns an actionable error when no credential is configured rather than
// letting the request go out anonymously and surface as a bare 401.
func (g *globals) authClient() (*client.Client, config.Resolved, error) {
	if err := g.checkTimeout(); err != nil {
		return nil, config.Resolved{}, err
	}

	resolved, err := g.resolve()
	if err != nil {
		return nil, config.Resolved{}, err
	}

	opts := []client.Option{
		client.WithUserAgent(g.userAgent()),
		client.WithHTTPClient(g.httpClient()),
	}
	// Only a credential this profile actually owns may be refreshed. An
	// explicit --token or TT_TOKEN belongs to the caller, and silently
	// replacing it with a stored profile's session would be a surprise.
	token := resolved.Token
	if resolved.FromStore {
		stored, storeErr := config.LoadCredentialRecord(resolved.Profile)
		if storeErr != nil {
			return nil, config.Resolved{}, storeErr
		}
		if refresher := refresherFor(resolved.Profile, resolved.URL, stored,
			client.WithUserAgent(g.userAgent()), client.WithHTTPClient(g.httpClient())); refresher != nil {
			opts = append(opts, client.WithRefresher(refresher))
			// Renew up front when the token is already spent, rather than
			// spending a request to be told what the expiry already says.
			if stored.ExpiresWithin(refreshSkew) {
				if fresh, refreshErr := refresher(context.Background()); refreshErr == nil {
					token = fresh
				}
			}
		}
	}

	c, err := client.NewAuthenticated(resolved.URL, token, opts...)
	if err != nil {
		if errors.Is(err, client.ErrNoCredentials) {
			return nil, config.Resolved{}, authError{fmt.Errorf(
				"no token for profile %q: run 'tt auth login %s', export %s, pass --token, or run 'tt auth set-token %s'",
				resolved.Profile, resolved.Profile, config.EnvToken, resolved.Profile)}
		}
		return nil, config.Resolved{}, usageError{err}
	}
	return c, resolved, nil
}

// usageError marks a mistake in how the command was invoked.
type usageError struct{ error }

func (e usageError) Unwrap() error { return e.error }

// authError marks a missing or rejected credential.
type authError struct{ error }

func (e authError) Unwrap() error { return e.error }

// NewRoot builds the tt command tree.
func NewRoot(version string) *cobra.Command {
	g := &globals{version: version}

	root := &cobra.Command{
		Use:   "tt",
		Short: "Administer a go-torrent-trader site from the terminal",
		Long: "tt drives a go-torrent-trader site over its REST API.\n\n" +
			"Everything it does is also reachable in the web interface; the point is\n" +
			"automation — cron jobs, shell pipelines, CI, and operators on a box with\n" +
			"no browser.",
		Version: version,
		// A failed API call is not a usage mistake, so do not answer one with a
		// wall of usage text. Cobra still prints usage for genuine flag errors.
		SilenceUsage: true,
		// Errors are printed once by Execute, which also decides the exit code.
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVar(&g.profile, "profile", "",
		"Configuration profile to use (default: the current profile)")
	root.PersistentFlags().StringVar(&g.url, "url", "",
		"Site base URL, overriding the profile")
	root.PersistentFlags().StringVar(&g.token, "token", "",
		"Bearer token, overriding the profile. Prefer "+config.EnvToken+" so the token stays out of shell history")
	root.PersistentFlags().StringVarP(&g.format, "output", "o", string(output.FormatTable),
		"Output format: "+strings.Join(output.Formats(), ", "))
	root.PersistentFlags().DurationVar(&g.timeout, "timeout", client.DefaultTimeout,
		"Per-request timeout. A cron job must not hang forever on an unreachable site")

	if err := root.RegisterFlagCompletionFunc("output",
		func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return output.Formats(), cobra.ShellCompDirectiveNoFileComp
		}); err != nil {
		// Only fails when the flag is undefined, which the line above guarantees.
		panic(err)
	}

	root.AddCommand(
		newVersionCmd(g),
		newProfileCmd(g),
		newAuthCmd(g),
		newWhoamiCmd(g),
	)
	tagArgumentErrors(root)
	return root
}

// tagArgumentErrors marks every command's argument-validation failure as a usage
// error, and gives grouping commands the argument check cobra does not.
//
// Cobra routes flag errors through SetFlagErrorFunc but returns argument errors
// as plain values, so "too many arguments" would otherwise be indistinguishable
// from a failed API call. Walking the tree here means a command added later is
// classified correctly without its author having to remember.
//
// The second case is the one that matters, because its absence made `tt` report
// success for a typo. When Args is nil cobra falls back to legacyArgs, which
// returns an error for an unknown subcommand of the *root* and nil for an unknown
// subcommand of anything else — so `tt profile lst` printed help and exited 0. A
// cron wrapper written as `tt profile lst || alert` never fires. Root was wrong
// too, in a smaller way: its error was real but untagged, so a mistyped command
// exited 1 (general failure) where a mistyped *flag* exited 2.
//
// Handled here rather than by putting cobra.NoArgs on each grouping command,
// because that relies on the author of the next one remembering — which is the
// same fragility this function exists to remove.
// Two cobra details make this fiddlier than it looks, and both were load-bearing
// in the bug. Command.Find only consults legacyArgs when Args is nil, so setting
// Args moves validation from Find into execute; and execute returns flag.ErrHelp
// for a command that is not Runnable *before* it calls ValidateArgs. A grouping
// command therefore needs both an Args check and a RunE, or the check is never
// reached and the typo still exits 0.
func tagArgumentErrors(cmd *cobra.Command) {
	// Read before RunE is assigned below, or every group looks runnable.
	runnable := cmd.Runnable()

	if validate := cmd.Args; validate != nil {
		cmd.Args = func(c *cobra.Command, args []string) error {
			if err := validate(c, args); err != nil {
				return usageError{err}
			}
			return nil
		}
	} else if cmd.HasSubCommands() {
		// Groups subcommands, so a leftover argument is a mistyped one. Leaf
		// commands keep nil Args and cobra's accept-anything default.
		cmd.Args = func(c *cobra.Command, args []string) error {
			if len(args) == 0 {
				return nil
			}
			return usageError{fmt.Errorf("unknown command %q for %q%s",
				args[0], c.CommandPath(), suggestionsFor(c, args[0]))}
		}
	}

	if cmd.HasSubCommands() && !runnable {
		// Makes ValidateArgs reachable. Only ever runs with no leftover
		// arguments, since the check above rejects the rest, so it reproduces
		// what cobra's non-runnable path did: show help and succeed.
		cmd.RunE = func(c *cobra.Command, _ []string) error { return c.Help() }
	}

	for _, child := range cmd.Commands() {
		tagArgumentErrors(child)
	}
}

// suggestionsFor reproduces the "Did you mean this?" block cobra appends to its
// own unknown-command error, which is unexported. Worth keeping: the whole point
// of failing on a typo is helping the operator fix it.
func suggestionsFor(cmd *cobra.Command, arg string) string {
	if cmd.DisableSuggestions {
		return ""
	}
	if cmd.SuggestionsMinimumDistance <= 0 {
		cmd.SuggestionsMinimumDistance = 2
	}
	suggestions := cmd.SuggestionsFor(arg)
	if len(suggestions) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n\nDid you mean this?\n")
	for _, s := range suggestions {
		_, _ = fmt.Fprintf(&sb, "\t%v\n", s)
	}
	return sb.String()
}

// exitCode classifies a command failure.
func exitCode(err error) int {
	if err == nil {
		return ExitOK
	}

	var usage usageError
	if errors.As(err, &usage) {
		return ExitUsage
	}
	var auth authError
	if errors.As(err, &auth) {
		return ExitAuth
	}
	if errors.Is(err, config.ErrTokenHostMismatch) {
		return ExitAuth
	}

	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden {
			return ExitAuth
		}
		// Any other status came from the site, so it answered: not a network
		// problem, whatever else it is.
		return ExitError
	}

	// Checked before net.Error, which *url.Error satisfies: a refused redirect
	// means the site answered and answered wrongly, so reporting it as
	// unreachable would have a cron wrapper retry an outage that is not happening.
	if errors.Is(err, client.ErrRedirect) {
		return ExitError
	}

	// A timeout or a refused connection means the site was never reached.
	var netErr net.Error
	if errors.As(err, &netErr) || errors.Is(err, os.ErrDeadlineExceeded) {
		return ExitNetwork
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ExitNetwork
	}
	return ExitError
}

// Execute runs the tree and reports the process exit code.
func Execute(version string, out, errOut io.Writer, args []string) int {
	root := NewRoot(version)
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs(args)
	// Cobra returns flag and argument errors as plain errors; tagging them here
	// keeps the classification in one place.
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError{err} })

	if err := root.Execute(); err != nil {
		// Nothing useful to do if stderr itself is broken.
		_, _ = fmt.Fprintf(errOut, "tt: %v\n", err)
		return exitCode(err)
	}
	return ExitOK
}
