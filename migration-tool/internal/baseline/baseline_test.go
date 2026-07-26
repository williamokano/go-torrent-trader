package baseline

import (
	"slices"
	"strings"
	"testing"
)

// The baseline is transcribed by hand from docs/FULL_FEATURE_DOCUMENTATION.md,
// so the tests below guard the transcription rather than the logic: a duplicate
// entry or an empty type would quietly distort every comparison.

func TestNoDuplicateTables(t *testing.T) {
	seen := map[string]bool{}
	for _, tb := range TorrentTrader30().Tables {
		lower := strings.ToLower(tb.Name)
		if seen[lower] {
			t.Errorf("table %q appears twice", tb.Name)
		}
		seen[lower] = true
	}
}

func TestNoDuplicateColumns(t *testing.T) {
	for _, tb := range TorrentTrader30().Tables {
		seen := map[string]bool{}
		for _, c := range tb.Columns {
			lower := strings.ToLower(c.Name)
			if seen[lower] {
				t.Errorf("%s.%s appears twice", tb.Name, c.Name)
			}
			seen[lower] = true
		}
	}
}

func TestEveryColumnHasNameAndType(t *testing.T) {
	for _, tb := range TorrentTrader30().Tables {
		for i, c := range tb.Columns {
			if c.Name == "" {
				t.Errorf("%s column %d has no name", tb.Name, i)
			}
			if c.Type == "" {
				t.Errorf("%s.%s has no type", tb.Name, c.Name)
			}
		}
	}
}

// ColumnsDocumented is what stops the tool reporting every column of a table it
// cannot describe as mod-added, so the flag and the data must agree.
func TestColumnsDocumentedMatchesTheData(t *testing.T) {
	for _, tb := range TorrentTrader30().Tables {
		switch {
		case tb.ColumnsDocumented && len(tb.Columns) == 0:
			t.Errorf("%s claims documented columns but has none", tb.Name)
		case !tb.ColumnsDocumented && len(tb.Columns) > 0:
			t.Errorf("%s has columns but does not claim to be documented", tb.Name)
		}
	}
}

func TestEveryTableHasAPurpose(t *testing.T) {
	for _, tb := range TorrentTrader30().Tables {
		if tb.Purpose == "" {
			t.Errorf("table %s has no purpose recorded", tb.Name)
		}
	}
}

// The nine tables section 1.2 of the reference document breaks down. Losing one
// would silently stop the tool checking that table's columns at all.
func TestDocumentedTablesAreTheOnesTheReferenceBreaksDown(t *testing.T) {
	want := []string{
		"users", "groups", "torrents", "peers", "completed",
		"messages", "forum_forums", "forum_topics", "forum_posts",
	}

	var got []string
	for _, tb := range TorrentTrader30().Tables {
		if tb.ColumnsDocumented {
			got = append(got, tb.Name)
		}
	}

	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("documented tables = %v, want %v", got, want)
	}
}

// A required table is one whose absence stops the migration. Marking a table
// required that is merely nice to have would fail runs that should succeed.
func TestRequiredTablesAreTheOnesNothingWorksWithout(t *testing.T) {
	want := []string{"users", "groups", "torrents", "peers", "completed", "categories"}

	var got []string
	for _, tb := range TorrentTrader30().Tables {
		if tb.Required {
			got = append(got, tb.Name)
		}
	}

	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("required tables = %v, want %v", got, want)
	}
}

func TestTableCountMatchesTheReference(t *testing.T) {
	// Section 1.1 lists 37 tables.
	const want = 37
	if got := len(TorrentTrader30().Tables); got != want {
		t.Errorf("baseline has %d tables, the reference document lists %d", got, want)
	}
}

func TestLookupIsCaseInsensitive(t *testing.T) {
	s := TorrentTrader30()

	tb, ok := s.Table("USERS")
	if !ok {
		t.Fatal("Table(\"USERS\") not found")
	}
	if _, ok := tb.Column("PASSKEY"); !ok {
		t.Error("Column(\"PASSKEY\") not found")
	}
	if _, ok := s.Table("no_such_table"); ok {
		t.Error("Table(\"no_such_table\") reported found")
	}
}

// Callers get a fresh value, so one caller mutating what it was handed cannot
// change what the next caller sees.
func TestCallersCannotMutateTheBaseline(t *testing.T) {
	first := TorrentTrader30()
	first.Tables[0].Name = "clobbered"
	first.Tables[0].Columns[0].Name = "clobbered"

	second := TorrentTrader30()
	if second.Tables[0].Name == "clobbered" {
		t.Error("mutating a returned table changed the baseline")
	}
	if second.Tables[0].Columns[0].Name == "clobbered" {
		t.Error("mutating a returned column changed the baseline")
	}
}

func TestTableNamesAreSorted(t *testing.T) {
	if names := TorrentTrader30().TableNames(); !slices.IsSorted(names) {
		t.Errorf("TableNames() not sorted: %v", names)
	}
}
