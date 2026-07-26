package main

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/williamokano/go-torrent-trader/migration-tool/internal/baseline"
	"github.com/williamokano/go-torrent-trader/migration-tool/internal/compare"
	"github.com/williamokano/go-torrent-trader/migration-tool/internal/mapping"
	"github.com/williamokano/go-torrent-trader/migration-tool/internal/schema"
)

// errSchemaDiffers is returned when validation fails, so the process exits
// non-zero without cobra printing a second, redundant explanation.
var errSchemaDiffers = errors.New("source schema is not usable as it stands")

var validateStrict bool

// validateCmd compares the source schema against the TorrentTrader 3.0 baseline.
var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate source database schema against expected format",
	Long: "Compares the legacy database against the TorrentTrader 3.0 schema and reports what\n" +
		"differs: tables that are missing, tables and columns that were added, and columns\n" +
		"whose type has changed.\n\n" +
		"Differences are expected — installs collect mods. The command fails only when a\n" +
		"table the migration cannot run without is missing, or a column a transformer reads\n" +
		"is missing. A column the migration skips anyway is reported and not held against\n" +
		"you. Use --strict to fail on any difference at all.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return withSchema(cmd, func(s schema.Schema, server string) error {
			report := compare.Compare(baseline.TorrentTrader30(), s, mapping.ReadColumns())
			if err := printReport(cmd.OutOrStdout(), report, server, s); err != nil {
				return err
			}

			switch {
			case report.Blocking():
				return errSchemaDiffers
			case validateStrict && !report.Clean():
				return fmt.Errorf("%w: --strict is set and the schema is not stock", errSchemaDiffers)
			default:
				return nil
			}
		})
	},
}

func init() {
	validateCmd.Flags().BoolVar(&validateStrict, "strict", false,
		"Fail on any difference from the stock schema, not just the blocking ones")
}

func printReport(out io.Writer, r compare.Report, server string, s schema.Schema) error {
	p := newPrinter(out)
	p.printf("Source: %s\n\n", server)
	printEncodings(p, s)

	if r.Clean() {
		p.println("This is a stock TorrentTrader 3.0 schema — no differences found.")
		return p.errf("the validation report")
	}

	if len(r.MissingTables) > 0 {
		p.println("Missing tables")
		for _, name := range r.MissingTables {
			note := ""
			if slices.Contains(r.MissingRequiredTables, name) {
				note = "  (required — the migration cannot run without it)"
			}
			p.printf("  - %s%s\n", name, note)
		}
		p.println()
	}

	if len(r.AddedTables) > 0 {
		p.println("Tables not in TorrentTrader 3.0")
		p.printf("  %s\n", strings.Join(r.AddedTables, ", "))
		p.println("  These are mods, or another application sharing the database.")
		p.println()
	}

	var changed, unchecked []compare.TableReport
	for _, t := range r.Tables {
		switch {
		case !t.ColumnsChecked:
			unchecked = append(unchecked, t)
		case t.Differs():
			changed = append(changed, t)
		}
	}

	for _, t := range changed {
		p.printf("Table %s\n", t.Name)
		for _, c := range t.MissingColumns {
			p.printf("  ! missing column %s — the migration reads this one\n", c)
		}
		for _, c := range t.DroppedColumns {
			p.printf("  - column %s is gone (not migrated anyway)\n", c)
		}
		for _, c := range t.AddedColumns {
			p.printf("  + added column %s\n", c)
		}
		for _, m := range t.TypeMismatches {
			p.printf("  ~ column %s is %s, expected %s\n", m.Column, m.Found, m.Baseline)
		}
		p.println()
	}

	if len(unchecked) > 0 {
		names := make([]string, 0, len(unchecked))
		for _, t := range unchecked {
			names = append(names, t.Name)
		}
		p.println("Columns not checked")
		p.printf("  %s\n", strings.Join(names, ", "))
		p.println("  The reference schema names these tables but does not document their")
		p.println("  columns, so only their presence was verified. `migrate mapping` lists")
		p.println("  their columns for review.")
		p.println()
	}

	if r.Blocking() {
		p.println("Result: the migration cannot run against this schema as it stands.")
		p.println()
		p.println("Every line marked ! is a column or table a transformer reads and your")
		p.println("database does not have. Restore it from a backup, add it back empty, or")
		p.println("edit the mapping so nothing reads it. Lines marked - are already handled.")
	} else {
		p.println("Result: usable. Generate a mapping with `migrate mapping` and review it")
		p.println("before running the migration.")
	}
	return p.errf("the validation report")
}

// printEncodings reports the character sets the source stores text in.
//
// This is separate from the baseline diff because it is not a schema
// difference — a stock TorrentTrader from 2008 is latin1 throughout and matches
// the baseline perfectly. It is reported anyway because the target stores UTF-8,
// and copying latin1 bytes into it unconverted is what turns every accented
// username and every non-English forum post into mojibake. That damage is
// silent, and it is discovered weeks later.
func printEncodings(p *printer, s schema.Schema) {
	encodings := s.TextEncodings()
	if len(encodings) == 0 {
		return
	}

	var legacy []string
	for _, e := range encodings {
		if !schema.IsUTF8(e) {
			legacy = append(legacy, e)
		}
	}
	if len(legacy) == 0 {
		p.printf("Text encoding: %s. Nothing to convert.\n\n", strings.Join(encodings, ", "))
		return
	}

	p.printf("Text encoding: %s\n", strings.Join(encodings, ", "))
	p.printf("  %s is not Unicode, and the target stores UTF-8.\n", strings.Join(legacy, " and "))
	p.println("  The migration must convert this text rather than copy its bytes. Reading")
	p.println("  the source with the wrong charset is what turns accented usernames and")
	p.println("  non-English forum posts into mojibake, and it is not noticed for weeks.")
	p.println()
}
