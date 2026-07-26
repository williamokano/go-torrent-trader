package command

import (
	"encoding/json"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/williamokano/go-torrent-trader/cli/internal/output"
)

// ownerSummary names only the fields the table shows.
//
// It is deliberately not a full mirror of the OwnerProfile schema: --output json
// emits the API's own bytes (see output.Passthrough), so this struct never
// decides what a script can see. Adding a field here adds a column, nothing more.
type ownerSummary struct {
	Username   string  `json:"username"`
	Uploaded   int64   `json:"uploaded"`
	Downloaded int64   `json:"downloaded"`
	Ratio      float64 `json:"ratio"`
	Invites    int     `json:"invites"`
	Warned     bool    `json:"warned"`
}

type meResponse struct {
	User ownerSummary `json:"user"`
}

// table summarises the identity. Full detail is available with --output json.
//
// The passkey is not a column. It is the tracker download credential embedded in
// every announce URL, and a terminal is a shared, scrolled-back, screen-shared
// place; showing it because someone asked "who am I" is not a trade worth making.
func (m meResponse) table() output.Table {
	u := m.User
	warned := "no"
	if u.Warned {
		warned = "yes"
	}
	return output.Table{
		Headers: []string{"username", "uploaded", "downloaded", "ratio", "invites", "warned"},
		Rows: [][]string{{
			u.Username,
			output.Bytes(u.Uploaded),
			output.Bytes(u.Downloaded),
			output.Ratio(u.Ratio),
			strconv.Itoa(u.Invites),
			warned,
		}},
	}
}

func newWhoamiCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the account the configured token belongs to",
		Long: "Calls /api/v1/auth/me. This is the quickest way to confirm a profile's\n" +
			"URL and token are both working before scripting anything against them.\n\n" +
			"The table is a summary. --output json returns the profile as the API sent\n" +
			"it, which includes your passkey — redirect it with care.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := g.printer(cmd)
			if err != nil {
				return err
			}
			c, resolved, err := g.authClient(cmd)
			if err != nil {
				return err
			}
			warnIfPlaintext(cmd.ErrOrStderr(), resolved.URL)

			var raw json.RawMessage
			if err := c.Get(cmd.Context(), "/api/v1/auth/me", &raw); err != nil {
				return err
			}
			var me meResponse
			if err := json.Unmarshal(raw, &me); err != nil {
				return err
			}
			return p.Print(output.Passthrough{Raw: raw, Summary: me.table()})
		},
	}
}
