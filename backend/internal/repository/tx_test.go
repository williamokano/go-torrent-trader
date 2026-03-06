package repository

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestWithTx_CommitsOnSuccess(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	db, err := openTestDB(dsn)
	if err != nil {
		t.Fatalf("opening test DB: %v", err)
	}
	defer func() { _ = db.Close() }()

	ctx := context.Background()

	_, err = db.ExecContext(ctx, "CREATE TEMP TABLE tx_test (val TEXT)")
	if err != nil {
		t.Fatalf("creating temp table: %v", err)
	}

	err = WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO tx_test (val) VALUES ($1)", "committed")
		return err
	})
	if err != nil {
		t.Fatalf("WithTx: %v", err)
	}

	var val string
	err = db.QueryRowContext(ctx, "SELECT val FROM tx_test").Scan(&val)
	if err != nil {
		t.Fatalf("querying: %v", err)
	}
	if val != "committed" {
		t.Errorf("got %q, want %q", val, "committed")
	}
}

func TestWithTx_RollsBackOnError(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	db, err := openTestDB(dsn)
	if err != nil {
		t.Fatalf("opening test DB: %v", err)
	}
	defer func() { _ = db.Close() }()

	ctx := context.Background()

	_, err = db.ExecContext(ctx, "CREATE TEMP TABLE tx_rollback_test (val TEXT)")
	if err != nil {
		t.Fatalf("creating temp table: %v", err)
	}

	sentinelErr := errors.New("rollback me")
	err = WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO tx_rollback_test (val) VALUES ($1)", "should-not-persist")
		if err != nil {
			return err
		}
		return sentinelErr
	})
	if !errors.Is(err, sentinelErr) {
		t.Fatalf("WithTx returned %v, want %v", err, sentinelErr)
	}

	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tx_rollback_test").Scan(&count)
	if err != nil {
		t.Fatalf("querying: %v", err)
	}
	if count != 0 {
		t.Errorf("got %d rows, want 0 (transaction should have been rolled back)", count)
	}
}
