package service

import (
	"testing"
	"time"
)

func TestNewSessionStore_Memory(t *testing.T) {
	store, err := NewSessionStore(SessionStoreConfig{
		Type: "memory",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store == nil {
		t.Fatal("expected non-nil store")
	}
	if _, ok := store.(*MemorySessionStore); !ok {
		t.Errorf("expected *MemorySessionStore, got %T", store)
	}
}

func TestNewSessionStore_UnknownType(t *testing.T) {
	_, err := NewSessionStore(SessionStoreConfig{
		Type: "badger",
	})
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestNewSessionStore_RedisInvalidURL(t *testing.T) {
	_, err := NewSessionStore(SessionStoreConfig{
		Type:            "redis",
		RedisURL:        "not-a-url",
		AccessTokenTTL:  time.Hour,
		RefreshTokenTTL: 720 * time.Hour,
	})
	if err == nil {
		t.Fatal("expected error for invalid Redis URL")
	}
}
