package postgres

import (
	"context"
	"database/sql"
)

// DBTX is the common interface satisfied by both *sql.DB and *sql.Tx.
// Repository structs use this so they can operate within a transaction
// or against the connection pool directly.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}
