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
	// Default is the column's DEFAULT expression, empty when there is none.
	Default string
	// Key is the information_schema COLUMN_KEY value: "PRI", "UNI", "MUL" or "".
	Key string
	// Extra is the information_schema EXTRA value, e.g. "auto_increment".
	Extra string
}

// Table is one table and its columns, in ordinal order.
type Table struct {
	Name    string
	Engine  string
	Columns []Column
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
// and neither do enum sets with different members.
func NormalizeType(t string) string {
	n := strings.ToLower(strings.TrimSpace(t))
	n = strings.Join(strings.Fields(n), " ")
	n = typeNoise.ReplaceAllString(n, "")
	n = intDisplayWidth.ReplaceAllString(n, "$1")
	if n == "integer" || strings.HasPrefix(n, "integer ") {
		n = "int" + strings.TrimPrefix(n, "integer")
	}
	// enum('yes', 'no') and enum('yes','no') are the same set.
	n = strings.ReplaceAll(n, ", '", ",'")
	return strings.TrimSpace(n)
}

// TypesEqual reports whether two MySQL type declarations describe the same type.
func TypesEqual(a, b string) bool {
	return NormalizeType(a) == NormalizeType(b)
}
