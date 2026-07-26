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

// The attribute keywords also occur inside enum literals, where they are data.
// An earlier version stripped them everywhere and silently truncated the type.
func TestNormalizeTypeDoesNotEditInsideLiterals(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"collate in a member", "enum('yes collate x','no')", "enum('yes collate x','no')"},
		{"binary in a member", "enum('a binary','b')", "enum('a binary','b')"},
		{"charset in a member", "set('character set utf8','b')", "set('character set utf8','b')"},
		{"paren in a member", "enum('a)b','c')", "enum('a)b','c')"},
		{"attribute still stripped outside", "enum('a','b') character set utf8", "enum('a','b')"},
		{"collation still stripped outside", "varchar(20) collate utf8_bin", "varchar(20)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeType(tc.in); got != tc.want {
				t.Errorf("NormalizeType(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	// The consequence that matters: two different enum sets must not collapse
	// into the same normalized form.
	if TypesEqual("enum('a binary','b')", "enum('a','b')") {
		t.Error("enum sets differing by a member containing \"binary\" compared equal")
	}
}

func TestIsUTF8(t *testing.T) {
	unicode := []string{"utf8", "utf8mb3", "utf8mb4", "UTF8MB4", "ucs2", "utf16", "utf16le", "utf32"}
	for _, cs := range unicode {
		if !IsUTF8(cs) {
			t.Errorf("IsUTF8(%q) = false, want true", cs)
		}
	}
	// latin1 is what a 2008 TorrentTrader actually is, and the one that has to
	// be caught.
	notUnicode := []string{"latin1", "latin2", "cp1251", "koi8r", "big5", "sjis", "binary", ""}
	for _, cs := range notUnicode {
		if IsUTF8(cs) {
			t.Errorf("IsUTF8(%q) = true, want false", cs)
		}
	}
}

func TestTextEncodings(t *testing.T) {
	s := Schema{Tables: []Table{
		{Name: "users", Columns: []Column{
			{Name: "id", Type: "int"}, // no charset
			{Name: "username", Type: "varchar(40)", Charset: "latin1"},
			{Name: "info", Type: "text", Charset: "LATIN1"}, // same set, other case
		}},
		{Name: "posts", Columns: []Column{
			{Name: "body", Type: "text", Charset: "utf8mb4"},
		}},
	}}

	got := s.TextEncodings()
	want := []string{"latin1", "utf8mb4"}
	if len(got) != len(want) {
		t.Fatalf("TextEncodings() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("TextEncodings()[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	if enc := (Schema{}).TextEncodings(); len(enc) != 0 {
		t.Errorf("TextEncodings() on an empty schema = %v, want none", enc)
	}
}
