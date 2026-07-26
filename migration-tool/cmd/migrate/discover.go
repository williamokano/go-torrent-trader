package main

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/williamokano/go-torrent-trader/migration-tool/internal/source"
)

var (
	discoverExact bool
	discoverTable string
	discoverDDL   bool
)

// discoverCmd reports what is in the source database.
var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Discover tables and data in the source database",
	Long: "Lists the tables in the legacy database with their storage engine, row count and\n" +
		"column count. With --table, prints that table's columns and the DDL the server\n" +
		"reports for it — which is what to attach to a bug report when a mapping looks\n" +
		"wrong. With --ddl, dumps SHOW CREATE TABLE for every table.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return withSource(cmd, func(ctx context.Context, db *source.DB) error {
			switch {
			case discoverTable != "":
				return describeTable(ctx, cmd.OutOrStdout(), db, discoverTable)
			case discoverDDL:
				return dumpDDL(ctx, cmd.OutOrStdout(), db)
			default:
				return listTables(ctx, cmd.OutOrStdout(), db)
			}
		})
	},
}

func init() {
	discoverCmd.Flags().BoolVar(&discoverExact, "exact", false,
		"Count rows with COUNT(*) instead of trusting the engine's estimate")
	discoverCmd.Flags().StringVar(&discoverTable, "table", "",
		"Describe one table's columns and DDL instead of listing all tables")
	discoverCmd.Flags().BoolVar(&discoverDDL, "ddl", false,
		"Dump SHOW CREATE TABLE for every table")
}

func listTables(ctx context.Context, out io.Writer, db *source.DB) error {
	s, err := db.Schema(ctx)
	if err != nil {
		return err
	}

	p := newPrinter(out)
	p.printf("Source: %s\n", db)
	counted := "estimated by the storage engine"
	if discoverExact {
		counted = "counted exactly"
	}
	p.printf("%d tables, rows %s.\n\n", len(s.Tables), counted)

	tp, flush := p.table()
	tp.println("TABLE\tENGINE\tROWS\tCOLUMNS")
	var total int64
	var countErr error
	for _, t := range s.Tables {
		rows := t.Rows
		if discoverExact {
			// A count can fail partway — a lock timeout on a big table is the
			// usual reason. Stop counting, but flush what was already gathered
			// rather than throwing the whole listing away.
			if rows, countErr = db.CountRows(ctx, t.Name); countErr != nil {
				break
			}
		}
		total += rows
		tp.printf("%s\t%s\t%d\t%d\n", t.Name, t.Engine, rows, len(t.Columns))
	}
	flush()

	if countErr != nil {
		return countErr
	}
	p.printf("\nTotal rows: %d\n", total)
	return p.errf("the table list")
}

func describeTable(ctx context.Context, out io.Writer, db *source.DB, name string) error {
	s, err := db.Schema(ctx)
	if err != nil {
		return err
	}
	t, ok := s.Table(name)
	if !ok {
		return fmt.Errorf("table %q is not in database %s", name, db.Database())
	}

	rows, err := db.CountRows(ctx, t.Name)
	if err != nil {
		return err
	}
	ddl, err := db.CreateTable(ctx, t.Name)
	if err != nil {
		return err
	}

	p := newPrinter(out)
	p.printf("Source: %s\n", db)
	p.printf("Table:  %s (%s, %d rows)\n\n", t.Name, t.Engine, rows)

	tp, flush := p.table()
	tp.println("COLUMN\tTYPE\tNULL\tKEY\tEXTRA")
	for _, c := range t.Columns {
		null := "NO"
		if c.Nullable {
			null = "YES"
		}
		tp.printf("%s\t%s\t%s\t%s\t%s\n", c.Name, c.Type, null, c.Key, c.Extra)
	}
	flush()

	p.printf("\n%s\n", ddl)
	return p.errf("the table description")
}

// dumpDDL prints the server's own CREATE TABLE for every table, which is the
// unambiguous record of what the source schema was on the night of the run.
func dumpDDL(ctx context.Context, out io.Writer, db *source.DB) error {
	s, err := db.Schema(ctx)
	if err != nil {
		return err
	}

	p := newPrinter(out)
	p.printf("-- Source: %s\n", db.Server())
	p.printf("-- %d tables\n", len(s.Tables))

	for _, t := range s.Tables {
		ddl, err := db.CreateTable(ctx, t.Name)
		if err != nil {
			return err
		}
		p.printf("\n%s;\n", ddl)
	}
	return p.errf("the DDL dump")
}
