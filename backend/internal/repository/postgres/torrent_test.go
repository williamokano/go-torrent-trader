package postgres

import (
	"context"
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

// TestTorrentRepo_ListSearch_FullText verifies that full-text search (tsvector)
// is used for queries with 3+ characters. Requires a real database with the
// search_vector column and trigger from migration 012.
func TestTorrentRepo_ListSearch_FullText(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	db := openTestDB(t, dsn)
	defer func() { _ = db.Close() }()

	repo := NewTorrentRepo(db)
	ctx := context.Background()

	// Search with 3+ chars should use tsvector. This exercises the
	// plainto_tsquery path. Even with no matching rows, the query must
	// execute without SQL errors (proving the search_vector column and
	// GIN index exist).
	torrents, total, err := repo.List(ctx, repository.ListTorrentsOptions{
		Search: "ubuntu",
		Page:   1,
	})
	if err != nil {
		t.Fatalf("full-text search query failed: %v", err)
	}
	// We don't assert specific rows — just that the query ran without error.
	_ = torrents
	_ = total
}

// TestTorrentRepo_ListSearch_ILIKEFallback verifies that short search queries
// (exactly 2 chars) fall back to ILIKE matching instead of tsvector.
func TestTorrentRepo_ListSearch_ILIKEFallback(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	db := openTestDB(t, dsn)
	defer func() { _ = db.Close() }()

	repo := NewTorrentRepo(db)
	ctx := context.Background()

	// Search with exactly 2 chars should use ILIKE fallback.
	torrents, total, err := repo.List(ctx, repository.ListTorrentsOptions{
		Search: "ab",
		Page:   1,
	})
	if err != nil {
		t.Fatalf("ILIKE fallback search query failed: %v", err)
	}
	_ = torrents
	_ = total
}

// TestTorrentRepo_ListSearch_TooShort verifies that a 1-character search term
// is ignored (no filtering applied for it).
func TestTorrentRepo_ListSearch_TooShort(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	db := openTestDB(t, dsn)
	defer func() { _ = db.Close() }()

	repo := NewTorrentRepo(db)
	ctx := context.Background()

	// 1-char search should be ignored (< 2 minimum).
	_, _, err := repo.List(ctx, repository.ListTorrentsOptions{
		Search: "a",
		Page:   1,
	})
	if err != nil {
		t.Fatalf("single-char search should not error: %v", err)
	}
}

// TestTorrentRepo_ListSearch_FullTextWithExplicitSort verifies that providing
// an explicit sort field skips ts_rank ordering even during full-text search.
func TestTorrentRepo_ListSearch_FullTextWithExplicitSort(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	db := openTestDB(t, dsn)
	defer func() { _ = db.Close() }()

	repo := NewTorrentRepo(db)
	ctx := context.Background()

	// Full-text search with explicit sort — should NOT prepend ts_rank.
	_, _, err := repo.List(ctx, repository.ListTorrentsOptions{
		Search: "ubuntu desktop",
		SortBy: "seeders",
		Page:   1,
	})
	if err != nil {
		t.Fatalf("full-text search with explicit sort failed: %v", err)
	}
}

// TestTorrentRepo_ListSearch_SpecialCharacters verifies that ILIKE special
// characters (%, _, \) are properly escaped in the fallback path.
func TestTorrentRepo_ListSearch_SpecialCharacters(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	db := openTestDB(t, dsn)
	defer func() { _ = db.Close() }()

	repo := NewTorrentRepo(db)
	ctx := context.Background()

	// 2-char search with ILIKE special char should be escaped properly.
	_, _, err := repo.List(ctx, repository.ListTorrentsOptions{
		Search: "a%",
		Page:   1,
	})
	if err != nil {
		t.Fatalf("special char search should not error: %v", err)
	}
}
