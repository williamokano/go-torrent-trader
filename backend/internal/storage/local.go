package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LocalStorage implements FileStorage using the local filesystem.
type LocalStorage struct {
	basePath string
}

// NewLocal creates a LocalStorage that stores files under basePath.
// The base directory is created if it does not already exist.
func NewLocal(basePath string) (*LocalStorage, error) {
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return nil, fmt.Errorf("create base path %s: %w", basePath, err)
	}
	return &LocalStorage{basePath: basePath}, nil
}

// Put writes the contents of reader to basePath/key, creating any
// intermediate directories as needed.
func (l *LocalStorage) Put(_ context.Context, key string, reader io.Reader) error {
	full := filepath.Join(l.basePath, key)

	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return fmt.Errorf("create directory for key %s: %w", key, err)
	}

	f, err := os.Create(full)
	if err != nil {
		return fmt.Errorf("create file %s: %w", key, err)
	}

	if _, err := io.Copy(f, reader); err != nil {
		_ = f.Close()
		return fmt.Errorf("write file %s: %w", key, err)
	}

	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("sync file %s: %w", key, err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("close file %s: %w", key, err)
	}

	return nil
}

// Get opens the file at basePath/key and returns it as a ReadCloser.
func (l *LocalStorage) Get(_ context.Context, key string) (io.ReadCloser, error) {
	full := filepath.Join(l.basePath, key)

	f, err := os.Open(full)
	if err != nil {
		return nil, fmt.Errorf("open file %s: %w", key, err)
	}

	return f, nil
}

// Delete removes the file at basePath/key.
func (l *LocalStorage) Delete(_ context.Context, key string) error {
	full := filepath.Join(l.basePath, key)

	if err := os.Remove(full); err != nil {
		return fmt.Errorf("delete file %s: %w", key, err)
	}

	return nil
}

// Exists reports whether the file at basePath/key exists.
func (l *LocalStorage) Exists(_ context.Context, key string) (bool, error) {
	full := filepath.Join(l.basePath, key)

	_, err := os.Stat(full)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("stat file %s: %w", key, err)
}

// URL returns a relative URL for serving the file via HTTP.
func (l *LocalStorage) URL(_ context.Context, key string) (string, error) {
	return fmt.Sprintf("/files/%s", key), nil
}
