// Package source reads a legacy TorrentTrader MySQL database: what tables it
// has, what columns they carry, and how much data is in them. It only reads.
package source

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/go-sql-driver/mysql"

	"github.com/williamokano/go-torrent-trader/migration-tool/internal/schema"
)

// DB is a read-only handle on the legacy database.
type DB struct {
	db       *sql.DB
	database string
	describe string
}

// Open parses the DSN, connects, and verifies the connection. The returned DB
// must be closed by the caller.
func Open(ctx context.Context, rawDSN string) (*DB, error) {
	dsn, err := ParseDSN(rawDSN)
	if err != nil {
		return nil, err
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("source DSN is not valid: %w", err)
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening source database: %w", err)
	}
	// Introspection is a handful of sequential queries; a large pool buys
	// nothing and a legacy server may be tightly limited.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)

	if err := db.PingContext(ctx); err != nil {
		// The ping error can name the address but never the password.
		if closeErr := db.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
		return nil, fmt.Errorf("connecting to source database %s: %w", DescribeDSN(rawDSN), err)
	}

	return &DB{db: db, database: cfg.DBName, describe: DescribeDSN(rawDSN)}, nil
}

// Close releases the connection pool.
func (d *DB) Close() error { return d.db.Close() }

// Database is the schema name the handle is scoped to.
func (d *DB) Database() string { return d.database }

// String describes the connection without its password.
func (d *DB) String() string { return d.describe }

const tablesQuery = `
SELECT TABLE_NAME, COALESCE(ENGINE, ''), COALESCE(TABLE_ROWS, 0)
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'BASE TABLE'
ORDER BY TABLE_NAME`

const columnsQuery = `
SELECT TABLE_NAME, COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE,
       COALESCE(COLUMN_DEFAULT, ''), COALESCE(COLUMN_KEY, ''), COALESCE(EXTRA, '')
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = ?
ORDER BY TABLE_NAME, ORDINAL_POSITION`

// Schema reads every base table and column in the database. Table.Rows carries
// the storage engine's row estimate, which for MyISAM — what TorrentTrader
// uses throughout — is exact, and for InnoDB is not. Use CountRows where the
// number has to be right.
func (d *DB) Schema(ctx context.Context) (schema.Schema, error) {
	tables, err := d.tables(ctx)
	if err != nil {
		return schema.Schema{}, err
	}
	columns, err := d.columns(ctx)
	if err != nil {
		return schema.Schema{}, err
	}

	out := schema.Schema{Database: d.database, Tables: make([]schema.Table, 0, len(tables))}
	for _, t := range tables {
		t.Columns = columns[t.Name]
		out.Tables = append(out.Tables, t)
	}
	return out, nil
}

func (d *DB) tables(ctx context.Context) ([]schema.Table, error) {
	rows, err := d.db.QueryContext(ctx, tablesQuery, d.database)
	if err != nil {
		return nil, fmt.Errorf("listing tables in %s: %w", d.database, err)
	}
	defer func() { _ = rows.Close() }()

	var tables []schema.Table
	for rows.Next() {
		var t schema.Table
		if err := rows.Scan(&t.Name, &t.Engine, &t.Rows); err != nil {
			return nil, fmt.Errorf("reading table list: %w", err)
		}
		tables = append(tables, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading table list: %w", err)
	}
	if len(tables) == 0 {
		return nil, fmt.Errorf("database %s has no tables", d.database)
	}
	return tables, nil
}

func (d *DB) columns(ctx context.Context) (map[string][]schema.Column, error) {
	rows, err := d.db.QueryContext(ctx, columnsQuery, d.database)
	if err != nil {
		return nil, fmt.Errorf("listing columns in %s: %w", d.database, err)
	}
	defer func() { _ = rows.Close() }()

	byTable := make(map[string][]schema.Column)
	for rows.Next() {
		var table, nullable string
		var c schema.Column
		if err := rows.Scan(&table, &c.Name, &c.Type, &nullable, &c.Default, &c.Key, &c.Extra); err != nil {
			return nil, fmt.Errorf("reading column list: %w", err)
		}
		c.Nullable = strings.EqualFold(nullable, "YES")
		byTable[table] = append(byTable[table], c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading column list: %w", err)
	}
	return byTable, nil
}

// CountRows runs an exact COUNT(*). On a large MyISAM table this is cheap; on
// InnoDB it is a full scan, which is why it is a separate call from Schema.
func (d *DB) CountRows(ctx context.Context, table string) (int64, error) {
	ident, err := quoteIdentifier(table)
	if err != nil {
		return 0, err
	}
	var n int64
	// #nosec G202 -- the identifier is quoted and validated by quoteIdentifier;
	// MySQL does not accept a placeholder in this position.
	if err := d.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+ident).Scan(&n); err != nil {
		return 0, fmt.Errorf("counting rows in %s: %w", table, err)
	}
	return n, nil
}

// CreateTable returns the table's DDL as SHOW CREATE TABLE reports it. This is
// what an operator should attach to a bug report when a mapping looks wrong.
func (d *DB) CreateTable(ctx context.Context, table string) (string, error) {
	ident, err := quoteIdentifier(table)
	if err != nil {
		return "", err
	}
	var name, ddl string
	// #nosec G202 -- see CountRows.
	if err := d.db.QueryRowContext(ctx, "SHOW CREATE TABLE "+ident).Scan(&name, &ddl); err != nil {
		return "", fmt.Errorf("reading DDL for %s: %w", table, err)
	}
	return ddl, nil
}

// quoteIdentifier renders a table name as a MySQL identifier. Identifiers
// cannot be parameterized, so this is the only thing standing between a table
// name and the query text: a backtick is escaped by doubling, and the
// characters MySQL does not allow in an identifier at all are refused.
func quoteIdentifier(name string) (string, error) {
	if name == "" {
		return "", errors.New("empty table name")
	}
	if strings.ContainsRune(name, 0) {
		return "", fmt.Errorf("table name %q contains a NUL byte", name)
	}
	return "`" + strings.ReplaceAll(name, "`", "``") + "`", nil
}
