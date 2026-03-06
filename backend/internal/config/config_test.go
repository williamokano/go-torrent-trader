package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost/test")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Server defaults.
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected Server.Host 0.0.0.0, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected Server.Port 8080, got %d", cfg.Server.Port)
	}

	// Database defaults.
	if cfg.Database.URL != "postgres://test:test@localhost/test" {
		t.Errorf("expected Database.URL from env, got %s", cfg.Database.URL)
	}
	if cfg.Database.MaxOpenConns != 25 {
		t.Errorf("expected Database.MaxOpenConns 25, got %d", cfg.Database.MaxOpenConns)
	}
	if cfg.Database.MaxIdleConns != 5 {
		t.Errorf("expected Database.MaxIdleConns 5, got %d", cfg.Database.MaxIdleConns)
	}
	if cfg.Database.ConnMaxLifetime != 5*time.Minute {
		t.Errorf("expected Database.ConnMaxLifetime 5m, got %s", cfg.Database.ConnMaxLifetime)
	}

	// Redis defaults.
	if cfg.Redis.URL != "redis://localhost:6379/0" {
		t.Errorf("expected Redis.URL redis://localhost:6379/0, got %s", cfg.Redis.URL)
	}

	// SMTP defaults.
	if cfg.SMTP.Host != "localhost" {
		t.Errorf("expected SMTP.Host localhost, got %s", cfg.SMTP.Host)
	}
	if cfg.SMTP.Port != 1025 {
		t.Errorf("expected SMTP.Port 1025, got %d", cfg.SMTP.Port)
	}
	if cfg.SMTP.From != "noreply@torrenttrader.local" {
		t.Errorf("expected SMTP.From noreply@torrenttrader.local, got %s", cfg.SMTP.From)
	}

	// Storage defaults.
	if cfg.Storage.Type != "local" {
		t.Errorf("expected Storage.Type local, got %s", cfg.Storage.Type)
	}
	if cfg.Storage.LocalPath != "./uploads" {
		t.Errorf("expected Storage.LocalPath ./uploads, got %s", cfg.Storage.LocalPath)
	}
	if cfg.Storage.S3UseSSL != false {
		t.Errorf("expected Storage.S3UseSSL false, got %v", cfg.Storage.S3UseSSL)
	}

	// Tracker defaults.
	if cfg.Tracker.AnnounceInterval != 1800 {
		t.Errorf("expected Tracker.AnnounceInterval 1800, got %d", cfg.Tracker.AnnounceInterval)
	}
	if cfg.Tracker.MinInterval != 900 {
		t.Errorf("expected Tracker.MinInterval 900, got %d", cfg.Tracker.MinInterval)
	}
	if cfg.Tracker.MaxPeersPerResponse != 50 {
		t.Errorf("expected Tracker.MaxPeersPerResponse 50, got %d", cfg.Tracker.MaxPeersPerResponse)
	}

	// Site defaults.
	if cfg.Site.Name != "TorrentTrader" {
		t.Errorf("expected Site.Name TorrentTrader, got %s", cfg.Site.Name)
	}
	if cfg.Site.Description != "Private BitTorrent Tracker" {
		t.Errorf("expected Site.Description Private BitTorrent Tracker, got %s", cfg.Site.Description)
	}
	if cfg.Site.BaseURL != "http://localhost:5173" {
		t.Errorf("expected Site.BaseURL http://localhost:5173, got %s", cfg.Site.BaseURL)
	}
	if cfg.Site.ApiURL != "http://localhost:8080" {
		t.Errorf("expected Site.ApiURL http://localhost:8080, got %s", cfg.Site.ApiURL)
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	// DATABASE_URL not set — should fail.
	_, err := Load()
	if err == nil {
		t.Fatal("expected error when DATABASE_URL is not set")
	}
	if err.Error() != "DATABASE_URL is required but not set" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestLoadInvalidServerPort(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost/test")
	t.Setenv("SERVER_PORT", "not-a-number")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid SERVER_PORT")
	}
}

func TestLoadNegativeServerPort(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost/test")
	t.Setenv("SERVER_PORT", "-1")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for negative SERVER_PORT")
	}
}

func TestLoadZeroServerPort(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost/test")
	t.Setenv("SERVER_PORT", "0")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for zero SERVER_PORT")
	}
}

func TestLoadAllEnvVarsSet(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@db:5432/mydb")
	t.Setenv("SERVER_HOST", "127.0.0.1")
	t.Setenv("SERVER_PORT", "9090")
	t.Setenv("DB_MAX_OPEN_CONNS", "50")
	t.Setenv("DB_MAX_IDLE_CONNS", "10")
	t.Setenv("DB_CONN_MAX_LIFETIME", "10m")
	t.Setenv("REDIS_URL", "redis://redis:6379/1")
	t.Setenv("SMTP_HOST", "mail.example.com")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_FROM", "admin@example.com")
	t.Setenv("STORAGE_TYPE", "s3")
	t.Setenv("STORAGE_LOCAL_PATH", "/data/uploads")
	t.Setenv("S3_ENDPOINT", "s3.amazonaws.com")
	t.Setenv("S3_ACCESS_KEY", "AKID")
	t.Setenv("S3_SECRET_KEY", "secret")
	t.Setenv("S3_BUCKET", "mybucket")
	t.Setenv("S3_USE_SSL", "true")
	t.Setenv("TRACKER_ANNOUNCE_INTERVAL", "3600")
	t.Setenv("TRACKER_MIN_INTERVAL", "1800")
	t.Setenv("TRACKER_MAX_PEERS", "100")
	t.Setenv("SITE_NAME", "MySite")
	t.Setenv("SITE_DESCRIPTION", "A cool tracker")
	t.Setenv("SITE_BASE_URL", "https://example.com")
	t.Setenv("API_URL", "https://api.example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected Server.Host 127.0.0.1, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("expected Server.Port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Database.URL != "postgres://user:pass@db:5432/mydb" {
		t.Errorf("expected Database.URL from env, got %s", cfg.Database.URL)
	}
	if cfg.Database.MaxOpenConns != 50 {
		t.Errorf("expected Database.MaxOpenConns 50, got %d", cfg.Database.MaxOpenConns)
	}
	if cfg.Database.MaxIdleConns != 10 {
		t.Errorf("expected Database.MaxIdleConns 10, got %d", cfg.Database.MaxIdleConns)
	}
	if cfg.Database.ConnMaxLifetime != 10*time.Minute {
		t.Errorf("expected Database.ConnMaxLifetime 10m, got %s", cfg.Database.ConnMaxLifetime)
	}
	if cfg.Redis.URL != "redis://redis:6379/1" {
		t.Errorf("expected Redis.URL redis://redis:6379/1, got %s", cfg.Redis.URL)
	}
	if cfg.SMTP.Host != "mail.example.com" {
		t.Errorf("expected SMTP.Host mail.example.com, got %s", cfg.SMTP.Host)
	}
	if cfg.SMTP.Port != 587 {
		t.Errorf("expected SMTP.Port 587, got %d", cfg.SMTP.Port)
	}
	if cfg.SMTP.From != "admin@example.com" {
		t.Errorf("expected SMTP.From admin@example.com, got %s", cfg.SMTP.From)
	}
	if cfg.Storage.Type != "s3" {
		t.Errorf("expected Storage.Type s3, got %s", cfg.Storage.Type)
	}
	if cfg.Storage.LocalPath != "/data/uploads" {
		t.Errorf("expected Storage.LocalPath /data/uploads, got %s", cfg.Storage.LocalPath)
	}
	if cfg.Storage.S3Endpoint != "s3.amazonaws.com" {
		t.Errorf("expected Storage.S3Endpoint s3.amazonaws.com, got %s", cfg.Storage.S3Endpoint)
	}
	if cfg.Storage.S3AccessKey != "AKID" {
		t.Errorf("expected Storage.S3AccessKey AKID, got %s", cfg.Storage.S3AccessKey)
	}
	if cfg.Storage.S3SecretKey != "secret" {
		t.Errorf("expected Storage.S3SecretKey secret, got %s", cfg.Storage.S3SecretKey)
	}
	if cfg.Storage.S3Bucket != "mybucket" {
		t.Errorf("expected Storage.S3Bucket mybucket, got %s", cfg.Storage.S3Bucket)
	}
	if cfg.Storage.S3UseSSL != true {
		t.Errorf("expected Storage.S3UseSSL true, got %v", cfg.Storage.S3UseSSL)
	}
	if cfg.Tracker.AnnounceInterval != 3600 {
		t.Errorf("expected Tracker.AnnounceInterval 3600, got %d", cfg.Tracker.AnnounceInterval)
	}
	if cfg.Tracker.MinInterval != 1800 {
		t.Errorf("expected Tracker.MinInterval 1800, got %d", cfg.Tracker.MinInterval)
	}
	if cfg.Tracker.MaxPeersPerResponse != 100 {
		t.Errorf("expected Tracker.MaxPeersPerResponse 100, got %d", cfg.Tracker.MaxPeersPerResponse)
	}
	if cfg.Site.Name != "MySite" {
		t.Errorf("expected Site.Name MySite, got %s", cfg.Site.Name)
	}
	if cfg.Site.Description != "A cool tracker" {
		t.Errorf("expected Site.Description A cool tracker, got %s", cfg.Site.Description)
	}
	if cfg.Site.BaseURL != "https://example.com" {
		t.Errorf("expected Site.BaseURL https://example.com, got %s", cfg.Site.BaseURL)
	}
	if cfg.Site.ApiURL != "https://api.example.com" {
		t.Errorf("expected Site.ApiURL https://api.example.com, got %s", cfg.Site.ApiURL)
	}
}

func TestLoadInvalidStorageType(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost/test")
	t.Setenv("STORAGE_TYPE", "gcs")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid STORAGE_TYPE")
	}
}

func TestLoadInvalidDBMaxOpenConns(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost/test")
	t.Setenv("DB_MAX_OPEN_CONNS", "0")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for zero DB_MAX_OPEN_CONNS")
	}
}

func TestLoadInvalidDBMaxIdleConns(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost/test")
	t.Setenv("DB_MAX_IDLE_CONNS", "-1")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for negative DB_MAX_IDLE_CONNS")
	}
}

func TestLoadInvalidConnMaxLifetime(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost/test")
	t.Setenv("DB_CONN_MAX_LIFETIME", "not-a-duration")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid DB_CONN_MAX_LIFETIME")
	}
}

func TestLoadInvalidSMTPPort(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost/test")
	t.Setenv("SMTP_PORT", "abc")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid SMTP_PORT")
	}
}

func TestLoadInvalidTrackerAnnounceInterval(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost/test")
	t.Setenv("TRACKER_ANNOUNCE_INTERVAL", "0")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for zero TRACKER_ANNOUNCE_INTERVAL")
	}
}

func TestLoadServerPortTooHigh(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost/test")
	t.Setenv("SERVER_PORT", "99999")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for SERVER_PORT > 65535")
	}
}

func TestLoadMaxIdleConnsExceedsMaxOpenConns(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost/test")
	t.Setenv("DB_MAX_IDLE_CONNS", "50")
	t.Setenv("DB_MAX_OPEN_CONNS", "10")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when MaxIdleConns > MaxOpenConns")
	}
}

func TestLoadS3RequiresCredentials(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost/test")
	t.Setenv("STORAGE_TYPE", "s3")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when STORAGE_TYPE=s3 without S3 credentials")
	}
}

func TestLoadS3WithAllCredentials(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost/test")
	t.Setenv("STORAGE_TYPE", "s3")
	t.Setenv("S3_ENDPOINT", "localhost:9000")
	t.Setenv("S3_ACCESS_KEY", "key")
	t.Setenv("S3_SECRET_KEY", "secret")
	t.Setenv("S3_BUCKET", "bucket")

	_, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadInvalidS3UseSSL(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test:test@localhost/test")
	t.Setenv("S3_USE_SSL", "notbool")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid S3_USE_SSL")
	}
}
