// Package database provides a thin wrapper around database/sql for PostgreSQL
// connection management using pgx as the underlying driver.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver
)

// ConnConfig holds connection pool tuning parameters.
type ConnConfig struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// DefaultConnConfig returns sensible defaults for connection pooling.
func DefaultConnConfig() ConnConfig {
	return ConnConfig{
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
	}
}

// Connect opens a PostgreSQL connection using the pgx stdlib driver,
// configures the connection pool, and verifies connectivity with a ping.
func Connect(databaseURL string, cfg ConnConfig) (*sql.DB, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("opening database connection: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	return db, nil
}

// ConnectPool opens a native pgx connection pool and verifies it with a ping.
//
// It exists alongside Connect for libraries that need pgx's own interface rather
// than database/sql — River's driver is the one today. It is a second pool, so
// each process can hold DB_MAX_OPEN_CONNS + maxConns connections. maxConns is
// set explicitly rather than left to pgxpool's default of max(4, NumCPU), which
// on a two-core host leaves River three connections to share with every
// enqueue on the request path. (pool_max_conns in the URL is not an option:
// the same URL feeds database/sql, which would send it to the server as a
// runtime parameter.)
func ConnectPool(ctx context.Context, databaseURL string, maxConns int) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing database URL for the pool: %w", err)
	}
	if maxConns > 0 {
		cfg.MaxConns = int32(min(maxConns, math.MaxInt32)) //nolint:gosec // bounded just above
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("opening database pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database pool: %w", err)
	}
	return pool, nil
}
