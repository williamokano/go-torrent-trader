package database

import (
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
