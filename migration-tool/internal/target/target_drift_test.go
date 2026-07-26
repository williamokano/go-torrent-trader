package target

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

// The declaration in target.go is hand-written because this module may not
// import the backend's. This test reads the backend's migrations and fails if
// the two disagree.
//
// It skips when the migrations are not there, so the module still builds and
// tests standalone — in the Docker image, for instance, whose build context is
// migration-tool/ alone.

const migrationsDir = "../../../backend/migrations"

// statement matches the four kinds of DDL that change the shape of a table.
// They are scanned in source order, because a migration that drops a table and
// recreates it — 030 does exactly this to news — means the last word wins.
var statement = regexp.MustCompile(
	`(?is)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-z_]+)\s*\((.*?)\n\)\s*;` +
		`|ALTER\s+TABLE\s+(?:ONLY\s+)?([a-z_]+)\s+ADD\s+COLUMN\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-z_][a-z0-9_]*)` +
		`|ALTER\s+TABLE\s+(?:ONLY\s+)?([a-z_]+)\s+DROP\s+COLUMN\s+(?:IF\s+EXISTS\s+)?([a-z_][a-z0-9_]*)` +
		`|DROP\s+TABLE\s+(?:IF\s+EXISTS\s+)?([a-z_]+)`)

var (
	gooseDown  = regexp.MustCompile(`(?im)^--\s*\+goose\s+Down\s*$`)
	constraint = regexp.MustCompile(`(?i)^(PRIMARY|UNIQUE|FOREIGN|CHECK|CONSTRAINT|EXCLUDE)\b`)
	columnName = regexp.MustCompile(`(?i)^([a-z_][a-z0-9_]*)\s+`)
)

// readBackendSchema replays every migration's Up section in order and returns
// the tables it leaves behind.
func readBackendSchema(t *testing.T) map[string][]string {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	if err != nil {
		t.Fatalf("globbing migrations: %v", err)
	}
	if len(paths) == 0 {
		t.Skipf("no migrations at %s — nothing to check this declaration against", migrationsDir)
	}
	sort.Strings(paths)

	tables := map[string]map[string]bool{}
	for _, path := range paths {
		content, err := os.ReadFile(path) //nolint:gosec // fixed path inside the repo
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		// Only the Up section: the Down section undoes it.
		up := string(content)
		if loc := gooseDown.FindStringIndex(up); loc != nil {
			up = up[:loc[0]]
		}

		for _, m := range statement.FindAllStringSubmatch(up, -1) {
			switch {
			case m[1] != "":
				applyCreate(tables, strings.ToLower(m[1]), m[2])
			case m[3] != "":
				ensure(tables, strings.ToLower(m[3]))[strings.ToLower(m[4])] = true
			case m[5] != "":
				delete(ensure(tables, strings.ToLower(m[5])), strings.ToLower(m[6]))
			case m[7] != "":
				delete(tables, strings.ToLower(m[7]))
			}
		}
	}

	out := make(map[string][]string, len(tables))
	for name, cols := range tables {
		list := make([]string, 0, len(cols))
		for c := range cols {
			list = append(list, c)
		}
		sort.Strings(list)
		out[name] = list
	}
	return out
}

func ensure(tables map[string]map[string]bool, name string) map[string]bool {
	if tables[name] == nil {
		tables[name] = map[string]bool{}
	}
	return tables[name]
}

func applyCreate(tables map[string]map[string]bool, name, body string) {
	cols := ensure(tables, name)
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSuffix(strings.TrimSpace(line), ",")
		if line == "" || strings.HasPrefix(line, "--") || constraint.MatchString(line) {
			continue
		}
		if m := columnName.FindStringSubmatch(line); m != nil {
			cols[strings.ToLower(m[1])] = true
		}
	}
}

// Every table this package declares must exist in the backend with exactly the
// columns claimed. A column that has gone is the dangerous direction — a rule
// pointing at it writes nowhere — but an extra one means the declaration is
// stale, which is how a rule comes to be missing for a column that exists.
func TestDeclarationMatchesBackendMigrations(t *testing.T) {
	backend := readBackendSchema(t)
	declared := PostgreSQL()

	for _, table := range declared.Tables() {
		actual, ok := backend[table]
		if !ok {
			t.Errorf("table %s is declared here but the backend does not create it", table)
			continue
		}

		for _, c := range declared.Columns(table) {
			if !slices.Contains(actual, c) {
				t.Errorf("%s.%s is declared here but not in backend/migrations", table, c)
			}
		}
		for _, c := range actual {
			if !declared.Has(table, c) {
				t.Errorf("%s.%s is in backend/migrations but missing from this declaration", table, c)
			}
		}
	}
}

// Seeded tables have to be real tables, or the warning it produces names
// something that does not exist.
func TestSeededTablesAreDeclared(t *testing.T) {
	declared := PostgreSQL()
	for table, note := range Seeded {
		if !declared.HasTable(table) {
			t.Errorf("Seeded names %s, which is not declared", table)
		}
		if strings.TrimSpace(note) == "" {
			t.Errorf("Seeded[%s] has no explanation", table)
		}
	}
}

func TestLookupIsCaseInsensitive(t *testing.T) {
	s := PostgreSQL()
	if !s.Has("users", "PASSKEY") {
		t.Error(`Has("users", "PASSKEY") = false`)
	}
	if s.Has("users", "no_such_column") {
		t.Error(`Has("users", "no_such_column") = true`)
	}
	if s.HasTable("no_such_table") {
		t.Error(`HasTable("no_such_table") = true`)
	}
}

// Columns hands out a copy, so a caller cannot reshape the declaration.
func TestColumnsCannotBeMutated(t *testing.T) {
	s := PostgreSQL()
	got := s.Columns("users")
	if len(got) == 0 {
		t.Fatal("users has no columns")
	}
	got[0] = "clobbered"

	if slices.Contains(PostgreSQL().Columns("users"), "clobbered") {
		t.Error("mutating the returned slice changed the declaration")
	}
}
