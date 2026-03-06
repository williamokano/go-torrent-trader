package storage

import (
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/config"
)

// New creates a FileStorage implementation based on the provided
// StorageConfig. It returns an error for unknown storage types.
func New(cfg config.StorageConfig) (FileStorage, error) {
	switch cfg.Type {
	case "local":
		return NewLocal(cfg.LocalPath)
	case "s3":
		return NewS3(cfg.S3Endpoint, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Bucket, cfg.S3UseSSL)
	default:
		return nil, fmt.Errorf("unknown storage type: %s", cfg.Type)
	}
}
