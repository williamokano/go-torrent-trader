package target_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"slices"
	"sort"
	"testing"

	_ "github.com/lib/pq"

	"github.com/williamokano/go-torrent-trader/migration-tool/internal/target"
	"github.com/williamokano/go-torrent-trader/migration-tool/internal/testenv"
)

// The declaration in target.go is hand-written, because this module may not
// import the backend's. A declaration nothing checks drifts silently, and this
// one did: three rules in the first version named target columns that did not
// exist, and one skipped a column claiming the target had no such thing.
//
// So it is checked against the real schema — the backend's own goose migrations
// applied to a real PostgreSQL, read back through information_schema. Parsing
// the migration SQL would be cheaper and would answer a slightly different
// question: what the files appear to say, rather than what they produce.

func TestMain(m *testing.M) {
	code := m.Run()
	testenv.Cleanup()
	os.Exit(code)
}

const columnsQuery = `
SELECT table_name, column_name
FROM information_schema.columns
WHERE table_schema = 'public'
ORDER BY table_name, ordinal_position`

// liveSchema reads the tables the backend's migrations actually produce.
func liveSchema(t *testing.T) map[string][]string {
	t.Helper()

	if _, err := testenv.MigrationsDir(); errors.Is(err, testenv.ErrNoMigrations) {
		// A standalone checkout of this module — the Docker build context,
		// for instance — has nothing to check against.
		t.Skip("skipping: backend/migrations is not present")
	}

	db, err := sql.Open("postgres", testenv.Target(t))
	if err != nil {
		t.Fatalf("opening the target database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing the target database: %v", err)
		}
	})

	rows, err := db.QueryContext(context.Background(), columnsQuery)
	if err != nil {
		t.Fatalf("reading the target schema: %v", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string][]string{}
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			t.Fatalf("scanning the target schema: %v", err)
		}
		out[table] = append(out[table], column)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the target schema: %v", err)
	}
	for _, cols := range out {
		sort.Strings(cols)
	}
	return out
}

// Every table this package declares must exist with exactly the columns
// claimed. A column that has gone is the dangerous direction — a rule pointing
// at it writes nowhere — but an extra one means the declaration is stale, which
// is how a rule comes to be missing for a column that does exist.
func TestDeclarationMatchesTheRealSchema(t *testing.T) {
	live := liveSchema(t)
	declared := target.PostgreSQL()

	for _, table := range declared.Tables() {
		actual, ok := live[table]
		if !ok {
			t.Errorf("table %s is declared here but the migrations do not create it", table)
			continue
		}
		for _, c := range declared.Columns(table) {
			if !slices.Contains(actual, c) {
				t.Errorf("%s.%s is declared here but is not in the migrated schema", table, c)
			}
		}
		for _, c := range actual {
			if !declared.Has(table, c) {
				t.Errorf("%s.%s exists in the migrated schema but is missing from this declaration", table, c)
			}
		}
	}
}

// The migrations have to apply to an empty database. A migration that only
// works against a database that already has data in it is one the operator
// discovers on the night.
func TestMigrationsApplyToAnEmptyDatabase(t *testing.T) {
	live := liveSchema(t)

	if len(live) == 0 {
		t.Fatal("the migrated schema has no tables")
	}
	// goose's own bookkeeping proves the migrations ran rather than the
	// tables having appeared some other way.
	if _, ok := live["goose_db_version"]; !ok {
		t.Error("goose_db_version is missing; the migrations did not run")
	}
}

// Seeded tables have to be real tables, and they have to actually contain rows
// — the warning the mapping prints about id collisions is worthless if the
// table it names turns out to be empty.
func TestSeededTablesReallyShipWithRows(t *testing.T) {
	if _, err := testenv.MigrationsDir(); errors.Is(err, testenv.ErrNoMigrations) {
		t.Skip("skipping: backend/migrations is not present")
	}

	db, err := sql.Open("postgres", testenv.Target(t))
	if err != nil {
		t.Fatalf("opening the target database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	declared := target.PostgreSQL()
	for table, note := range target.Seeded {
		if !declared.HasTable(table) {
			t.Errorf("Seeded names %s, which is not declared", table)
			continue
		}

		var n int
		// #nosec G202 -- table comes from this package's own constant map.
		if err := db.QueryRow("SELECT COUNT(*) FROM " + pq(table)).Scan(&n); err != nil {
			t.Errorf("counting %s: %v", table, err)
			continue
		}
		if n == 0 {
			t.Errorf("%s is recorded as seeded (%s) but the migrations leave it empty", table, note)
		}
	}
}

// pq quotes a PostgreSQL identifier. The names come from a constant map in this
// package, but the query is still built by concatenation, so it is quoted.
func pq(name string) string {
	return `"` + name + `"`
}
