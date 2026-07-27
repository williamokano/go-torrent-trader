package target_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"slices"
	"sort"
	"strconv"
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
			continue
		}
		// The count in the note is printed into the operator's mapping file, so a
		// wrong one is a wrong instruction rather than an untidy comment. `groups`
		// said 6 and had 7 — the Uploader group arrived in a later migration and
		// nothing here was watching.
		if want, ok := leadingCount(note); ok && want != n {
			t.Errorf("%s holds %d rows but Seeded says %q", table, n, note)
		}
	}
}

// The direction that was missing, and the reason `forums` was absent from Seeded
// for as long as it was: the loop above only asks "does every table we declared
// as seeded have rows?", which cannot notice a seeded table nobody declared.
//
// 039_create_forums.sql seeds 6 forums. Because the mapping keeps legacy ids so
// foreign keys resolve, that meant a legacy install with forum ids starting at 1
// aborted the forum load on a primary key collision — and one whose ids started
// above 6 finished "successfully" with 6 phantom stock forums and a BIGSERIAL that
// had never advanced, so the first forum created after cutover collided too.
//
// Any table the migrations populate has to be declared here, whether or not the
// mapping currently writes to it: the next rule that starts writing to it inherits
// the collision, and the warning is generated from this map.
func TestEverySeededTargetTableIsDeclaredAsSeeded(t *testing.T) {
	if _, err := testenv.MigrationsDir(); errors.Is(err, testenv.ErrNoMigrations) {
		t.Skip("skipping: backend/migrations is not present")
	}

	db, err := sql.Open("postgres", testenv.Target(t))
	if err != nil {
		t.Fatalf("opening the target database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, table := range target.PostgreSQL().Tables() {
		var n int
		// #nosec G202 -- table comes from this package's own declaration.
		if err := db.QueryRow("SELECT COUNT(*) FROM " + pq(table)).Scan(&n); err != nil {
			t.Errorf("counting %s: %v", table, err)
			continue
		}
		if n == 0 {
			continue
		}
		if _, declared := target.Seeded[table]; !declared {
			t.Errorf("the migrations leave %d rows in %s, but it is not in target.Seeded — "+
				"a mapping that keeps legacy ids will collide with them, and nothing "+
				"warns the operator", n, table)
		}
	}
}

// leadingCount reads the row count off the front of a Seeded note ("6 rows, ...").
func leadingCount(note string) (int, bool) {
	end := 0
	for end < len(note) && note[end] >= '0' && note[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(note[:end])
	if err != nil {
		return 0, false
	}
	return n, true
}

// pq quotes a PostgreSQL identifier. The names come from a constant map in this
// package, but the query is still built by concatenation, so it is quoted.
func pq(name string) string {
	return `"` + name + `"`
}

// The self-referencing tables are declared by hand, so they are checked against the
// real schema — derived from pg_constraint rather than from a second hand-typed
// list, which would agree with the first for the same wrong reason.
//
// This matters because no table ordering can make a self-reference safe: foreign keys
// fire at end-of-statement, so one multi-row INSERT is fine while crossing a batch
// boundary is not. A self-reference added to the schema later and missed here would
// reintroduce exactly that, and the failure only shows up on a real cutover at a real
// batch size.
func TestSelfReferencingMatchesTheRealSchema(t *testing.T) {
	if _, err := testenv.MigrationsDir(); errors.Is(err, testenv.ErrNoMigrations) {
		t.Skip("skipping: backend/migrations is not present")
	}

	db, err := sql.Open("postgres", testenv.Target(t))
	if err != nil {
		t.Fatalf("opening the target database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	const query = `
SELECT c.conrelid::regclass::text AS tbl, a.attname AS col
FROM pg_constraint c
JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = ANY (c.conkey)
WHERE c.contype = 'f'
  AND c.conrelid = c.confrelid
  AND connamespace = 'public'::regnamespace`

	rows, err := db.QueryContext(context.Background(), query)
	if err != nil {
		t.Fatalf("reading self-referencing constraints: %v", err)
	}
	defer func() { _ = rows.Close() }()

	live := map[string]string{}
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			t.Fatalf("scanning: %v", err)
		}
		live[table] = column
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading self-referencing constraints: %v", err)
	}

	declared := target.PostgreSQL()
	for table, column := range live {
		// Only tables the migration writes to are this tool's business.
		if !declared.HasTable(table) {
			continue
		}
		got, ok := target.SelfReferencing[table]
		if !ok {
			t.Errorf("%s.%s is a self-referencing foreign key in the real schema but is "+
				"not in target.SelfReferencing — a batched write to it will fail on a "+
				"row whose parent lands in a later batch, and nothing warns", table, column)
			continue
		}
		if got != column {
			t.Errorf("target.SelfReferencing[%q] = %q, but the real column is %q", table, got, column)
		}
	}

	for table, column := range target.SelfReferencing {
		if liveCol, ok := live[table]; !ok {
			t.Errorf("target.SelfReferencing names %s.%s, which is not a self-referencing "+
				"foreign key in the real schema", table, column)
		} else if liveCol != column {
			t.Errorf("target.SelfReferencing[%q] = %q, real schema says %q", table, column, liveCol)
		}
	}
}
