package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// Shared fixtures for the repository tests. Each helper inserts the minimum
// row needed to satisfy the FK graph and returns the new ID, so a test can say
// what it needs and nothing more.

// uniq makes a name unique per call, so tests that do not reset the database
// between them cannot collide on a UNIQUE constraint.
var uniqCounter int

func uniq(prefix string) string {
	uniqCounter++
	return fmt.Sprintf("%s%d", prefix, uniqCounter)
}

// anyGroupID returns a seeded group. Groups come from the migrations and are
// preserved by resetTestData.
func anyGroupID(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`SELECT id FROM groups ORDER BY level LIMIT 1`).Scan(&id); err != nil {
		t.Fatalf("no seeded group: %v", err)
	}
	return id
}

// newUser inserts a user through the repository under test, which keeps the
// fixture honest: if Create is broken, every test that needs a user says so.
func newUser(t *testing.T, db *sql.DB) *model.User {
	t.Helper()

	name := uniq("user")
	u := &model.User{
		Username:       name,
		Email:          name + "@example.test",
		PasswordHash:   "hash",
		PasswordScheme: "argon2id",
		GroupID:        anyGroupID(t, db),
		Enabled:        true,
		CanDownload:    true,
		CanUpload:      true,
		CanChat:        true,
		CanForum:       true,
	}
	if err := NewUserRepo(db).Create(context.Background(), u); err != nil {
		t.Fatalf("creating user: %v", err)
	}
	return u
}

func newCategoryID(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	name := uniq("cat")
	var id int64
	err := db.QueryRow(
		`INSERT INTO categories (name, slug) VALUES ($1, $2) RETURNING id`, name, name,
	).Scan(&id)
	if err != nil {
		t.Fatalf("creating category: %v", err)
	}
	return id
}

// newTorrent creates a torrent through TorrentRepo rather than raw SQL: the
// fixture then exercises the same Create the production code calls, so a broken
// insert surfaces immediately instead of being papered over by bespoke SQL.
func newTorrent(t *testing.T, db *sql.DB, uploaderID int64) *model.Torrent {
	t.Helper()

	tor := &model.Torrent{
		Name:       uniq("Release"), // one lexeme: full-text search tokenises on it
		InfoHash:   infoHash(uniq("hash")),
		Size:       1024,
		CategoryID: newCategoryID(t, db),
		UploaderID: uploaderID,
		Visible:    true,
		FileCount:  1,
	}
	if err := NewTorrentRepo(db).Create(context.Background(), tor); err != nil {
		t.Fatalf("creating torrent: %v", err)
	}
	return tor
}

// infoHash pads a seed string to the 20 bytes a BitTorrent info hash occupies.
func infoHash(seed string) []byte {
	h := make([]byte, 20)
	copy(h, seed)
	return h
}

// forumFixture is the set of IDs most forum repository tests need.
type forumFixture struct {
	CategoryID int64
	ForumID    int64
	TopicID    int64
	UserID     int64
}

func newForumFixture(t *testing.T, db *sql.DB) forumFixture {
	t.Helper()

	user := newUser(t, db)

	var catID int64
	if err := db.QueryRow(
		`INSERT INTO forum_categories (name) VALUES ($1) RETURNING id`, uniq("fcat"),
	).Scan(&catID); err != nil {
		t.Fatalf("creating forum category: %v", err)
	}

	var forumID int64
	if err := db.QueryRow(
		`INSERT INTO forums (category_id, name) VALUES ($1, $2) RETURNING id`, catID, uniq("forum"),
	).Scan(&forumID); err != nil {
		t.Fatalf("creating forum: %v", err)
	}

	var topicID int64
	if err := db.QueryRow(
		`INSERT INTO forum_topics (forum_id, user_id, title) VALUES ($1, $2, $3) RETURNING id`,
		forumID, user.ID, uniq("topic"),
	).Scan(&topicID); err != nil {
		t.Fatalf("creating forum topic: %v", err)
	}

	return forumFixture{CategoryID: catID, ForumID: forumID, TopicID: topicID, UserID: user.ID}
}

func ptr[T any](v T) *T { return &v }
