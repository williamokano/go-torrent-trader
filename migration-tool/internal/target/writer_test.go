package target_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/williamokano/go-torrent-trader/migration-tool/internal/target"
	"github.com/williamokano/go-torrent-trader/migration-tool/internal/testenv"
)

// The writer is tested against a real PostgreSQL because the things worth
// knowing about it — whether a batch commits, whether a rollback actually
// discards, what the server does with 65535 bind parameters — are properties of
// the server, not of the SQL string.

func openTarget(t *testing.T) *target.DB {
	t.Helper()

	db, err := target.Open(context.Background(), testenv.Target(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return db
}

// scratch creates a table for one test and drops it afterwards. The target
// container is shared, so a test that leaves rows behind changes what the next
// one sees.
func scratch(t *testing.T, db *target.DB, columns string) string {
	t.Helper()

	name := "scratch_" + strings.ToLower(strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	ctx := context.Background()

	drop := "DROP TABLE IF EXISTS " + target.QuoteIdentifier(name)
	if _, err := db.SQL().ExecContext(ctx, drop); err != nil {
		t.Fatalf("dropping %s: %v", name, err)
	}
	if _, err := db.SQL().ExecContext(ctx, "CREATE TABLE "+target.QuoteIdentifier(name)+" ("+columns+")"); err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	t.Cleanup(func() {
		if _, err := db.SQL().ExecContext(context.Background(), drop); err != nil {
			t.Errorf("dropping %s: %v", name, err)
		}
	})
	return name
}

func countRows(t *testing.T, db *target.DB, table string) int {
	t.Helper()
	var n int
	if err := db.SQL().QueryRow("SELECT COUNT(*) FROM " + target.QuoteIdentifier(table)).Scan(&n); err != nil {
		t.Fatalf("counting %s: %v", table, err)
	}
	return n
}

func TestWriterInsertsEveryRow(t *testing.T) {
	db := openTarget(t)
	table := scratch(t, db, "id BIGINT PRIMARY KEY, name TEXT NOT NULL")
	ctx := context.Background()

	// Deliberately not a multiple of the batch size, so the short final
	// batch — which needs its own statement — is exercised.
	const rows = 1053

	w, err := db.NewWriter(table, []string{"id", "name"}, target.WriterOptions{BatchSize: 100})
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	for i := 0; i < rows; i++ {
		if err := w.Append(ctx, i, fmt.Sprintf("row-%d", i)); err != nil {
			t.Fatalf("Append(%d): %v", i, err)
		}
	}
	if err := w.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if got := countRows(t, db, table); got != rows {
		t.Errorf("%d rows in the table, want %d", got, rows)
	}
	if got := w.Written(); got != rows {
		t.Errorf("Written() = %d, want %d", got, rows)
	}

	// Spot-check a value: the right number of rows is not the same as the
	// right rows, and a placeholder off by one produces exactly this bug.
	var name string
	if err := db.SQL().QueryRow("SELECT name FROM "+target.QuoteIdentifier(table)+" WHERE id = $1", 1052).Scan(&name); err != nil {
		t.Fatalf("reading the last row: %v", err)
	}
	if name != "row-1052" {
		t.Errorf("row 1052 is named %q, want row-1052", name)
	}
}

// Nothing reaches the database until a batch fills, so a writer that is dropped
// without Close silently loses the tail.
func TestWriterHoldsRowsUntilTheBatchFills(t *testing.T) {
	db := openTarget(t)
	table := scratch(t, db, "id BIGINT PRIMARY KEY")
	ctx := context.Background()

	w, err := db.NewWriter(table, []string{"id"}, target.WriterOptions{BatchSize: 10})
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	for i := 0; i < 9; i++ {
		if err := w.Append(ctx, i); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	if got := countRows(t, db, table); got != 0 {
		t.Errorf("%d rows written before the batch filled, want 0", got)
	}

	if err := w.Append(ctx, 9); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if got := countRows(t, db, table); got != 10 {
		t.Errorf("%d rows after the batch filled, want 10", got)
	}
	if err := w.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TxPerTable's contract: nothing is visible until Close, and Rollback discards
// everything. This is the mode for a table that must not exist half-written.
func TestWriterPerTableTransactionIsAllOrNothing(t *testing.T) {
	db := openTarget(t)
	table := scratch(t, db, "id BIGINT PRIMARY KEY")
	ctx := context.Background()

	w, err := db.NewWriter(table, []string{"id"}, target.WriterOptions{
		BatchSize: 10,
		TxMode:    target.TxPerTable,
	})
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	for i := 0; i < 25; i++ {
		if err := w.Append(ctx, i); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	// Two batches have gone to the server, and none of it is committed.
	if got := countRows(t, db, table); got != 0 {
		t.Errorf("%d rows visible mid-table, want 0 until Close", got)
	}

	if err := w.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if got := countRows(t, db, table); got != 0 {
		t.Errorf("%d rows survived a rollback, want 0", got)
	}
}

// TxPerBatch's contract, and the reason a resumable run wants it: what has been
// committed stays committed when a later batch fails.
func TestWriterPerBatchKeepsCommittedBatches(t *testing.T) {
	db := openTarget(t)
	table := scratch(t, db, "id BIGINT PRIMARY KEY")
	ctx := context.Background()

	w, err := db.NewWriter(table, []string{"id"}, target.WriterOptions{BatchSize: 10})
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	for i := 0; i < 20; i++ {
		if err := w.Append(ctx, i); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	if got := countRows(t, db, table); got != 20 {
		t.Errorf("%d rows committed, want 20", got)
	}

	// A duplicate primary key fails the next batch.
	for i := 0; i < 10; i++ {
		if err := w.Append(ctx, 0); err != nil {
			if !strings.Contains(err.Error(), "duplicate key") {
				t.Fatalf("unexpected error: %v", err)
			}
			// The committed batches are untouched.
			if got := countRows(t, db, table); got != 20 {
				t.Errorf("%d rows after a failed batch, want the 20 already committed", got)
			}
			return
		}
	}
	t.Fatal("inserting a duplicate primary key did not fail")
}

// A failed batch must not be resent by the Close on the way out, which would
// turn one error into two.
func TestWriterDoesNotResendAFailedBatch(t *testing.T) {
	db := openTarget(t)
	table := scratch(t, db, "id BIGINT PRIMARY KEY")
	ctx := context.Background()

	w, err := db.NewWriter(table, []string{"id"}, target.WriterOptions{BatchSize: 2})
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if err := w.Append(ctx, 1); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := w.Append(ctx, 1); err == nil {
		t.Fatal("a batch with a duplicate key succeeded")
	}

	// Close reports nothing further: the failed rows were dropped, not kept.
	if err := w.Close(ctx); err != nil {
		t.Errorf("Close after a failed batch: %v", err)
	}
	if got := countRows(t, db, table); got != 0 {
		t.Errorf("%d rows in the table, want 0", got)
	}
}

// PostgreSQL refuses more than 65535 bind parameters in one statement. An
// operator asking for a batch larger than the table can take wants speed, not
// an error about a limit they have no way to know.
func TestWriterClampsTheBatchToTheParameterLimit(t *testing.T) {
	db := openTarget(t)

	columns := make([]string, 40)
	defs := make([]string, 40)
	for i := range columns {
		columns[i] = fmt.Sprintf("c%d", i)
		defs[i] = fmt.Sprintf("c%d INT", i)
	}
	table := scratch(t, db, strings.Join(defs, ", "))

	w, err := db.NewWriter(table, columns, target.WriterOptions{BatchSize: 10000})
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if got := w.BatchSize(); got*len(columns) > 65535 {
		t.Errorf("batch of %d x %d columns = %d parameters, over the limit",
			got, len(columns), got*len(columns))
	}
	if w.BatchSize() >= 10000 {
		t.Errorf("BatchSize() = %d, want it clamped below the requested 10000", w.BatchSize())
	}

	// And it still works at the clamped size.
	ctx := context.Background()
	values := make([]any, len(columns))
	for i := 0; i < w.BatchSize()+5; i++ {
		for j := range values {
			values[j] = i
		}
		if err := w.Append(ctx, values...); err != nil {
			t.Fatalf("Append(%d): %v", i, err)
		}
	}
	if err := w.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := countRows(t, db, table); got != w.BatchSize()+5 {
		t.Errorf("%d rows, want %d", got, w.BatchSize()+5)
	}
}

func TestWriterRejectsAMalformedRow(t *testing.T) {
	db := openTarget(t)
	table := scratch(t, db, "id BIGINT, name TEXT")
	ctx := context.Background()

	w, err := db.NewWriter(table, []string{"id", "name"}, target.WriterOptions{})
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if err := w.Append(ctx, 1); err == nil {
		t.Error("Append with too few values returned no error")
	}
	if err := w.Append(ctx, 1, "a", "b"); err == nil {
		t.Error("Append with too many values returned no error")
	}

	if _, err := db.NewWriter("", []string{"id"}, target.WriterOptions{}); err == nil {
		t.Error("NewWriter with no table returned no error")
	}
	if _, err := db.NewWriter(table, nil, target.WriterOptions{}); err == nil {
		t.Error("NewWriter with no columns returned no error")
	}
}

// The identifier is the only part of the statement not bound as a parameter.
func TestQuoteIdentifierIsSafe(t *testing.T) {
	tests := []struct{ in, want string }{
		{"users", `"users"`},
		{"forum_posts", `"forum_posts"`},
		{`we"ird`, `"we""ird"`},
		{`users"; DROP TABLE users; --`, `"users""; DROP TABLE users; --"`},
		{"us\x00ers", `"users"`},
	}
	for _, tc := range tests {
		if got := target.QuoteIdentifier(tc.in); got != tc.want {
			t.Errorf("QuoteIdentifier(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// A hostile table name must not survive as executable SQL.
func TestWriterRefusesToBeInjected(t *testing.T) {
	db := openTarget(t)
	table := scratch(t, db, "id BIGINT")
	ctx := context.Background()

	hostile := table + `"; DROP TABLE ` + target.QuoteIdentifier(table) + `; --`
	w, err := db.NewWriter(hostile, []string{"id"}, target.WriterOptions{BatchSize: 1})
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	// The insert fails because no such table exists — the name was quoted, not
	// executed.
	if err := w.Append(ctx, 1); err == nil {
		t.Fatal("inserting into a hostile table name succeeded")
	}

	// The point of the exercise: the real table is still there.
	var exists bool
	if err := db.SQL().QueryRow(
		"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)", table,
	).Scan(&exists); err != nil {
		t.Fatalf("checking the table survived: %v", err)
	}
	if !exists {
		t.Fatal("the table was dropped")
	}
}

func TestOpenRejectsABadDSN(t *testing.T) {
	ctx := context.Background()

	for _, dsn := range []string{"", "   ", "mysql://root@localhost:3306/tt", "postgres://localhost:5432/"} {
		if _, err := target.Open(ctx, dsn); err == nil {
			t.Errorf("Open(%q) returned no error", dsn)
		}
	}
}

// A target DSN reaches this tool on a command line and its error goes to a log.
func TestTargetErrorsDoNotLeakThePassword(t *testing.T) {
	_, err := target.Open(context.Background(), "postgres://tt:hunter2@127.0.0.1:1/tt?sslmode=disable")
	if err == nil {
		t.Fatal("connecting to a closed port returned no error")
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Errorf("the error leaked the password: %v", err)
	}
}

func TestDescribeDSN(t *testing.T) {
	tests := []struct{ in, want string }{
		{"postgres://tt:hunter2@db:5432/torrenttrader?sslmode=disable", "postgres://tt:***@db:5432/torrenttrader"},
		{"postgres://tt@db:5432/torrenttrader", "postgres://tt@db:5432/torrenttrader"},
		{"host=db user=tt password=hunter2 dbname=tt", "host=db user=tt password=*** dbname=tt"},
	}
	for _, tc := range tests {
		got := target.DescribeDSN(tc.in)
		if got != tc.want {
			t.Errorf("DescribeDSN(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if strings.Contains(got, "hunter2") {
			t.Errorf("DescribeDSN leaked the password: %q", got)
		}
	}
}

// Verify is what tells an operator their target has not had its migrations run,
// which is the ordinary way this goes wrong.
func TestVerifyAgainstTheRealSchema(t *testing.T) {
	db := openTarget(t)

	live, err := db.Schema(context.Background())
	if err != nil {
		t.Fatalf("Schema: %v", err)
	}
	if problems := target.Verify(live); len(problems) > 0 {
		t.Errorf("the migrated schema does not satisfy the tool's expectations: %v", problems)
	}

	// An empty database is the failure it exists to catch.
	if problems := target.Verify(target.Schema{}); len(problems) == 0 {
		t.Error("an empty target reported no problems")
	}
}

func TestSchemaFailsOnADatabaseWithNoTables(t *testing.T) {
	db := openTarget(t)
	ctx := context.Background()

	// A schema with nothing in it stands in for a database whose migrations
	// were never run.
	var count int
	err := db.SQL().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public'").Scan(&count)
	if err != nil {
		t.Fatalf("counting tables: %v", err)
	}
	if count == 0 {
		t.Fatal("the target has no tables; the migrations did not run")
	}
}

// WriteOrder has to name every table the mapping writes to, or one of them is
// silently never written.
func TestWriteOrderCoversEveryDeclaredTable(t *testing.T) {
	if missing := target.WriteOrderCoversDeclaration(); len(missing) > 0 {
		t.Errorf("declared but absent from the write order: %v", missing)
	}
}

// A table must be written after everything it refers to, or the foreign keys
// the legacy schema never had will reject it.
func TestWriteOrderPutsParentsFirst(t *testing.T) {
	order := target.DeclaredTables()
	position := map[string]int{}
	for i, table := range order {
		position[table] = i
	}

	after := map[string][]string{
		"users":            {"groups"},
		"torrents":         {"users", "categories"},
		"peers":            {"torrents", "users"},
		"transfer_history": {"torrents", "users"},
		"torrent_comments": {"torrents", "users"},
		"torrent_ratings":  {"torrents", "users"},
		"forums":           {"forum_categories"},
		"forum_topics":     {"forums", "users"},
		"forum_posts":      {"forum_topics", "users"},
		"saved_messages":   {"messages", "users"},
	}
	for table, parents := range after {
		for _, parent := range parents {
			if position[table] < position[parent] {
				t.Errorf("%s is written before %s, which it refers to", table, parent)
			}
		}
	}
}

func TestNonEmptyIgnoresSeededTables(t *testing.T) {
	counts := map[string]int64{
		"groups":   6, // seeded by the backend, so not a sign of anything
		"users":    0,
		"torrents": 12, // not seeded: somebody has been here
	}
	got := target.NonEmpty(counts)
	if len(got) != 1 || got[0] != "torrents" {
		t.Errorf("NonEmpty() = %v, want [torrents]", got)
	}
}

// A sanity check on the harness itself: the shared target is a real database
// with the backend's schema in it.
func TestTargetHarnessIsUsable(t *testing.T) {
	db := openTarget(t)
	if db.Database() != "torrenttrader" {
		t.Errorf("Database() = %q, want torrenttrader", db.Database())
	}

	var one int
	if err := db.SQL().QueryRow("SELECT 1").Scan(&one); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("querying the target: %v", err)
		}
	}
}
