package compare

import (
	"slices"
	"testing"

	"github.com/williamokano/go-torrent-trader/migration-tool/internal/baseline"
	"github.com/williamokano/go-torrent-trader/migration-tool/internal/mapping"
	"github.com/williamokano/go-torrent-trader/migration-tool/internal/schema"
)

// stockSchema builds a source schema that matches the baseline exactly, which
// is the starting point every test then perturbs in one way.
func stockSchema(t *testing.T) schema.Schema {
	t.Helper()
	base := baseline.TorrentTrader30()

	s := schema.Schema{Database: "torrenttrader"}
	for _, bt := range base.Tables {
		st := schema.Table{Name: bt.Name, Engine: "MyISAM"}
		for _, bc := range bt.Columns {
			st.Columns = append(st.Columns, schema.Column{Name: bc.Name, Type: bc.Type})
		}
		s.Tables = append(s.Tables, st)
	}
	return s
}

func TestCompareStockSchemaIsClean(t *testing.T) {
	r := Compare(baseline.TorrentTrader30(), stockSchema(t), mapping.ReadColumns())

	if !r.Clean() {
		t.Errorf("a schema built from the baseline reported differences: %+v", r)
	}
	if r.Blocking() {
		t.Error("a stock schema must not block the migration")
	}
}

// The MySQL version an install runs on changes how types are reported. A
// server-side difference must not read as a schema difference.
func TestCompareToleratesServerReportingDifferences(t *testing.T) {
	s := stockSchema(t)
	users, _ := s.Table("users")
	for i, c := range users.Columns {
		switch c.Name {
		case "id":
			users.Columns[i].Type = "int(10) unsigned"
		case "username":
			users.Columns[i].Type = "VARCHAR(40)"
		case "secret":
			users.Columns[i].Type = "varchar(20) binary"
		case "acceptpms":
			users.Columns[i].Type = "enum('yes', 'no')"
		}
	}

	r := Compare(baseline.TorrentTrader30(), s, mapping.ReadColumns())
	if !r.Clean() {
		t.Errorf("reporting differences were treated as schema differences: %+v", r)
	}
}

func TestCompareReportsMissingTable(t *testing.T) {
	s := stockSchema(t)
	s.Tables = slices.DeleteFunc(s.Tables, func(tb schema.Table) bool { return tb.Name == "polls" })

	r := Compare(baseline.TorrentTrader30(), s, mapping.ReadColumns())

	if !slices.Contains(r.MissingTables, "polls") {
		t.Errorf("MissingTables = %v, want it to contain polls", r.MissingTables)
	}
	// polls is not required: an install that dropped it can still migrate.
	if r.Blocking() {
		t.Error("a missing optional table must not block the migration")
	}
	if r.Clean() {
		t.Error("a missing table is a difference")
	}
}

func TestCompareBlocksOnMissingRequiredTable(t *testing.T) {
	s := stockSchema(t)
	s.Tables = slices.DeleteFunc(s.Tables, func(tb schema.Table) bool { return tb.Name == "torrents" })

	r := Compare(baseline.TorrentTrader30(), s, mapping.ReadColumns())

	if !slices.Contains(r.MissingRequiredTables, "torrents") {
		t.Errorf("MissingRequiredTables = %v, want it to contain torrents", r.MissingRequiredTables)
	}
	if !r.Blocking() {
		t.Error("a missing required table must block the migration")
	}
}

func TestCompareBlocksOnMissingColumn(t *testing.T) {
	s := stockSchema(t)
	for i, tb := range s.Tables {
		if tb.Name != "users" {
			continue
		}
		s.Tables[i].Columns = slices.DeleteFunc(tb.Columns, func(c schema.Column) bool {
			return c.Name == "passkey"
		})
	}

	r := Compare(baseline.TorrentTrader30(), s, mapping.ReadColumns())

	var users TableReport
	for _, tr := range r.Tables {
		if tr.Name == "users" {
			users = tr
		}
	}
	if !slices.Contains(users.MissingColumns, "passkey") {
		t.Errorf("MissingColumns = %v, want it to contain passkey", users.MissingColumns)
	}
	if !r.Blocking() {
		t.Error("a missing column must block: the migration has nothing to read")
	}
}

// Mods are the normal case, not an error case.
func TestCompareReportsModsWithoutBlocking(t *testing.T) {
	s := stockSchema(t)
	for i, tb := range s.Tables {
		if tb.Name == "users" {
			s.Tables[i].Columns = append(tb.Columns, schema.Column{Name: "seedbonus", Type: "float"})
		}
	}
	s.Tables = append(s.Tables, schema.Table{Name: "bonus_log", Columns: []schema.Column{{Name: "id", Type: "int"}}})

	r := Compare(baseline.TorrentTrader30(), s, mapping.ReadColumns())

	if !slices.Contains(r.AddedTables, "bonus_log") {
		t.Errorf("AddedTables = %v, want it to contain bonus_log", r.AddedTables)
	}
	var users TableReport
	for _, tr := range r.Tables {
		if tr.Name == "users" {
			users = tr
		}
	}
	if !slices.Contains(users.AddedColumns, "seedbonus") {
		t.Errorf("AddedColumns = %v, want it to contain seedbonus", users.AddedColumns)
	}
	if r.Blocking() {
		t.Error("mods must not block the migration; that is what the mapping file is for")
	}
	if r.Clean() {
		t.Error("mods are differences and must be reported")
	}
}

func TestCompareReportsTypeMismatchWithoutBlocking(t *testing.T) {
	s := stockSchema(t)
	for i, tb := range s.Tables {
		if tb.Name != "users" {
			continue
		}
		for j, c := range tb.Columns {
			if c.Name == "username" {
				s.Tables[i].Columns[j].Type = "varchar(64)"
			}
		}
	}

	r := Compare(baseline.TorrentTrader30(), s, mapping.ReadColumns())

	var found bool
	for _, tr := range r.Tables {
		for _, m := range tr.TypeMismatches {
			if tr.Name == "users" && m.Column == "username" {
				found = true
				if m.Baseline != "varchar(40)" || m.Found != "varchar(64)" {
					t.Errorf("mismatch = %+v, want baseline varchar(40) and found varchar(64)", m)
				}
			}
		}
	}
	if !found {
		t.Error("a widened column was not reported as a type mismatch")
	}
	if r.Blocking() {
		t.Error("a type mismatch must not block; the transform decides whether it matters")
	}
}

// The baseline documents some tables by name only. Reporting every column of
// those as mod-added would bury the real findings.
func TestCompareSkipsColumnsOfUndocumentedTables(t *testing.T) {
	s := stockSchema(t)
	for i, tb := range s.Tables {
		if tb.Name == "shoutbox" {
			s.Tables[i].Columns = []schema.Column{
				{Name: "id", Type: "int unsigned"},
				{Name: "text", Type: "text"},
			}
		}
	}

	r := Compare(baseline.TorrentTrader30(), s, mapping.ReadColumns())

	for _, tr := range r.Tables {
		if tr.Name != "shoutbox" {
			continue
		}
		if tr.ColumnsChecked {
			t.Error("shoutbox columns are not documented, so they must not be checked")
		}
		if tr.Differs() {
			t.Errorf("undocumented table reported column differences: %+v", tr)
		}
		return
	}
	t.Fatal("shoutbox missing from the report")
}

func TestCompareSortsOutput(t *testing.T) {
	s := schema.Schema{Tables: []schema.Table{
		{Name: "zeta"},
		{Name: "alpha"},
		{Name: "users"},
	}}

	r := Compare(baseline.TorrentTrader30(), s, mapping.ReadColumns())

	if !slices.IsSorted(r.AddedTables) {
		t.Errorf("AddedTables not sorted: %v", r.AddedTables)
	}
	if !slices.IsSorted(r.MissingTables) {
		t.Errorf("MissingTables not sorted: %v", r.MissingTables)
	}
	names := make([]string, 0, len(r.Tables))
	for _, tr := range r.Tables {
		names = append(names, tr.Name)
	}
	if !slices.IsSorted(names) {
		t.Errorf("Tables not sorted: %v", names)
	}
}

// dropColumn removes a column from a table in a discovered schema.
func dropColumn(s schema.Schema, table, column string) schema.Schema {
	for i, tb := range s.Tables {
		if tb.Name != table {
			continue
		}
		s.Tables[i].Columns = slices.DeleteFunc(slices.Clone(tb.Columns), func(c schema.Column) bool {
			return c.Name == column
		})
	}
	return s
}

// The regression this package was written wrong for once: 35 baseline columns
// are skipped by the plan, and an install that dropped one used to be told its
// database could not be migrated.
func TestDroppingASkippedColumnDoesNotBlock(t *testing.T) {
	// Every one of these is an ActionSkip rule in the mapping plan.
	for _, column := range []string{"age", "gender", "signature", "tzoffset", "privacy", "stylesheet", "page"} {
		t.Run(column, func(t *testing.T) {
			s := dropColumn(stockSchema(t), "users", column)
			r := Compare(baseline.TorrentTrader30(), s, mapping.ReadColumns())

			if r.Blocking() {
				t.Errorf("dropping users.%s blocked the migration, but nothing reads it", column)
			}

			var users TableReport
			for _, tr := range r.Tables {
				if tr.Name == "users" {
					users = tr
				}
			}
			if !slices.Contains(users.DroppedColumns, column) {
				t.Errorf("DroppedColumns = %v, want it to contain %s", users.DroppedColumns, column)
			}
			if slices.Contains(users.MissingColumns, column) {
				t.Errorf("%s was reported as missing, which is the blocking list", column)
			}
			// It is still a difference — the operator should see it.
			if r.Clean() {
				t.Error("a dropped column was not reported at all")
			}
		})
	}
}

// The other half: a column the migration does read must still block.
func TestDroppingAReadColumnBlocks(t *testing.T) {
	for _, column := range []string{"passkey", "username", "uploaded", "class"} {
		t.Run(column, func(t *testing.T) {
			s := dropColumn(stockSchema(t), "users", column)
			r := Compare(baseline.TorrentTrader30(), s, mapping.ReadColumns())

			if !r.Blocking() {
				t.Errorf("dropping users.%s did not block, but the migration reads it", column)
			}

			var users TableReport
			for _, tr := range r.Tables {
				if tr.Name == "users" {
					users = tr
				}
			}
			if !slices.Contains(users.MissingColumns, column) {
				t.Errorf("MissingColumns = %v, want it to contain %s", users.MissingColumns, column)
			}
		})
	}
}

// A caller with no plan to hand gets the cautious reading, not a silent pass.
func TestNilReadsTreatsEveryColumnAsRead(t *testing.T) {
	s := dropColumn(stockSchema(t), "users", "age")
	r := Compare(baseline.TorrentTrader30(), s, nil)

	if !r.Blocking() {
		t.Error("with no plan supplied, any missing column must be treated as read")
	}
}
