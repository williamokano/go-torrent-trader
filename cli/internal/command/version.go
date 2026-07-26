package command

import (
	"runtime"

	"github.com/spf13/cobra"

	"github.com/williamokano/go-torrent-trader/cli/internal/output"
)

// versionInfo is the payload of `tt version`, shaped so --output json is useful
// in a bug report.
type versionInfo struct {
	Version   string `json:"version" yaml:"version"`
	GoVersion string `json:"go_version" yaml:"go_version"`
	Platform  string `json:"platform" yaml:"platform"`
}

// Table renders the version as one row, so `tt version` reads like a sentence in
// the default format while --output json stays machine-readable.
func (v versionInfo) Table() output.Table {
	return output.Table{
		Headers: []string{"version", "platform", "go"},
		Rows:    [][]string{{v.Version, v.Platform, v.GoVersion}},
	}
}

func newVersionCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the tt version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := g.printer(cmd)
			if err != nil {
				return err
			}
			return p.Print(versionInfo{
				Version:   g.version,
				GoVersion: runtime.Version(),
				Platform:  runtime.GOOS + "/" + runtime.GOARCH,
			})
		},
	}
}
