package command

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/williamokano/go-torrent-trader/cli/internal/client"
	"github.com/williamokano/go-torrent-trader/cli/internal/config"
	"github.com/williamokano/go-torrent-trader/cli/internal/output"
)

// profileRow is one row of `tt profile list`.
type profileRow struct {
	Name    string `json:"name" yaml:"name"`
	URL     string `json:"url" yaml:"url"`
	Current bool   `json:"current" yaml:"current"`
	// HasToken reports whether a stored token exists, never the token itself.
	// Printing a credential because someone asked to list their profiles is how
	// tokens end up in terminal scrollback and CI logs.
	HasToken bool `json:"has_token" yaml:"has_token"`
}

type profileList []profileRow

func (l profileList) Table() output.Table {
	rows := make([][]string, 0, len(l))
	for _, p := range l {
		current := ""
		if p.Current {
			current = "*"
		}
		token := "no"
		if p.HasToken {
			token = "yes"
		}
		rows = append(rows, []string{current, p.Name, p.URL, token})
	}
	return output.Table{Headers: []string{"", "name", "url", "token"}, Rows: rows}
}

func newProfileCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage the sites tt can talk to",
		Long: "A profile is a named site. Profile definitions live in config.yaml;\n" +
			"tokens are managed separately with 'tt auth'.",
	}
	cmd.AddCommand(
		newProfileListCmd(g),
		newProfileSetCmd(),
		newProfileUseCmd(),
		newProfileRemoveCmd(),
	)
	return cmd
}

func newProfileListCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := g.printer(cmd)
			if err != nil {
				return err
			}
			f, err := config.Load()
			if err != nil {
				return err
			}

			current := config.ProfileName(f, g.profile)

			names := make([]string, 0, len(f.Profiles))
			for name := range f.Profiles {
				names = append(names, name)
			}
			sort.Strings(names)

			// Read the credential file once rather than per profile.
			stored, err := config.StoredProfiles()
			if err != nil {
				return err
			}

			list := make(profileList, 0, len(names))
			for _, name := range names {
				list = append(list, profileRow{
					Name:     name,
					URL:      f.Profiles[name].URL,
					Current:  name == current,
					HasToken: stored[name],
				})
			}
			return p.Print(list)
		},
	}
}

func newProfileSetCmd() *cobra.Command {
	var rawURL string
	cmd := &cobra.Command{
		Use:   "set <name>",
		Short: "Create or update a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := strings.TrimSpace(args[0])
			// An empty name produces a profile that can never be selected and
			// can never become current, so it is only ever a typo.
			if name == "" {
				return usageError{errors.New("profile name must not be empty")}
			}
			if rawURL == "" {
				return usageError{errors.New("--url is required")}
			}
			// Validate now rather than at first use: reporting "missing scheme"
			// when the profile is created points at the thing that was wrong,
			// whereas reporting it during `tt whoami` points at the wrong command.
			if _, err := client.New(rawURL); err != nil {
				return usageError{err}
			}

			// Read and write under one lock. Unlocked, two concurrent
			// `tt profile set` calls both read the old document and the second
			// write wins, so a profile disappears while both report success.
			var stored string
			if err := config.MutateSaved(func(f *config.File) error {
				f.Profiles[name] = config.Profile{URL: strings.TrimRight(strings.TrimSpace(rawURL), "/")}
				// The first profile created becomes the current one, so a fresh
				// install does not need a second command to be usable.
				if f.CurrentProfile == "" {
					f.CurrentProfile = name
				}
				stored = f.Profiles[name].URL
				return nil
			}); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "Profile %q set to %s\n", name, stored)
			return err
		},
	}
	cmd.Flags().StringVar(&rawURL, "url", "", "Site base URL, for example https://tracker.example.com")
	_ = cmd.MarkFlagRequired("url")
	return cmd
}

func newProfileUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "use <name>",
		Short:             "Select the profile used when none is given",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeProfileNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			// Existence is checked against the same read the write is based on,
			// so a profile removed concurrently cannot become the current one.
			if err := config.MutateSaved(func(f *config.File) error {
				if _, ok := f.Profiles[name]; !ok {
					return usageError{fmt.Errorf("%w: %q", config.ErrNoProfile, name)}
				}
				f.CurrentProfile = name
				return nil
			}); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "Now using profile %q\n", name)
			return err
		},
	}
}

func newProfileRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "remove <name>",
		Short:             "Delete a profile and its stored token",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeProfileNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			// Whole thing under the config lock, so a concurrent `profile set`
			// cannot reinstate the profile between the check and the write.
			var hadToken bool
			if err := config.MutateSaved(func(f *config.File) error {
				if _, ok := f.Profiles[name]; !ok {
					return usageError{fmt.Errorf("%w: %q", config.ErrNoProfile, name)}
				}

				// Delete the credential first. If this ran after the config write
				// and failed, the profile would already be gone from config.yaml
				// while a live token stayed on disk — and `profile list` only
				// walks config, so nothing would ever show it again.
				var err error
				if hadToken, err = config.HasCredential(name); err != nil {
					return err
				}
				if err := config.DeleteCredential(name); err != nil {
					return err
				}

				delete(f.Profiles, name)
				if f.CurrentProfile == name {
					f.CurrentProfile = ""
				}
				return nil
			}); err != nil {
				return err
			}

			msg := fmt.Sprintf("Removed profile %q\n", name)
			if hadToken {
				msg = fmt.Sprintf("Removed profile %q and its stored token\n", name)
			}
			_, err := fmt.Fprint(cmd.OutOrStdout(), msg)
			return err
		},
	}
}

// completeProfileNames offers the configured profile names to the shell.
func completeProfileNames(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	f, err := config.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	names := make([]string, 0, len(f.Profiles))
	for name := range f.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, cobra.ShellCompDirectiveNoFileComp
}
