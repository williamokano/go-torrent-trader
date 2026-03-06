package repository

import (
	"context"
	"database/sql"
	"fmt"
)

// TxFunc is a function executed within a database transaction.
type TxFunc func(ctx context.Context, tx *sql.Tx) error

// WithTx executes fn within a database transaction.
// If fn returns an error or panics, the transaction is rolled back.
// Otherwise, the transaction is committed.
func WithTx(ctx context.Context, db *sql.DB, fn TxFunc) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()

	if err := fn(ctx, tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("rolling back transaction: %v (original error: %w)", rbErr, err)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}
