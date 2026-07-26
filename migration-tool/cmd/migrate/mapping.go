package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/williamokano/go-torrent-trader/migration-tool/internal/config"
	"github.com/williamokano/go-torrent-trader/migration-tool/internal/mapping"
	"github.com/williamokano/go-torrent-trader/migration-tool/internal/source"
)

var (
	mappingOut   string
	mappingForce bool
)

// mappingCmd writes the column mapping an operator reviews before a migration.
var mappingCmd = &cobra.Command{
	Use:   "mapping",
	Short: "Generate a column mapping file from the source database",
	Long: "Reads the legacy database and writes a YAML file proposing where every column\n" +
		"should land in the new schema, with the reasoning kept as comments.\n\n" +
		"The file is generated from your database rather than from a reference schema, so\n" +
		"columns a mod added appear as entries marked `custom` rather than going missing.\n" +
		"Edit it, keep it in version control, and pass it back when you run the migration.\n\n" +
		"Writing to - sends the file to stdout.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.LoadFromFlags(cmd)
		if err != nil {
			return err
		}
		return withSource(cmd, func(ctx context.Context, db *source.DB) error {
			s, err := db.Schema(ctx)
			if err != nil {
				return err
			}
			// The file is meant to be committed, so it records the server
			// without any credentials — not even the username.
			doc := mapping.Build(s, db.Server())
			out, err := mapping.Render(doc)
			if err != nil {
				return err
			}
			if err := writeMapping(cmd, out, cfg.DryRun); err != nil {
				return err
			}
			return printMappingSummary(cmd.ErrOrStderr(), doc)
		})
	},
}

func init() {
	mappingCmd.Flags().StringVar(&mappingOut, "out", "mapping.yaml",
		"File to write the mapping to, or - for stdout")
	mappingCmd.Flags().BoolVar(&mappingForce, "force", false,
		"Overwrite the output file if it already exists")
}

// writeMapping refuses to overwrite an existing file unless asked. A mapping is
// hand-edited, so silently replacing one would throw away an operator's review.
func writeMapping(cmd *cobra.Command, content []byte, dryRun bool) error {
	if dryRun {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(),
			"--dry-run: %s not written (%d bytes).\n", mappingOut, len(content)); err != nil {
			return fmt.Errorf("writing to stdout: %w", err)
		}
		return nil
	}
	if mappingOut == "-" {
		_, err := cmd.OutOrStdout().Write(content)
		if err != nil {
			return fmt.Errorf("writing mapping: %w", err)
		}
		return nil
	}

	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if mappingForce {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	f, err := os.OpenFile(mappingOut, flags, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%s already exists; pass --force to replace it, or --out to write elsewhere", mappingOut)
		}
		return fmt.Errorf("creating %s: %w", mappingOut, err)
	}
	if _, err := f.Write(content); err != nil {
		return errors.Join(fmt.Errorf("writing %s: %w", mappingOut, err), f.Close())
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", mappingOut, err)
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s\n", mappingOut); err != nil {
		return fmt.Errorf("writing to stdout: %w", err)
	}
	return nil
}

// printMappingSummary goes to stderr so that `--out -` stays a clean YAML
// stream that can be piped.
func printMappingSummary(out io.Writer, doc mapping.Document) error {
	p := newPrinter(out)
	p.printf("\n%d tables mapped from %s.\n", len(doc.Tables), doc.Server)

	decisions := doc.Decisions()
	if len(decisions) == 0 {
		p.println("Every column has a rule. Review the file before running the migration.")
	} else {
		p.printf("\n%d columns need a decision before the migration can run:\n", len(decisions))
		for _, d := range summarize(decisions, 20) {
			p.printf("  %s\n", d)
		}
	}

	if absent := doc.AbsentColumns(); len(absent) > 0 {
		p.printf("\n%d columns this mapping expects are not in your database:\n", len(absent))
		for _, a := range summarize(absent, 20) {
			p.printf("  %s\n", a)
		}
		p.println("  Each one is a rule with nothing to read. Check them against `migrate validate`.")
	}
	return p.errf("the mapping summary")
}

// summarize caps a list for display and says how much it left out, so a long
// list never reads as a complete one.
func summarize(items []string, limit int) []string {
	if len(items) <= limit {
		return items
	}
	out := make([]string, 0, limit+1)
	out = append(out, items[:limit]...)
	return append(out, fmt.Sprintf("... and %d more (all of them are in the file)", len(items)-limit))
}
