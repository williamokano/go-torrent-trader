package schema

import "testing"

func TestNormalizeTypeIgnoresReportingDifferences(t *testing.T) {
	// Pairs a MySQL server may report differently for the same declaration.
	same := []struct {
		name string
		a, b string
	}{
		{"integer display width", "int(10) unsigned", "int unsigned"},
		{"bigint display width", "BIGINT(20) UNSIGNED", "bigint unsigned"},
		{"tinyint display width", "tinyint(3) unsigned", "TINYINT UNSIGNED"},
		{"case", "VARCHAR(40)", "varchar(40)"},
		{"binary attribute", "varchar(20) binary", "varchar(20)"},
		{"collation", "varchar(20) COLLATE utf8_bin", "varchar(20)"},
		{"charset", "text CHARACTER SET utf8", "text"},
		{"enum spacing", "enum('yes', 'no')", "enum('yes','no')"},
		{"integer alias", "integer unsigned", "int unsigned"},
		{"surrounding space", "  datetime  ", "datetime"},
	}
	for _, tc := range same {
		t.Run(tc.name, func(t *testing.T) {
			if !TypesEqual(tc.a, tc.b) {
				t.Errorf("TypesEqual(%q, %q) = false, want true (normalized to %q and %q)",
					tc.a, tc.b, NormalizeType(tc.a), NormalizeType(tc.b))
			}
		})
	}
}

func TestNormalizeTypeKeepsRealDifferences(t *testing.T) {
	// Differences that change what the column can hold must survive
	// normalization, or the tool would report a modded schema as stock.
	different := []struct {
		name string
		a, b string
	}{
		{"varchar length", "varchar(40)", "varchar(20)"},
		{"signedness", "int unsigned", "int"},
		{"width class", "int", "bigint"},
		{"enum members", "enum('yes','no')", "enum('yes','no','maybe')"},
		{"text size", "text", "longtext"},
		{"char vs varchar", "char(3)", "varchar(3)"},
		{"decimal precision", "decimal(10,2)", "decimal(10,4)"},
	}
	for _, tc := range different {
		t.Run(tc.name, func(t *testing.T) {
			if TypesEqual(tc.a, tc.b) {
				t.Errorf("TypesEqual(%q, %q) = true, want false: both normalized to %q",
					tc.a, tc.b, NormalizeType(tc.a))
			}
		})
	}
}

func TestTableColumnLookupIsCaseInsensitive(t *testing.T) {
	table := Table{Name: "users", Columns: []Column{{Name: "PassKey", Type: "varchar(32)"}}}

	got, ok := table.Column("passkey")
	if !ok {
		t.Fatal("Column(\"passkey\") not found; MySQL column names are not case-sensitive")
	}
	if got.Name != "PassKey" {
		t.Errorf("got column %q, want the stored spelling PassKey", got.Name)
	}

	if _, ok := table.Column("nope"); ok {
		t.Error("Column(\"nope\") reported found")
	}
}

func TestSchemaTableLookupIsCaseInsensitive(t *testing.T) {
	s := Schema{Tables: []Table{{Name: "Torrents"}, {Name: "users"}}}

	if _, ok := s.Table("torrents"); !ok {
		t.Error("Table(\"torrents\") not found")
	}
	if _, ok := s.Table("missing"); ok {
		t.Error("Table(\"missing\") reported found")
	}

	names := s.TableNames()
	want := []string{"Torrents", "users"}
	if len(names) != len(want) {
		t.Fatalf("TableNames() = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("TableNames()[%d] = %q, want %q", i, names[i], want[i])
		}
	}
}

func TestColumnNames(t *testing.T) {
	table := Table{Columns: []Column{{Name: "id"}, {Name: "username"}}}
	got := table.ColumnNames()
	if len(got) != 2 || got[0] != "id" || got[1] != "username" {
		t.Errorf("ColumnNames() = %v, want [id username] in ordinal order", got)
	}
}
