// Package testenv builds the two databases the tool's tests run against: a
// MySQL loaded with the legacy corpus, and a PostgreSQL built from the
// backend's real migrations.
//
// Both are real servers in containers rather than fakes, because the things
// most likely to be wrong here are not the tool's logic but its assumptions
// about what a server does — how MySQL 8 reports a type it was given in 2008,
// whether a legacy zero date survives the trip, what PostgreSQL does with a
// foreign key the MyISAM schema never enforced. A fake agrees with whatever the
// author believed, which is exactly the thing under test.
//
// This package is test-only infrastructure. Nothing in cmd/ or the other
// internal packages imports it.
package testenv

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	// Registers the "postgres" driver for goose.
	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// startTimeout bounds container startup. Pulling an image on a cold CI runner
// is the slow part, not the server.
const startTimeout = 5 * time.Minute

// LegacyCorpus is the legacy database every test runs against, embedded so it
// can be loaded from any package regardless of the working directory.
//
//go:embed testdata/legacy.sql
var LegacyCorpus string

// Counts the corpus is built to have. They live here, next to the corpus
// itself, so adding a row means updating the number in the same diff rather
// than discovering it as a failure in another package.
const (
	CorpusUsers    = 8
	CorpusTorrents = 5
	CorpusPeers    = 3
	CorpusTables   = 15
	// UserColumns is 43 stock columns plus the two a mod added.
	UserColumns = 45
)

// container is one lazily-started, shared server. Starting a database costs
// more than every test that uses it, and nothing here writes to the source, so
// one server serves the whole test binary.
type container struct {
	once sync.Once
	dsn  string
	err  error
	stop func()
}

func (c *container) get(t testing.TB, start func() (string, func(), error)) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: needs Docker")
	}

	c.once.Do(func() {
		c.dsn, c.stop, c.err = start()
	})
	if c.err != nil {
		t.Fatal(c.err)
	}
	return c.dsn
}

func (c *container) terminate() {
	if c.stop != nil {
		c.stop()
	}
}

var (
	legacy = &container{}
	target = &container{}
)

// Cleanup terminates every container this package started. Call it from
// TestMain after m.Run returns.
func Cleanup() {
	legacy.terminate()
	target.terminate()
}

// Legacy returns a DSN for a MySQL server loaded with the legacy corpus. The
// database is read-only as far as the tests are concerned.
func Legacy(t testing.TB) string {
	return legacy.get(t, startLegacy)
}

// Target returns a DSN for a PostgreSQL server with the backend's migrations
// applied.
//
// Tests that write to it are responsible for leaving it as they found it — one
// server is shared, so a test that migrates data into it and does not clean up
// changes what the next test sees.
func Target(t testing.TB) string {
	return target.get(t, startTarget)
}

func startLegacy() (string, func(), error) {
	ctx, cancel := context.WithTimeout(context.Background(), startTimeout)
	defer cancel()

	// The corpus is embedded, so it is written out here rather than mounted
	// from a path that only resolves for one package.
	script, cleanupScript, err := writeTemp("legacy-*.sql", LegacyCorpus)
	if err != nil {
		return "", nil, err
	}
	defer cleanupScript()

	c, err := tcmysql.Run(ctx,
		"mysql:8.0",
		tcmysql.WithDatabase("torrenttrader"),
		tcmysql.WithUsername("tt"),
		tcmysql.WithPassword("tt-secret"),
		tcmysql.WithScripts(script),
	)
	if err != nil {
		return "", nil, fmt.Errorf("starting MySQL: %w", err)
	}
	stop := terminator(c, "MySQL")

	dsn, err := c.ConnectionString(ctx)
	if err != nil {
		stop()
		return "", nil, fmt.Errorf("building MySQL connection string: %w", err)
	}
	return dsn, stop, nil
}

func startTarget() (string, func(), error) {
	ctx, cancel := context.WithTimeout(context.Background(), startTimeout)
	defer cancel()

	c, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("torrenttrader"),
		tcpostgres.WithUsername("tt"),
		tcpostgres.WithPassword("tt-secret"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(startTimeout),
		),
	)
	if err != nil {
		return "", nil, fmt.Errorf("starting PostgreSQL: %w", err)
	}
	stop := terminator(c, "PostgreSQL")

	dsn, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		stop()
		return "", nil, fmt.Errorf("building PostgreSQL connection string: %w", err)
	}

	if err := applyMigrations(dsn); err != nil {
		stop()
		return "", nil, err
	}
	return dsn, stop, nil
}

// applyMigrations builds the target schema by running the backend's own goose
// migrations. Anything else — a transcribed schema, a dump — is a second copy
// to keep in sync, and this repository has already paid for one of those.
func applyMigrations(dsn string) error {
	dir, err := MigrationsDir()
	if err != nil {
		return err
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("opening the target database: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("selecting the goose dialect: %w", err)
	}
	goose.SetLogger(goose.NopLogger())

	if err := goose.Up(db, dir); err != nil {
		return fmt.Errorf("applying backend migrations from %s: %w", dir, err)
	}
	return nil
}

// ErrNoMigrations is returned when the backend's migrations are not on disk,
// which is the normal case for a standalone checkout of this module — the
// Docker build context, for instance.
var ErrNoMigrations = errors.New("backend/migrations not found")

// MigrationsDir locates backend/migrations by walking up from the working
// directory. Tests run with the working directory set to their own package, so
// a fixed relative path would only ever be right for one of them.
func MigrationsDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("finding the working directory: %w", err)
	}

	for {
		candidate := filepath.Join(dir, "backend", "migrations")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrNoMigrations
		}
		dir = parent
	}
}

func writeTemp(pattern, content string) (path string, cleanup func(), err error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", nil, fmt.Errorf("creating a temporary file: %w", err)
	}
	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", nil, fmt.Errorf("writing %s: %w", f.Name(), err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", nil, fmt.Errorf("closing %s: %w", f.Name(), err)
	}
	return f.Name(), func() { _ = os.Remove(f.Name()) }, nil
}

func terminator(c testcontainers.Container, name string) func() {
	return func() {
		if err := testcontainers.TerminateContainer(c); err != nil {
			log.Printf("terminating the %s container: %v", name, err)
		}
	}
}
