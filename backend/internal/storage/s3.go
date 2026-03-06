package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// defaultPresignExpiry is the default expiry for presigned GET URLs.
const defaultPresignExpiry = 1 * time.Hour

// S3Storage implements FileStorage using an S3-compatible object store.
type S3Storage struct {
	client *minio.Client
	bucket string
}

// NewS3 creates an S3Storage backed by the given S3-compatible endpoint.
// It works with MinIO, AWS S3, and any other S3-compatible service.
func NewS3(endpoint, accessKey, secretKey, bucket string, useSSL bool) (*S3Storage, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}

	return &S3Storage{
		client: client,
		bucket: bucket,
	}, nil
}

// Put uploads the contents of reader to the bucket under the given key.
func (s *S3Storage) Put(ctx context.Context, key string, reader io.Reader) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, reader, -1, minio.PutObjectOptions{})
	if err != nil {
		return fmt.Errorf("put object %s: %w", key, err)
	}
	return nil
}

// Get retrieves the object identified by key from the bucket.
func (s *S3Storage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object %s: %w", key, err)
	}
	return obj, nil
}

// Delete removes the object identified by key from the bucket.
func (s *S3Storage) Delete(ctx context.Context, key string) error {
	err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("delete object %s: %w", key, err)
	}
	return nil
}

// Exists reports whether an object with the given key exists in the bucket.
func (s *S3Storage) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		errResp := minio.ToErrorResponse(err)
		if errResp.Code == "NoSuchKey" {
			return false, nil
		}
		return false, fmt.Errorf("stat object %s: %w", key, err)
	}
	return true, nil
}

// URL returns a presigned GET URL for the object, valid for the default
// expiry duration.
func (s *S3Storage) URL(ctx context.Context, key string) (string, error) {
	u, err := s.client.PresignedGetObject(ctx, s.bucket, key, defaultPresignExpiry, nil)
	if err != nil {
		return "", fmt.Errorf("presign URL for %s: %w", key, err)
	}
	return u.String(), nil
}
