// Package schema models a relational schema as far as the migration tool needs
// to see it: table names, column names, and column types. It is deliberately
// smaller than MySQL's real metadata — the tool compares shapes and generates a
// mapping, it does not reproduce DDL.
package schema

import (
	"regexp"
	"sort"
	"strings"
)

// Column is one column of a source table.
type Column struct {
	Name string
	// Type is the full MySQL type as written in DDL, lower-cased by the
	// server: "varchar(40)", "enum('yes','no')", "bigint unsigned".
	Type     string
	Nullable bool
	// Charset and Collation are empty for a non-text column. They matter
	// because a latin1 database loaded into a UTF-8 target is the classic
	// way to turn every accented username into mojibake.
	Charset   string
	Collation string
	// Default is the column's DEFAULT expression, empty when there is none.
	Default string
	// Key is the information_schema COLUMN_KEY value: "PRI", "UNI", "MUL" or "".
	Key string
	// Extra is the information_schema EXTRA value, e.g. "auto_increment".
	Extra string
}

// Table is one table and its columns, in ordinal order.
type Table struct {
	Name string
	// Engine is the storage engine: TorrentTrader uses MyISAM throughout.
	Engine string
	// Collation is the table's default collation, which its columns inherit
	// unless they override it.
	Collation string
	Columns   []Column
	// Rows is the row count. Where it came from — an estimate from
	// information_schema or an exact COUNT(*) — is the caller's business.
	Rows int64
}

// Column returns the named column, matched case-insensitively because MySQL
// column names are not case-sensitive.
func (t Table) Column(name string) (Column, bool) {
	for _, c := range t.Columns {
		if strings.EqualFold(c.Name, name) {
			return c, true
		}
	}
	return Column{}, false
}

// ColumnNames returns the column names in ordinal order.
func (t Table) ColumnNames() []string {
	names := make([]string, 0, len(t.Columns))
	for _, c := range t.Columns {
		names = append(names, c.Name)
	}
	return names
}

// Schema is a set of tables, held sorted by name.
type Schema struct {
	// Database is the schema name the tables were read from.
	Database string
	Tables   []Table
}

// Table returns the named table, matched case-insensitively. MySQL table-name
// case sensitivity depends on the server's filesystem, so the tool never relies
// on it.
func (s Schema) Table(name string) (Table, bool) {
	for _, t := range s.Tables {
		if strings.EqualFold(t.Name, name) {
			return t, true
		}
	}
	return Table{}, false
}

// TableNames returns the table names in sorted order.
func (s Schema) TableNames() []string {
	names := make([]string, 0, len(s.Tables))
	for _, t := range s.Tables {
		names = append(names, t.Name)
	}
	sort.Strings(names)
	return names
}

// intDisplayWidth matches the deprecated display width on an integer type:
// MySQL 5.x reports "int(10) unsigned" where MySQL 8 reports "int unsigned".
// The width carries no meaning, so it must not read as a type mismatch.
var intDisplayWidth = regexp.MustCompile(`^(tinyint|smallint|mediumint|int|integer|bigint)\(\d+\)`)

// typeNoise matches attributes that describe storage or collation rather than
// the type itself, and so must not read as a type mismatch either.
var typeNoise = regexp.MustCompile(`\s+(binary|zerofill|character set \S+|collate \S+)`)

// NormalizeType reduces a MySQL type to the form used for comparison, so that
// declarations which differ only in how the server chose to report them compare
// equal. It lower-cases, collapses whitespace, drops integer display widths and
// storage attributes, and spells "integer" as "int".
//
// Genuine differences survive: varchar(40) and varchar(20) do not compare equal,
// and neither do enum sets with different members — including members that
// happen to contain a word like "binary" or "collate", which is why the
// attribute stripping is confined to the part of the declaration outside the
// parentheses.
func NormalizeType(t string) string {
	n := strings.ToLower(strings.TrimSpace(t))
	n = strings.Join(strings.Fields(n), " ")

	body, attrs := splitTypeAttributes(n)
	attrs = typeNoise.ReplaceAllString(" "+attrs, "")

	n = strings.TrimSpace(body + " " + attrs)
	n = strings.Join(strings.Fields(n), " ")
	n = intDisplayWidth.ReplaceAllString(n, "$1")
	if n == "integer" || strings.HasPrefix(n, "integer ") {
		n = "int" + strings.TrimPrefix(n, "integer")
	}
	// enum('yes', 'no') and enum('yes','no') are the same set.
	n = strings.ReplaceAll(n, ", '", ",'")
	return strings.TrimSpace(n)
}

// splitTypeAttributes divides a declaration into the type itself and the
// attributes trailing it. For "enum('a','b') character set utf8" that is
// "enum('a','b')" and "character set utf8"; for "bigint unsigned" it is
// "bigint" and "unsigned".
//
// The parenthesised part is found by scanning rather than by regexp because it
// can contain anything — quotes, commas, and words that look like attributes.
func splitTypeAttributes(t string) (body, attrs string) {
	open := strings.IndexByte(t, '(')
	if open < 0 {
		if space := strings.IndexByte(t, ' '); space >= 0 {
			return t[:space], t[space+1:]
		}
		return t, ""
	}

	inQuote := false
	for i := open; i < len(t); i++ {
		switch t[i] {
		case '\'':
			// '' inside a quoted literal is an escaped quote, and flipping
			// twice leaves the state correct either way.
			inQuote = !inQuote
		case ')':
			if !inQuote {
				return t[:i+1], strings.TrimSpace(t[i+1:])
			}
		}
	}
	// Unbalanced — no server produces this, so treat it all as the type.
	return t, ""
}

// TypesEqual reports whether two MySQL type declarations describe the same type.
func TypesEqual(a, b string) bool {
	return NormalizeType(a) == NormalizeType(b)
}

// TextEncodings returns the distinct character sets used by text columns in the
// schema, sorted. An operator needs this before a migration: everything the
// target stores is UTF-8, so anything else here has to be converted on the way
// in rather than copied byte for byte.
func (s Schema) TextEncodings() []string {
	seen := map[string]bool{}
	for _, t := range s.Tables {
		for _, c := range t.Columns {
			if c.Charset != "" {
				seen[strings.ToLower(c.Charset)] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// IsUTF8 reports whether a MySQL character set name stores Unicode. utf8mb3 —
// MySQL's old three-byte "utf8" — counts: it is Unicode, just unable to hold
// astral characters, which is a narrower problem than a latin1 column.
func IsUTF8(charset string) bool {
	switch strings.ToLower(charset) {
	case "utf8", "utf8mb3", "utf8mb4", "ucs2", "utf16", "utf16le", "utf32":
		return true
	default:
		return false
	}
}
