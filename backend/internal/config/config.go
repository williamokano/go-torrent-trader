package config

import "time"

// Config holds all application configuration.
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	Session  SessionConfig
	SMTP     SMTPConfig
	Storage  StorageConfig
	Tracker  TrackerConfig
	Site     SiteConfig
}

// SessionConfig holds session store settings.
type SessionConfig struct {
	Store           string        // SESSION_STORE, default "redis" (memory|redis)
	AccessTokenTTL  time.Duration // ACCESS_TOKEN_TTL, default 1h
	RefreshTokenTTL time.Duration // REFRESH_TOKEN_TTL, default 720h (30 days)
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Host string // SERVER_HOST, default "0.0.0.0"
	Port int    // SERVER_PORT, default 8080
}

// DatabaseConfig holds database connection settings.
type DatabaseConfig struct {
	URL             string        // DATABASE_URL, required
	MaxOpenConns    int           // DB_MAX_OPEN_CONNS, default 25
	MaxIdleConns    int           // DB_MAX_IDLE_CONNS, default 5
	ConnMaxLifetime time.Duration // DB_CONN_MAX_LIFETIME, default 5m
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	URL string // REDIS_URL, default "redis://localhost:6379/0"
}

// SMTPConfig holds email sending settings.
type SMTPConfig struct {
	Host string // SMTP_HOST, default "localhost"
	Port int    // SMTP_PORT, default 1025
	From string // SMTP_FROM, default "noreply@torrenttrader.local"
}

// StorageConfig holds file storage settings.
type StorageConfig struct {
	Type       string // STORAGE_TYPE, default "local" (local|s3)
	LocalPath  string // STORAGE_LOCAL_PATH, default "./uploads"
	S3Endpoint string // S3_ENDPOINT
	S3AccessKey string // S3_ACCESS_KEY
	S3SecretKey string // S3_SECRET_KEY
	S3Bucket   string // S3_BUCKET
	S3UseSSL   bool   // S3_USE_SSL, default false
}

// TrackerConfig holds BitTorrent tracker settings.
type TrackerConfig struct {
	AnnounceInterval    int // TRACKER_ANNOUNCE_INTERVAL, default 1800 (seconds)
	MinInterval         int // TRACKER_MIN_INTERVAL, default 900
	MaxPeersPerResponse int // TRACKER_MAX_PEERS, default 50
}

// SiteConfig holds general site metadata.
type SiteConfig struct {
	Name        string // SITE_NAME, default "TorrentTrader"
	Description string // SITE_DESCRIPTION, default "Private BitTorrent Tracker"
	BaseURL     string // SITE_BASE_URL, default "http://localhost:5173" — website (frontend) URL, used in emails and links
	ApiURL      string // API_URL, default "http://localhost:8080" — backend API URL, used for announce/scrape URLs
}
