package postgres

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

var _ repository.TorrentRepository = (*TorrentRepo)(nil)

func TestNewTorrentRepo_ReturnsNonNil(t *testing.T) {
	repo := NewTorrentRepo(&sql.DB{})
	if repo == nil {
		t.Fatal("expected non-nil repo")
	}
}

func TestTorrentRepo_Integration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	db := openTestDB(t, dsn)
	defer func() { _ = db.Close() }()

	_ = NewTorrentRepo(db)
}
