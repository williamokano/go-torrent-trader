package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/williamokano/go-torrent-trader/backend/internal/database"
)

const postgresImage = "postgres:16-alpine"

var testDB *sql.DB

// TestMain mirrors internal/repository/postgres/main_test.go's pattern: one
// throwaway Postgres container per package run, migrated with the real goose
// migrations so this also catches a backfill that only works against a
// hand-maintained schema.
func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(m.Run())
	}

	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, postgresImage,
		tcpostgres.WithDatabase("backfill_mentions_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(90*time.Second),
		),
	)
	if err != nil {
		log.Fatalf("starting postgres container: %v", err)
	}

	code, err := runWithContainer(ctx, container, m)
	if termErr := testcontainers.TerminateContainer(container); termErr != nil {
		log.Printf("terminating postgres container: %v", termErr)
	}
	if err != nil {
		log.Fatalf("preparing test database: %v", err)
	}
	os.Exit(code)
}

func runWithContainer(ctx context.Context, container *tcpostgres.PostgresContainer, m *testing.M) (int, error) {
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return 0, fmt.Errorf("building connection string: %w", err)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return 0, fmt.Errorf("opening test database: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.PingContext(ctx); err != nil {
		return 0, fmt.Errorf("pinging test database: %w", err)
	}
	if err := database.RunMigrations(db, "../../migrations"); err != nil {
		return 0, fmt.Errorf("migrating test database: %w", err)
	}

	testDB = db
	return m.Run(), nil
}

func requireDB(t *testing.T) *sql.DB {
	t.Helper()
	if testDB == nil {
		t.Skip("skipping: needs the Postgres container (not available under -short)")
	}
	return testDB
}
