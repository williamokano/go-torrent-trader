package worker

import (
	"log/slog"
	"testing"
)

func TestNewClientValidURL(t *testing.T) {
	client, err := NewClient("redis://localhost:6379/0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	_ = client.Close()
}

func TestNewClientInvalidURL(t *testing.T) {
	_, err := NewClient("not-a-valid-url")
	if err == nil {
		t.Fatal("expected error for invalid Redis URL")
	}
}

func TestNewServerValidURL(t *testing.T) {
	srv, err := NewServer("redis://localhost:6379/0", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestNewServerInvalidURL(t *testing.T) {
	_, err := NewServer("not-a-valid-url", 10)
	if err == nil {
		t.Fatal("expected error for invalid Redis URL")
	}
}

func TestSlogAdapterDoesNotPanic(t *testing.T) {
	adapter := newSlogAdapter(slog.Default())
	adapter.Debug("test debug")
	adapter.Info("test info")
	adapter.Warn("test warn")
	adapter.Error("test error")
	// Fatal is not tested here because it calls os.Exit(1).
}
