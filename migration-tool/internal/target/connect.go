package target

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	// Registers the "postgres" driver.
	_ "github.com/lib/pq"
)

// DB is a handle on the target database.
type DB struct {
	db       *sql.DB
	database string
	describe string
}

// Open connects to the target and verifies the connection.
func Open(ctx context.Context, rawDSN string) (*DB, error) {
	dsn, database, err := parseDSN(rawDSN)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening the target database: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
		return nil, fmt.Errorf("connecting to the target database %s: %w", DescribeDSN(rawDSN), err)
	}

	return &DB{db: db, database: database, describe: DescribeDSN(rawDSN)}, nil
}

// Close releases the connection pool.
func (d *DB) Close() error { return d.db.Close() }

// Database is the database name the handle is scoped to.
func (d *DB) Database() string { return d.database }

// String describes the connection without its password.
func (d *DB) String() string { return d.describe }

// SQL exposes the underlying handle for the writer. Nothing else should reach
// for it: every query this tool runs against the target belongs in this package,
// where the identifier quoting lives.
func (d *DB) SQL() *sql.DB { return d.db }

// parseDSN accepts the URL form an operator writes and returns a DSN the driver
// takes, plus the database name. lib/pq accepts a URL directly, so this exists
// to reject the mistakes worth catching before a connection is attempted rather
// than to rewrite anything.
func parseDSN(raw string) (dsn, database string, err error) {
	if strings.TrimSpace(raw) == "" {
		return "", "", errors.New("no target DSN: pass --target or set MIGRATION_TARGET_DSN")
	}

	// The key=value form is passed through: it has no URL structure to check,
	// and the driver's own error is clearer than anything invented here.
	if !strings.Contains(raw, "://") {
		return raw, databaseFromKeyValue(raw), nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		// url.Error embeds the URL, and the URL carries the password.
		return "", "", errors.New("target DSN is not a valid URL")
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return "", "", fmt.Errorf("target DSN scheme is %q, want postgres", u.Scheme)
	}

	database = strings.TrimPrefix(u.Path, "/")
	if database == "" {
		return "", "", errors.New("target DSN has no database name")
	}
	if strings.Contains(database, "/") {
		return "", "", errors.New("target DSN path is not a database name")
	}
	return raw, database, nil
}

func databaseFromKeyValue(raw string) string {
	for _, field := range strings.Fields(raw) {
		if name, ok := strings.CutPrefix(field, "dbname="); ok {
			return name
		}
	}
	return ""
}

// DescribeDSN renders a target DSN with the password removed. Nothing in this
// package prints one any other way.
func DescribeDSN(raw string) string {
	if !strings.Contains(raw, "://") {
		return redactKeyValue(raw)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "<unparseable DSN>"
	}

	// Built by hand rather than through url.String, which percent-encodes the
	// mask into "%2A%2A%2A" and makes the redaction look like a corrupt value.
	var b strings.Builder
	b.WriteString(u.Scheme)
	b.WriteString("://")
	if u.User != nil {
		b.WriteString(u.User.Username())
		if _, hasPassword := u.User.Password(); hasPassword {
			b.WriteString(":***")
		}
		b.WriteString("@")
	}
	b.WriteString(u.Host)
	b.WriteString(u.Path)
	return b.String()
}

func redactKeyValue(raw string) string {
	fields := strings.Fields(raw)
	for i, field := range fields {
		if strings.HasPrefix(field, "password=") {
			fields[i] = "password=***"
		}
	}
	return strings.Join(fields, " ")
}

const liveColumnsQuery = `
SELECT table_name, column_name
FROM information_schema.columns
WHERE table_schema = 'public'
ORDER BY table_name, ordinal_position`

// Schema reads the target's actual tables and columns.
func (d *DB) Schema(ctx context.Context) (Schema, error) {
	rows, err := d.db.QueryContext(ctx, liveColumnsQuery)
	if err != nil {
		return Schema{}, fmt.Errorf("reading the target schema: %w", err)
	}
	defer func() { _ = rows.Close() }()

	tables := map[string][]string{}
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			return Schema{}, fmt.Errorf("reading the target schema: %w", err)
		}
		tables[table] = append(tables[table], column)
	}
	if err := rows.Err(); err != nil {
		return Schema{}, fmt.Errorf("reading the target schema: %w", err)
	}
	if len(tables) == 0 {
		return Schema{}, fmt.Errorf("the target database %s has no tables — has the backend applied its migrations?", d.database)
	}

	for _, columns := range tables {
		sort.Strings(columns)
	}
	return Schema{tables: tables}, nil
}

// RowCounts counts the rows in each declared table.
//
// Feeds NonEmpty, which answers the question the pre-flight was not asking: does this
// database already have data in it? A table with non-seed rows means either a
// half-finished previous run or a live site, and the second is unrecoverable — so it
// is worth stopping for, and it is arguably the most dangerous question after schema
// drift.
//
// Counted per table rather than in one statement because the set is small and fixed,
// and a table that has gone should be reported by the schema check rather than
// breaking this one.
func (d *DB) RowCounts(ctx context.Context, live Schema) (map[string]int64, error) {
	counts := make(map[string]int64, len(DeclaredTables()))
	for _, table := range DeclaredTables() {
		if !live.HasTable(table) {
			// Missing tables are the schema check's problem, not this one's.
			continue
		}
		var n int64
		// #nosec G202 -- table comes from this package's own declaration, and is
		// quoted; it is never operator input.
		q := "SELECT COUNT(*) FROM " + QuoteIdentifier(table)
		if err := d.db.QueryRowContext(ctx, q).Scan(&n); err != nil {
			return nil, fmt.Errorf("counting rows in %s: %w", table, err)
		}
		counts[table] = n
	}
	return counts, nil
}
