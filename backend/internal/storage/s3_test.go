package storage

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"
)

func TestNewS3Constructor(t *testing.T) {
	store, err := NewS3("localhost:9000", "minioadmin", "minioadmin", "test-bucket", false)
	if err != nil {
		t.Fatalf("NewS3: %v", err)
	}
	if store == nil {
		t.Fatal("NewS3 returned nil store")
	}
	if store.bucket != "test-bucket" {
		t.Errorf("bucket = %q, want %q", store.bucket, "test-bucket")
	}
}

func TestNewS3ConstructorWithSSL(t *testing.T) {
	store, err := NewS3("s3.amazonaws.com", "access", "secret", "my-bucket", true)
	if err != nil {
		t.Fatalf("NewS3 with SSL: %v", err)
	}
	if store == nil {
		t.Fatal("NewS3 returned nil store with SSL")
	}
}

func TestS3Integration(t *testing.T) {
	endpoint := os.Getenv("S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("S3_ENDPOINT not set, skipping integration test")
	}

	accessKey := os.Getenv("S3_ACCESS_KEY")
	secretKey := os.Getenv("S3_SECRET_KEY")
	bucket := os.Getenv("S3_BUCKET")
	if bucket == "" {
		bucket = "test-bucket"
	}

	store, err := NewS3(endpoint, accessKey, secretKey, bucket, false)
	if err != nil {
		t.Fatalf("NewS3: %v", err)
	}

	ctx := context.Background()
	key := "integration-test/hello.txt"
	content := "hello from integration test"

	// Put
	if err := store.Put(ctx, key, bytes.NewBufferString(content)); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Exists
	exists, err := store.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !exists {
		t.Error("Exists returned false after Put, want true")
	}

	// Get
	rc, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != content {
		t.Errorf("Get returned %q, want %q", string(got), content)
	}

	// URL
	u, err := store.URL(ctx, key)
	if err != nil {
		t.Fatalf("URL: %v", err)
	}
	if u == "" {
		t.Error("URL returned empty string")
	}

	// Delete
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify deleted
	exists, err = store.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists after Delete: %v", err)
	}
	if exists {
		t.Error("Exists returned true after Delete, want false")
	}
}
