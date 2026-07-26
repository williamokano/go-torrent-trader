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
)

// discoverCmd reports what is in the source database.
var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Discover tables and data in the source database",
	Long: "Lists the tables in the legacy database with their storage engine, row count and\n" +
		"column count. With --table, prints that table's columns and the DDL the server\n" +
		"reports for it — which is what to attach to a bug report when a mapping looks wrong.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return withSource(cmd, func(ctx context.Context, db *source.DB) error {
			if discoverTable != "" {
				return describeTable(ctx, cmd.OutOrStdout(), db, discoverTable)
			}
			return listTables(ctx, cmd.OutOrStdout(), db)
		})
	},
}

func init() {
	discoverCmd.Flags().BoolVar(&discoverExact, "exact", false,
		"Count rows with COUNT(*) instead of trusting the engine's estimate")
	discoverCmd.Flags().StringVar(&discoverTable, "table", "",
		"Describe one table's columns and DDL instead of listing all tables")
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
	for _, t := range s.Tables {
		rows := t.Rows
		if discoverExact {
			if rows, err = db.CountRows(ctx, t.Name); err != nil {
				return err
			}
		}
		total += rows
		tp.printf("%s\t%s\t%d\t%d\n", t.Name, t.Engine, rows, len(t.Columns))
	}
	flush()

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
