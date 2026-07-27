package target

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// maxParameters is PostgreSQL's hard limit on bind parameters in one statement.
// A batch of N rows of C columns sends N*C of them, so the batch size is capped
// by the column count rather than chosen freely — a 40-column table cannot take
// 2000 rows at once however large the configured batch is.
const maxParameters = 65535

// DefaultBatchSize is the batch used when none is configured. It is a
// compromise: large enough that the round trips stop dominating, small enough
// that a failed batch does not throw away much work and the parameter cap is
// not hit by an ordinary table.
const DefaultBatchSize = 500

// TxMode says how a Writer wraps its inserts in transactions.
type TxMode int

const (
	// TxPerBatch commits every batch. A failure leaves the batches before it
	// committed, which is what makes a run resumable, and what makes a
	// half-finished run visible rather than invisible.
	TxPerBatch TxMode = iota
	// TxPerTable commits once, when the table finishes. A failure rolls the
	// whole table back, so the target never holds a partial table — at the
	// cost of holding a transaction open for its whole duration.
	TxPerTable
)

// WriterOptions configures a Writer. The zero value is usable: batches of
// DefaultBatchSize, committed per batch.
type WriterOptions struct {
	BatchSize int
	TxMode    TxMode
}

// Writer inserts rows into one target table in batches.
//
// Rows are buffered and sent as a single multi-row INSERT. Sending them one at
// a time is the difference between a migration that takes minutes and one that
// takes hours, because the cost is dominated by round trips rather than by the
// inserts themselves.
type Writer struct {
	db        *sql.DB
	table     string
	columns   []string
	batchSize int
	txMode    TxMode

	insertSQL string // for a full batch, built once
	pending   []any
	rows      int
	written   int64

	tx *sql.Tx // held open only in TxPerTable

	// failed is sticky. Once a batch has failed, this writer is finished: the
	// rollback nils tx, so without it the next Append would open a *fresh*
	// transaction and Close would commit it — leaving a partial table in
	// TxPerTable, the one mode whose contract is that partial tables cannot
	// happen, and returning nil from Close while doing so. A caller that logs and
	// continues past one bad row is the ordinary way to hit that.
	failed error

	// Self-reference backfill. A table that points at itself cannot be written
	// naively in batches: foreign keys fire at end-of-statement, so one multi-row
	// INSERT is safe while a batch boundary is not — a row whose parent lands in a
	// later batch fails on the way in (#240).
	//
	// Deferring the constraint does not help: nothing in backend/migrations is
	// declared DEFERRABLE, and SET CONSTRAINTS ALL DEFERRED is silently ignored for
	// a NOT DEFERRABLE constraint. Verified against a real PostgreSQL rather than
	// assumed — the deferral looked like it worked and did nothing at all.
	//
	// So the column goes in as NULL and is set afterwards, once every row exists.
	// That works in both transaction modes and needs no schema change. All four
	// self-referencing columns are nullable, which is what makes it available.
	selfRefColumn string // "" when this table does not reference itself
	selfRefIndex  int    // its position in columns
	idIndex       int    // position of the primary key, needed to target the UPDATE
	deferred      []selfRef
}

// selfRef is one row's self-referencing value, held back until every row exists.
type selfRef struct {
	id     any
	parent any
}

// NewWriter builds a Writer for a table and a fixed column list. Every row
// appended must supply exactly these columns, in this order.
func (d *DB) NewWriter(table string, columns []string, opts WriterOptions) (*Writer, error) {
	if table == "" {
		return nil, errors.New("no table named")
	}
	if len(columns) == 0 {
		return nil, fmt.Errorf("%s: no columns named", table)
	}

	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}
	// Clamp rather than fail: an operator who asks for a bigger batch than
	// PostgreSQL can take wants speed, not an error message about a limit
	// they have no way to know.
	if perRow := len(columns); batchSize*perRow > maxParameters {
		batchSize = maxParameters / perRow
	}

	w := &Writer{
		db:           d.db,
		table:        table,
		columns:      columns,
		batchSize:    batchSize,
		txMode:       opts.TxMode,
		pending:      make([]any, 0, batchSize*len(columns)),
		selfRefIndex: -1,
		idIndex:      -1,
	}

	// Only when this writer is actually carrying the self-referencing column. A
	// caller that does not write it has nothing to hold back.
	if column, selfRef := SelfReferencing[table]; selfRef {
		w.selfRefIndex = indexOf(columns, column)
		w.idIndex = indexOf(columns, "id")
		if w.selfRefIndex >= 0 {
			if w.idIndex < 0 {
				return nil, fmt.Errorf("%s references itself through %s, so its id column "+
					"must be written too — the value is set in a second pass and there is "+
					"otherwise no way to say which row to set it on", table, column)
			}
			w.selfRefColumn = column
		}
	}

	w.insertSQL = w.buildInsert(batchSize)
	return w, nil
}

func indexOf(haystack []string, needle string) int {
	for i, s := range haystack {
		if s == needle {
			return i
		}
	}
	return -1
}

// BatchSize is the batch actually in use, which may be smaller than the one
// requested if the table has enough columns to hit the parameter cap.
func (w *Writer) BatchSize() int { return w.batchSize }

// Written is the number of rows sent to the database so far. Rows still
// buffered are not counted.
func (w *Writer) Written() int64 { return w.written }

// Append adds one row, flushing when the batch is full. The values must match
// the writer's columns in number and order.
func (w *Writer) Append(ctx context.Context, values ...any) error {
	if w.failed != nil {
		return w.failed
	}
	if len(values) != len(w.columns) {
		return fmt.Errorf("%s: %d values for %d columns", w.table, len(values), len(w.columns))
	}

	// Hold the self-referencing value back and insert NULL in its place. Recorded
	// with the row's id so the second pass knows where to put it.
	if w.selfRefColumn != "" {
		if parent := values[w.selfRefIndex]; parent != nil {
			w.deferred = append(w.deferred, selfRef{id: values[w.idIndex], parent: parent})
			// Copied because the caller owns the slice it passed and must not see
			// its value silently replaced with nil.
			values = append([]any{}, values...)
			values[w.selfRefIndex] = nil
		}
	}

	w.pending = append(w.pending, values...)
	w.rows++

	if w.rows >= w.batchSize {
		return w.flush(ctx, w.insertSQL)
	}
	return nil
}

// Flush sends any buffered rows.
func (w *Writer) Flush(ctx context.Context) error {
	if w.failed != nil {
		return w.failed
	}
	if w.rows == 0 {
		return nil
	}
	// A short final batch needs its own statement: the prebuilt one has
	// placeholders for a full batch.
	return w.flush(ctx, w.buildInsert(w.rows))
}

// Close flushes what is buffered and commits. It must be called, and its error
// checked — in TxPerTable everything written is still uncommitted until it
// returns.
func (w *Writer) Close(ctx context.Context) error {
	// Reported rather than swallowed. The doc tells callers to check Close's
	// error, and a nil from here after a lost batch is indistinguishable from a
	// clean finish — which is the worst answer this type can give.
	if w.failed != nil {
		return errors.Join(w.failed, w.rollback())
	}
	if err := w.Flush(ctx); err != nil {
		return errors.Join(err, w.rollback())
	}
	// TxPerBatch has already committed each batch, so there is no transaction left
	// here — but the backfill still has to run. Returning early on a nil tx skipped
	// it entirely in that mode, leaving every held-back value NULL, which is the
	// invite graph silently dropped rather than a visible failure.
	if w.tx != nil {
		tx := w.tx
		w.tx = nil
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing %s: %w", w.table, err)
		}
	}
	return w.backfillSelfReferences(ctx)
}

// backfillSelfReferences sets the values held back by Append, now that every row the
// writer was given exists.
//
// Runs after the commit rather than inside it. In TxPerBatch the earlier batches are
// already committed, so there is no single transaction to join; in TxPerTable the
// rows have to be visible to the foreign key check, which they are only once the
// insert has committed. Either way the parent is present by the time its child is
// pointed at it.
//
// A failure here leaves the rows in place with a NULL parent, which is recoverable —
// the operator can re-run this pass — and is reported rather than swallowed.
func (w *Writer) backfillSelfReferences(ctx context.Context) error {
	if len(w.deferred) == 0 {
		return nil
	}

	stmt := fmt.Sprintf("UPDATE %s SET %s = $1 WHERE %s = $2",
		QuoteIdentifier(w.table), QuoteIdentifier(w.selfRefColumn), QuoteIdentifier("id"))

	for _, ref := range w.deferred {
		if _, err := w.db.ExecContext(ctx, stmt, ref.parent, ref.id); err != nil {
			w.failed = fmt.Errorf("backfilling %s.%s for id %v: %w",
				w.table, w.selfRefColumn, ref.id, err)
			return w.failed
		}
	}
	w.deferred = nil
	return nil
}

// Rollback abandons anything uncommitted. In TxPerBatch the batches already
// committed stay committed; that is the mode's contract, not an oversight.
func (w *Writer) Rollback() error { return w.rollback() }

func (w *Writer) rollback() error {
	if w.tx == nil {
		return nil
	}
	tx := w.tx
	w.tx = nil
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		return fmt.Errorf("rolling back %s: %w", w.table, err)
	}
	return nil
}

func (w *Writer) flush(ctx context.Context, query string) error {
	rows := w.rows
	args := w.pending

	// Reset before the write, so a failed batch is not resent by a later
	// Flush on the way out.
	w.pending = make([]any, 0, w.batchSize*len(w.columns))
	w.rows = 0

	exec, err := w.executor(ctx)
	if err != nil {
		return err
	}
	if _, err := exec.ExecContext(ctx, query, args...); err != nil {
		w.failed = fmt.Errorf("inserting %d rows into %s: %w", rows, w.table, err)
		return errors.Join(w.failed, w.rollback())
	}

	if w.txMode == TxPerBatch && w.tx != nil {
		tx := w.tx
		w.tx = nil
		if err := tx.Commit(); err != nil {
			w.failed = fmt.Errorf("committing a batch of %s: %w", w.table, err)
			return w.failed
		}
	}
	// Counted only once the rows are actually durable. Incrementing before the
	// commit meant a connection dropped at commit time reported 1000 written with
	// 500 on disk — and #166 computing a resume offset from that would skip the
	// difference permanently.
	w.written += int64(rows)
	return nil
}

// executor returns whatever the next statement should run against: a fresh
// transaction per batch, one held open for the table, or the pool.
func (w *Writer) executor(ctx context.Context) (interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, error) {
	if w.tx != nil {
		return w.tx, nil
	}
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("starting a transaction for %s: %w", w.table, err)
	}

	w.tx = tx
	return tx, nil
}

// buildInsert renders a multi-row INSERT for exactly n rows.
func (w *Writer) buildInsert(n int) string {
	var b strings.Builder
	b.WriteString("INSERT INTO ")
	b.WriteString(QuoteIdentifier(w.table))
	b.WriteString(" (")
	for i, c := range w.columns {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(QuoteIdentifier(c))
	}
	b.WriteString(") VALUES ")

	param := 1
	for row := 0; row < n; row++ {
		if row > 0 {
			b.WriteString(", ")
		}
		b.WriteByte('(')
		for col := range w.columns {
			if col > 0 {
				b.WriteString(", ")
			}
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(param))
			param++
		}
		b.WriteByte(')')
	}
	return b.String()
}

// QuoteIdentifier renders a table or column name as a PostgreSQL identifier.
//
// Identifiers cannot be bound as parameters, so this is the only thing between
// a name and the query text. A double quote is escaped by doubling it, which is
// what PostgreSQL specifies; a NUL byte cannot appear in an identifier at all
// and is stripped rather than allowed to truncate the string.
func QuoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(name, "\x00", ""), `"`, `""`) + `"`
}
