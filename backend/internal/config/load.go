package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Load reads configuration from environment variables, applies defaults,
// and validates required fields. It returns an error if validation fails.
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Host: envOrDefault("SERVER_HOST", "0.0.0.0"),
			Port: 8080,
		},
		Database: DatabaseConfig{
			MaxOpenConns:    25,
			MaxIdleConns:    5,
			ConnMaxLifetime: 5 * time.Minute,
		},
		Redis: RedisConfig{
			URL: envOrDefault("REDIS_URL", "redis://localhost:6379/0"),
		},
		SMTP: SMTPConfig{
			Host: envOrDefault("SMTP_HOST", "localhost"),
			Port: 1025,
			From: envOrDefault("SMTP_FROM", "noreply@torrenttrader.local"),
		},
		Storage: StorageConfig{
			Type:       envOrDefault("STORAGE_TYPE", "local"),
			LocalPath:  envOrDefault("STORAGE_LOCAL_PATH", "./uploads"),
			S3Endpoint: os.Getenv("S3_ENDPOINT"),
			S3AccessKey: os.Getenv("S3_ACCESS_KEY"),
			S3SecretKey: os.Getenv("S3_SECRET_KEY"),
			S3Bucket:   os.Getenv("S3_BUCKET"),
			S3UseSSL:   false,
		},
		Tracker: TrackerConfig{
			AnnounceInterval:    1800,
			MinInterval:         900,
			MaxPeersPerResponse: 50,
		},
		Site: SiteConfig{
			Name:        envOrDefault("SITE_NAME", "TorrentTrader"),
			Description: envOrDefault("SITE_DESCRIPTION", "Private BitTorrent Tracker"),
			BaseURL:     envOrDefault("SITE_BASE_URL", "http://localhost:8080"),
		},
	}

	// Parse integer env vars with defaults.
	var err error

	if v := os.Getenv("SERVER_PORT"); v != "" {
		cfg.Server.Port, err = strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid SERVER_PORT %q: %w", v, err)
		}
	}

	if v := os.Getenv("DB_MAX_OPEN_CONNS"); v != "" {
		cfg.Database.MaxOpenConns, err = strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid DB_MAX_OPEN_CONNS %q: %w", v, err)
		}
	}

	if v := os.Getenv("DB_MAX_IDLE_CONNS"); v != "" {
		cfg.Database.MaxIdleConns, err = strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid DB_MAX_IDLE_CONNS %q: %w", v, err)
		}
	}

	if v := os.Getenv("DB_CONN_MAX_LIFETIME"); v != "" {
		cfg.Database.ConnMaxLifetime, err = time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid DB_CONN_MAX_LIFETIME %q: %w", v, err)
		}
	}

	if v := os.Getenv("SMTP_PORT"); v != "" {
		cfg.SMTP.Port, err = strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid SMTP_PORT %q: %w", v, err)
		}
	}

	if v := os.Getenv("S3_USE_SSL"); v != "" {
		cfg.Storage.S3UseSSL, err = strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("invalid S3_USE_SSL %q: %w", v, err)
		}
	}

	if v := os.Getenv("TRACKER_ANNOUNCE_INTERVAL"); v != "" {
		cfg.Tracker.AnnounceInterval, err = strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid TRACKER_ANNOUNCE_INTERVAL %q: %w", v, err)
		}
	}

	if v := os.Getenv("TRACKER_MIN_INTERVAL"); v != "" {
		cfg.Tracker.MinInterval, err = strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid TRACKER_MIN_INTERVAL %q: %w", v, err)
		}
	}

	if v := os.Getenv("TRACKER_MAX_PEERS"); v != "" {
		cfg.Tracker.MaxPeersPerResponse, err = strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid TRACKER_MAX_PEERS %q: %w", v, err)
		}
	}

	// Required fields.
	cfg.Database.URL = os.Getenv("DATABASE_URL")
	if cfg.Database.URL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required but not set")
	}

	// Validate value ranges.
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return nil, fmt.Errorf("SERVER_PORT must be between 1 and 65535, got %d", cfg.Server.Port)
	}

	if cfg.Database.MaxOpenConns <= 0 {
		return nil, fmt.Errorf("DB_MAX_OPEN_CONNS must be greater than 0, got %d", cfg.Database.MaxOpenConns)
	}

	if cfg.Database.MaxIdleConns <= 0 {
		return nil, fmt.Errorf("DB_MAX_IDLE_CONNS must be greater than 0, got %d", cfg.Database.MaxIdleConns)
	}

	if cfg.Database.ConnMaxLifetime <= 0 {
		return nil, fmt.Errorf("DB_CONN_MAX_LIFETIME must be greater than 0, got %s", cfg.Database.ConnMaxLifetime)
	}

	if cfg.SMTP.Port <= 0 || cfg.SMTP.Port > 65535 {
		return nil, fmt.Errorf("SMTP_PORT must be between 1 and 65535, got %d", cfg.SMTP.Port)
	}

	if cfg.Database.MaxIdleConns > cfg.Database.MaxOpenConns {
		return nil, fmt.Errorf("DB_MAX_IDLE_CONNS (%d) must not exceed DB_MAX_OPEN_CONNS (%d)", cfg.Database.MaxIdleConns, cfg.Database.MaxOpenConns)
	}

	if cfg.Tracker.AnnounceInterval <= 0 {
		return nil, fmt.Errorf("TRACKER_ANNOUNCE_INTERVAL must be greater than 0, got %d", cfg.Tracker.AnnounceInterval)
	}

	if cfg.Tracker.MinInterval <= 0 {
		return nil, fmt.Errorf("TRACKER_MIN_INTERVAL must be greater than 0, got %d", cfg.Tracker.MinInterval)
	}

	if cfg.Tracker.MaxPeersPerResponse <= 0 {
		return nil, fmt.Errorf("TRACKER_MAX_PEERS must be greater than 0, got %d", cfg.Tracker.MaxPeersPerResponse)
	}

	storageType := cfg.Storage.Type
	if storageType != "local" && storageType != "s3" {
		return nil, fmt.Errorf("STORAGE_TYPE must be \"local\" or \"s3\", got %q", storageType)
	}

	if storageType == "s3" {
		if cfg.Storage.S3Endpoint == "" {
			return nil, fmt.Errorf("S3_ENDPOINT is required when STORAGE_TYPE=s3")
		}
		if cfg.Storage.S3AccessKey == "" {
			return nil, fmt.Errorf("S3_ACCESS_KEY is required when STORAGE_TYPE=s3")
		}
		if cfg.Storage.S3SecretKey == "" {
			return nil, fmt.Errorf("S3_SECRET_KEY is required when STORAGE_TYPE=s3")
		}
		if cfg.Storage.S3Bucket == "" {
			return nil, fmt.Errorf("S3_BUCKET is required when STORAGE_TYPE=s3")
		}
	}

	return cfg, nil
}

func envOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
