package database

import (
	"context"
	"testing"
	"time"
)

func TestConnectInvalidDSN(t *testing.T) {
	cfg := ConnConfig{
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: 1 * time.Minute,
	}

	// An unreachable host should fail on Ping.
	_, err := Connect("postgres://invalid:invalid@localhost:1/nonexistent?connect_timeout=1", cfg)
	if err == nil {
		t.Fatal("expected error when connecting with invalid DSN, got nil")
	}
}

func TestDefaultConnConfig(t *testing.T) {
	cfg := DefaultConnConfig()

	if cfg.MaxOpenConns != 25 {
		t.Errorf("expected MaxOpenConns 25, got %d", cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns != 5 {
		t.Errorf("expected MaxIdleConns 5, got %d", cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime != 5*time.Minute {
		t.Errorf("expected ConnMaxLifetime 5m, got %s", cfg.ConnMaxLifetime)
	}
}

func TestConnectPoolInvalidDSN(t *testing.T) {
	if _, err := ConnectPool(context.Background(), "not a url ::", 4); err == nil {
		t.Fatal("expected an error for a malformed DSN, got nil")
	}
}

// pgxpool connects lazily, so without the ping an unreachable database would
// only surface at the first job insert, long after startup reported success.
func TestConnectPoolUnreachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := ConnectPool(ctx, "postgres://invalid:invalid@localhost:1/nonexistent?connect_timeout=1", 4); err == nil {
		t.Fatal("expected an error for an unreachable database, got nil")
	}
}
