// Package database provides a thin wrapper around database/sql for PostgreSQL
// connection management using pgx as the underlying driver.
package database

import (
	"database/sql"
	"fmt"
	"time"

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
