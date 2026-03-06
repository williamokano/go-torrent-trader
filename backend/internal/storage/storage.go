package storage

import (
	"context"
	"io"
)

// FileStorage defines the interface for file storage operations.
// Implementations can store files locally on disk or in S3-compatible
// object storage (MinIO, AWS S3, etc.).
type FileStorage interface {
	// Put stores the contents of reader under the given key.
	Put(ctx context.Context, key string, reader io.Reader) error

	// Get retrieves the file identified by key. The caller is responsible
	// for closing the returned ReadCloser.
	Get(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete removes the file identified by key.
	Delete(ctx context.Context, key string) error

	// Exists reports whether a file with the given key exists.
	Exists(ctx context.Context, key string) (bool, error)

	// URL returns a URL that can be used to access the file.
	// For local storage this is a relative path; for S3 it may be a
	// presigned URL.
	URL(ctx context.Context, key string) (string, error)
}
