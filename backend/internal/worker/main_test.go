package worker

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

// postgresImage matches internal/repository/postgres's test image, which in
// turn matches docker-compose, so this package runs its DB-backed tests
// against the same server version as dev and prod.
const postgresImage = "postgres:16-alpine"

// testDB is a connection to a throwaway Postgres container, shared by every
// test in this package that needs one. It is nil under `go test -short`,
// where the container is never started.
var testDB *sql.DB

// TestMain mirrors internal/repository/postgres/main_test.go and
// cmd/backfill-mentions/main_test.go: one throwaway Postgres container per
// package run, migrated with the real goose migrations. The worker package
// has its own raw-SQL cleanup queries (internal/worker/handlers.go) that
// aren't behind a repository interface, so exercising them for real needs a
// real database rather than a mock.
func TestMain(m *testing.M) {
	// testing.Short() reads a flag, so the flags must be parsed before m.Run.
	flag.Parse()

	if testing.Short() {
		// No Docker in short mode: tests that need testDB skip themselves.
		os.Exit(m.Run())
	}

	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, postgresImage,
		tcpostgres.WithDatabase("worker_test"),
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

	// Terminate before exiting: os.Exit skips deferred functions, which would
	// leak the container.
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

	// Apply the real migrations rather than a hand-maintained test schema, so
	// this also catches a migration that fails to apply from scratch.
	if err := database.RunMigrations(db, "../../migrations"); err != nil {
		return 0, fmt.Errorf("migrating test database: %w", err)
	}

	testDB = db
	return m.Run(), nil
}

// requireDB returns the shared test database, skipping the test when the
// container was not started (`go test -short`).
func requireDB(t *testing.T) *sql.DB {
	t.Helper()
	if testDB == nil {
		t.Skip("skipping: needs the Postgres container (not available under -short)")
	}
	return testDB
}
