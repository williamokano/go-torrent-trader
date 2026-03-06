package postgres

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

func TestNewUserRepo_ReturnsInterface(t *testing.T) {
	// Verify the constructor returns a value that satisfies the interface.
	var _ repository.UserRepository = NewUserRepo(&sql.DB{})
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
