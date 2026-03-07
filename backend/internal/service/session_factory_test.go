package service

import (
	"testing"
	"time"
)

func TestNewSessionStore_UnknownType(t *testing.T) {
	_, err := NewSessionStore(SessionStoreConfig{
		Type: "badger",
	})
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestNewSessionStore_MemoryTypeRemoved(t *testing.T) {
	_, err := NewSessionStore(SessionStoreConfig{
		Type: "memory",
	})
	if err == nil {
		t.Fatal("expected error for removed memory type")
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
