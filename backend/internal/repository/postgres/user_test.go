package postgres

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// Compile-time interface conformance check.
var _ repository.UserRepository = (*UserRepo)(nil)

func TestNewUserRepo_ReturnsNonNil(t *testing.T) {
	repo := NewUserRepo(&sql.DB{})
	if repo == nil {
		t.Fatal("expected non-nil repo")
	}
}

func TestUserRepo_Integration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	db := openTestDB(t, dsn)
	defer func() { _ = db.Close() }()

	_ = NewUserRepo(db)
	// Integration tests against real DB would go here.
}
