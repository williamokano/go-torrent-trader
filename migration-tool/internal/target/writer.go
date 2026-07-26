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
		db:        d.db,
		table:     table,
		columns:   columns,
		batchSize: batchSize,
		txMode:    opts.TxMode,
		pending:   make([]any, 0, batchSize*len(columns)),
	}
	w.insertSQL = w.buildInsert(batchSize)
	return w, nil
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
	if len(values) != len(w.columns) {
		return fmt.Errorf("%s: %d values for %d columns", w.table, len(values), len(w.columns))
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
	if err := w.Flush(ctx); err != nil {
		return errors.Join(err, w.rollback())
	}
	if w.tx == nil {
		return nil
	}

	tx := w.tx
	w.tx = nil
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing %s: %w", w.table, err)
	}
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
		return errors.Join(fmt.Errorf("inserting %d rows into %s: %w", rows, w.table, err), w.rollback())
	}
	w.written += int64(rows)

	if w.txMode == TxPerBatch && w.tx != nil {
		tx := w.tx
		w.tx = nil
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing a batch of %s: %w", w.table, err)
		}
	}
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
