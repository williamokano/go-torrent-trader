package main

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository/postgres"
)

var fixtureCounter int

func uniq(prefix string) string {
	fixtureCounter++
	return fmt.Sprintf("%s%d", prefix, fixtureCounter)
}

func newUser(t *testing.T, db *sql.DB) *model.User {
	t.Helper()
	var groupID int64
	if err := db.QueryRow(`SELECT id FROM groups ORDER BY level LIMIT 1`).Scan(&groupID); err != nil {
		t.Fatalf("no seeded group: %v", err)
	}
	name := uniq("user")
	u := &model.User{
		Username:       name,
		Email:          name + "@example.test",
		PasswordHash:   "hash",
		PasswordScheme: "argon2id",
		GroupID:        groupID,
		Enabled:        true,
		CanDownload:    true,
		CanUpload:      true,
		CanChat:        true,
		CanForum:       true,
	}
	if err := postgres.NewUserRepo(db).Create(context.Background(), u); err != nil {
		t.Fatalf("creating user: %v", err)
	}
	return u
}

// newStaleComment inserts a torrent_comments row the way it would have looked
// before the @mention feature existed: mentioned_usernames still at its
// column default ('[]'), even though body contains a real mention. updated_at
// is captured on return so a test can assert the backfill never touches it.
func newStaleComment(t *testing.T, db *sql.DB, authorID int64, body string) (id int64, updatedAt sql.NullTime) {
	t.Helper()
	var catID, torID int64
	if err := db.QueryRow(`INSERT INTO categories (name, slug) VALUES ($1, $1) RETURNING id`, uniq("cat")).Scan(&catID); err != nil {
		t.Fatalf("creating category: %v", err)
	}
	hash := make([]byte, 20)
	copy(hash, uniq("hash"))
	if err := db.QueryRow(
		`INSERT INTO torrents (name, info_hash, size, category_id, uploader_id, file_count) VALUES ($1,$2,1024,$3,$4,1) RETURNING id`,
		uniq("Release"), hash, catID, authorID,
	).Scan(&torID); err != nil {
		t.Fatalf("creating torrent: %v", err)
	}
	if err := db.QueryRow(
		`INSERT INTO torrent_comments (torrent_id, user_id, body) VALUES ($1,$2,$3) RETURNING id, updated_at`,
		torID, authorID, body,
	).Scan(&id, &updatedAt); err != nil {
		t.Fatalf("creating stale comment: %v", err)
	}
	return id, updatedAt
}

// newStalePost mirrors newStaleComment for forum_posts.
func newStalePost(t *testing.T, db *sql.DB, authorID int64, body string) int64 {
	t.Helper()
	var fcatID, forumID, topicID, postID int64
	if err := db.QueryRow(`INSERT INTO forum_categories (name) VALUES ($1) RETURNING id`, uniq("fcat")).Scan(&fcatID); err != nil {
		t.Fatalf("creating forum category: %v", err)
	}
	if err := db.QueryRow(`INSERT INTO forums (category_id, name) VALUES ($1,$2) RETURNING id`, fcatID, uniq("forum")).Scan(&forumID); err != nil {
		t.Fatalf("creating forum: %v", err)
	}
	if err := db.QueryRow(`INSERT INTO forum_topics (forum_id, user_id, title) VALUES ($1,$2,$3) RETURNING id`, forumID, authorID, uniq("topic")).Scan(&topicID); err != nil {
		t.Fatalf("creating forum topic: %v", err)
	}
	if err := db.QueryRow(`INSERT INTO forum_posts (topic_id, user_id, body) VALUES ($1,$2,$3) RETURNING id`, topicID, authorID, body).Scan(&postID); err != nil {
		t.Fatalf("creating stale post: %v", err)
	}
	return postID
}

func newStaleMessage(t *testing.T, db *sql.DB, senderID, receiverID int64, body string) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(
		`INSERT INTO messages (sender_id, receiver_id, subject, body) VALUES ($1,$2,'subject',$3) RETURNING id`,
		senderID, receiverID, body,
	).Scan(&id); err != nil {
		t.Fatalf("creating stale message: %v", err)
	}
	return id
}

func TestRunBackfill_ResolvesStaleMentionsAcrossAllThreeTablesWithoutTouchingEditMetadata(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()

	author := newUser(t, db)
	receiver := newUser(t, db)
	mentioned := newUser(t, db)

	commentID, originalUpdatedAt := newStaleComment(t, db, author.ID, "hey @"+mentioned.Username)
	postID := newStalePost(t, db, author.ID, "hey @"+mentioned.Username)
	msgID := newStaleMessage(t, db, author.ID, receiver.ID, "hey @"+mentioned.Username)

	stats, err := runBackfill(ctx, db, Options{BatchSize: 500})
	if err != nil {
		t.Fatalf("runBackfill: %v", err)
	}
	if stats.Tables["torrent_comments"].Changed != 1 {
		t.Errorf("torrent_comments changed = %d, want 1", stats.Tables["torrent_comments"].Changed)
	}
	if stats.Tables["forum_posts"].Changed != 1 {
		t.Errorf("forum_posts changed = %d, want 1", stats.Tables["forum_posts"].Changed)
	}
	if stats.Tables["messages"].Changed != 1 {
		t.Errorf("messages changed = %d, want 1", stats.Tables["messages"].Changed)
	}

	var commentMentioned []byte
	var updatedAt sql.NullTime
	if err := db.QueryRow(`SELECT mentioned_usernames, updated_at FROM torrent_comments WHERE id = $1`, commentID).
		Scan(&commentMentioned, &updatedAt); err != nil {
		t.Fatalf("reading back comment: %v", err)
	}
	if got := postgres.ScanMentionedUsernames(commentMentioned); len(got) != 1 || got[0] != mentioned.Username {
		t.Errorf("comment mentioned_usernames = %v, want [%s]", got, mentioned.Username)
	}
	if !updatedAt.Time.Equal(originalUpdatedAt.Time) {
		t.Errorf("comment updated_at changed from %v to %v — backfill must not touch it", originalUpdatedAt.Time, updatedAt.Time)
	}

	var postMentioned []byte
	var editedAt sql.NullTime
	if err := db.QueryRow(`SELECT mentioned_usernames, edited_at FROM forum_posts WHERE id = $1`, postID).
		Scan(&postMentioned, &editedAt); err != nil {
		t.Fatalf("reading back post: %v", err)
	}
	if got := postgres.ScanMentionedUsernames(postMentioned); len(got) != 1 || got[0] != mentioned.Username {
		t.Errorf("post mentioned_usernames = %v, want [%s]", got, mentioned.Username)
	}
	if editedAt.Valid {
		t.Errorf("post edited_at = %v, want NULL — backfill must not mark historical posts as edited", editedAt.Time)
	}

	var msgMentioned []byte
	if err := db.QueryRow(`SELECT mentioned_usernames FROM messages WHERE id = $1`, msgID).Scan(&msgMentioned); err != nil {
		t.Fatalf("reading back message: %v", err)
	}
	if got := postgres.ScanMentionedUsernames(msgMentioned); len(got) != 1 || got[0] != mentioned.Username {
		t.Errorf("message mentioned_usernames = %v, want [%s]", got, mentioned.Username)
	}
}

func TestRunBackfill_DryRunMakesNoWrites(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()

	author := newUser(t, db)
	receiver := newUser(t, db)
	mentioned := newUser(t, db)

	msgID := newStaleMessage(t, db, author.ID, receiver.ID, "hey @"+mentioned.Username)

	stats, err := runBackfill(ctx, db, Options{BatchSize: 500, DryRun: true})
	if err != nil {
		t.Fatalf("runBackfill: %v", err)
	}
	if stats.Tables["messages"].Changed != 1 {
		t.Errorf("dry-run changed count = %d, want 1 (reported, but not written)", stats.Tables["messages"].Changed)
	}

	var msgMentioned []byte
	if err := db.QueryRow(`SELECT mentioned_usernames FROM messages WHERE id = $1`, msgID).Scan(&msgMentioned); err != nil {
		t.Fatalf("reading back message: %v", err)
	}
	if got := postgres.ScanMentionedUsernames(msgMentioned); len(got) != 0 {
		t.Errorf("dry-run must not write: mentioned_usernames = %v, want still empty", got)
	}
}

func TestCheckSchema_PassesOnceMigrated(t *testing.T) {
	db := requireDB(t)
	if err := checkSchema(context.Background(), db); err != nil {
		t.Errorf("checkSchema on a fully migrated database should pass, got: %v", err)
	}
}
