package storage

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalPutAndGet(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocal(dir)
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}

	ctx := context.Background()
	key := "test.txt"
	content := "hello, storage!"

	if err := store.Put(ctx, key, bytes.NewBufferString(content)); err != nil {
		t.Fatalf("Put: %v", err)
	}

	rc, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if string(got) != content {
		t.Errorf("Get returned %q, want %q", string(got), content)
	}
}

func TestLocalExists(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocal(dir)
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}

	ctx := context.Background()
	key := "exists-test.txt"

	exists, err := store.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists (before Put): %v", err)
	}
	if exists {
		t.Error("Exists returned true before Put, want false")
	}

	if err := store.Put(ctx, key, bytes.NewBufferString("data")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	exists, err = store.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists (after Put): %v", err)
	}
	if !exists {
		t.Error("Exists returned false after Put, want true")
	}
}

func TestLocalDelete(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocal(dir)
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}

	ctx := context.Background()
	key := "delete-test.txt"

	if err := store.Put(ctx, key, bytes.NewBufferString("to be deleted")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	exists, err := store.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists (after Delete): %v", err)
	}
	if exists {
		t.Error("Exists returned true after Delete, want false")
	}
}

func TestLocalPutCreatesSubdirectories(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocal(dir)
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}

	ctx := context.Background()
	key := "torrents/123.torrent"
	content := "torrent file data"

	if err := store.Put(ctx, key, bytes.NewBufferString(content)); err != nil {
		t.Fatalf("Put with nested key: %v", err)
	}

	// Verify the file exists at the expected nested path.
	fullPath := filepath.Join(dir, key)
	if _, err := os.Stat(fullPath); err != nil {
		t.Fatalf("file not found at %s: %v", fullPath, err)
	}
}

func TestLocalGetNonExistent(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocal(dir)
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}

	ctx := context.Background()
	_, err = store.Get(ctx, "does-not-exist.txt")
	if err == nil {
		t.Error("Get returned nil error for non-existent key, want error")
	}
}

func TestLocalURL(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocal(dir)
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}

	ctx := context.Background()
	key := "torrents/abc.torrent"

	url, err := store.URL(ctx, key)
	if err != nil {
		t.Fatalf("URL: %v", err)
	}

	expected := "/files/torrents/abc.torrent"
	if url != expected {
		t.Errorf("URL returned %q, want %q", url, expected)
	}
}
