package postgres

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/williamokano/go-torrent-trader/backend/internal/database"
)

// postgresImage matches docker-compose so tests run against the same server
// version as dev and prod. A repository test that passes on a different major
// version is not evidence about the database we actually ship on.
const postgresImage = "postgres:16-alpine"

// testDB is a connection to a throwaway Postgres container, shared by every
// test in this package. It is nil under `go test -short`, where the container
// is never started.
var testDB *sql.DB

// TestMain starts one Postgres container for the whole package, applies the
// real goose migrations to it, and tears it down afterwards.
//
// It also exports TEST_DATABASE_URL, which is how the pre-existing repository
// integration tests (user, peer, torrent) locate a database. Those tests were
// written to skip when the variable is unset — and nothing ever set it, in CI
// or locally, so they had never actually run. Setting it here revives them
// without touching them.
func TestMain(m *testing.M) {
	// testing.Short() reads a flag, so the flags must be parsed before m.Run.
	flag.Parse()

	if testing.Short() {
		// No Docker in short mode: tests that need testDB skip themselves.
		os.Exit(m.Run())
	}

	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, postgresImage,
		tcpostgres.WithDatabase("torrenttrader_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			// Postgres logs "ready to accept connections" once while the entrypoint
			// initialises the cluster and again once it is serving for real. Waiting
			// for the second occurrence avoids connecting during the init phase.
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

// runWithContainer connects to the container, migrates it, and runs the suite.
// Split out from TestMain so failures can be returned rather than fatally
// exiting before the container is cleaned up.
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
	// these tests also catch migrations that fail to apply from scratch.
	if err := database.RunMigrations(db, "../../../migrations"); err != nil {
		return 0, fmt.Errorf("migrating test database: %w", err)
	}

	testDB = db

	// The pre-existing integration tests read this to find a database.
	if err := os.Setenv("TEST_DATABASE_URL", dsn); err != nil {
		return 0, fmt.Errorf("exporting TEST_DATABASE_URL: %w", err)
	}

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

// preservedTables are populated by the migrations themselves and are treated as
// reference data: other tests depend on them existing (a user needs a group, a
// torrent needs a category). Truncating them would silently break those tests,
// so resetTestData leaves them alone.
var preservedTables = map[string]bool{
	"goose_db_version": true, // migration bookkeeping; dropping it forces a re-migration
	"groups":           true,
	"categories":       true,
	"countries":        true,
	"languages":        true,
	"site_settings":    true,
}

// resetTestData empties every table that holds test-created rows, so each test
// starts from a known state. Seeded reference data (see preservedTables) is
// kept.
//
// RESTART IDENTITY resets sequences, so ID-ordering assertions do not depend on
// how many rows earlier tests happened to insert. CASCADE follows the FK graph,
// which BE-9.12 made strict.
func resetTestData(t *testing.T, db *sql.DB) {
	t.Helper()

	rows, err := db.Query(`SELECT tablename FROM pg_tables WHERE schemaname = 'public'`)
	if err != nil {
		t.Fatalf("listing tables: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scanning table name: %v", err)
		}
		if preservedTables[name] {
			continue
		}
		tables = append(tables, pq(name))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating tables: %v", err)
	}
	if len(tables) == 0 {
		return
	}

	stmt := "TRUNCATE TABLE " + strings.Join(tables, ", ") + " RESTART IDENTITY CASCADE"
	if _, err := db.Exec(stmt); err != nil {
		t.Fatalf("truncating tables: %v", err)
	}
}

// pq quotes an identifier for interpolation. Table names come from pg_tables on
// our own schema rather than from user input, but TRUNCATE cannot take them as
// bind parameters, so quote them rather than concatenating raw.
func pq(ident string) string {
	return `"` + ident + `"`
}
