package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// These exercise the repository SQL against a real Postgres. That matters here
// specifically: every bug below is one a query-string mock would have waved
// through, because the SQL was syntactically fine and simply returned the wrong
// rows.

// --- fixtures ---------------------------------------------------------------

func seedUser(t *testing.T, db *sql.DB, username string) int64 {
	t.Helper()

	var groupID int64
	if err := db.QueryRow(`SELECT id FROM groups ORDER BY id LIMIT 1`).Scan(&groupID); err != nil {
		t.Fatalf("no seeded group available: %v", err)
	}

	var id int64
	err := db.QueryRow(`
		INSERT INTO users (username, email, password_hash, group_id)
		VALUES ($1, $2, 'x', $3) RETURNING id`,
		username, username+"@example.test", groupID,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seeding user %q: %v", username, err)
	}
	return id
}

func seedForum(t *testing.T, db *sql.DB) int64 {
	t.Helper()

	var catID int64
	err := db.QueryRow(
		`INSERT INTO forum_categories (name) VALUES ('Test Category') RETURNING id`,
	).Scan(&catID)
	if err != nil {
		t.Fatalf("seeding forum category: %v", err)
	}

	var id int64
	err = db.QueryRow(
		`INSERT INTO forums (category_id, name) VALUES ($1, 'Test Forum') RETURNING id`, catID,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seeding forum: %v", err)
	}
	return id
}

func seedTopic(t *testing.T, db *sql.DB, forumID, userID int64) int64 {
	t.Helper()

	var id int64
	err := db.QueryRow(
		`INSERT INTO forum_topics (forum_id, user_id, title) VALUES ($1, $2, 'Test Topic') RETURNING id`,
		forumID, userID,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seeding topic: %v", err)
	}
	return id
}

// seedPost inserts a post with an explicit created_at so ordering can be
// controlled — including the tie that the id tiebreaker exists to resolve.
func seedPost(t *testing.T, db *sql.DB, topicID, userID int64, body string, createdAt time.Time) int64 {
	t.Helper()

	var id int64
	err := db.QueryRow(
		`INSERT INTO forum_posts (topic_id, user_id, body, created_at) VALUES ($1, $2, $3, $4) RETURNING id`,
		topicID, userID, body, createdAt,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seeding post %q: %v", body, err)
	}
	return id
}

// --- tests ------------------------------------------------------------------

// BE-9.11 shipped an id tiebreaker on ListByTopic because forum search derives
// a deep-link page from an id-rank. This proves the two orderings agree against
// a real planner: with identical created_at values, Postgres is free to return
// posts in any order, and only the tiebreaker pins it down.
func TestListByTopicOrdersDeterministicallyOnCreatedAtTie(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	userID := seedUser(t, db, "alice")
	forumID := seedForum(t, db)
	topicID := seedTopic(t, db, forumID, userID)

	// Same instant for all three: without the tiebreaker the order is undefined.
	at := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	first := seedPost(t, db, topicID, userID, "first", at)
	second := seedPost(t, db, topicID, userID, "second", at)
	third := seedPost(t, db, topicID, userID, "third", at)

	repo := NewForumPostRepo(db)

	// Repeat: a single pass could match by luck.
	for i := 0; i < 5; i++ {
		posts, total, err := repo.ListByTopic(ctx, topicID, 1, 25)
		if err != nil {
			t.Fatalf("ListByTopic: %v", err)
		}
		if total != 3 {
			t.Fatalf("total = %d, want 3", total)
		}
		got := []int64{posts[0].ID, posts[1].ID, posts[2].ID}
		want := []int64{first, second, third}
		for j := range want {
			if got[j] != want[j] {
				t.Fatalf("pass %d: post order = %v, want %v (ascending id on created_at tie)", i, got, want)
			}
		}
	}
}

// The page a search result deep-links to is ceil(post_number/25), so
// post_number must be the post's real rank within its topic — the same order
// ListByTopic paginates in. If these disagree, the link lands on the wrong page.
func TestSearchPostNumberMatchesListByTopicRank(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	userID := seedUser(t, db, "bob")
	forumID := seedForum(t, db)
	topicID := seedTopic(t, db, forumID, userID)

	base := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		seedPost(t, db, topicID, userID, "filler post", base.Add(time.Duration(i)*time.Minute))
	}
	// The post we search for is the 6th in the topic.
	seedPost(t, db, topicID, userID, "distinctive elephant", base.Add(5*time.Minute))

	repo := NewForumPostRepo(db)

	results, total, err := repo.Search(ctx, BuildPrefixQuery("elephant"), nil, 99, 1, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if total != 1 {
		t.Fatalf("total = %d, want 1 matching post", total)
	}
	if got := results[0].PostNumber; got != 6 {
		t.Errorf("PostNumber = %d, want 6 (its rank within the topic)", got)
	}

	// Cross-check the rank against the order ListByTopic actually returns.
	posts, _, err := repo.ListByTopic(ctx, topicID, 1, 25)
	if err != nil {
		t.Fatalf("ListByTopic: %v", err)
	}
	rank := 0
	for i, p := range posts {
		if p.ID == results[0].PostID {
			rank = i + 1
		}
	}
	if rank != results[0].PostNumber {
		t.Errorf("PostNumber %d disagrees with the ListByTopic rank %d — the deep link would open the wrong page",
			results[0].PostNumber, rank)
	}
}

// Soft-deleted posts stay visible as tombstones, so ListByTopic's total must
// count them; but they must NOT inflate the author's post count.
func TestListByTopicCountsTombstonesButNotInUserPostCount(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	userID := seedUser(t, db, "carol")
	modID := seedUser(t, db, "mod")
	forumID := seedForum(t, db)
	topicID := seedTopic(t, db, forumID, userID)

	base := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	seedPost(t, db, topicID, userID, "kept", base)
	doomed := seedPost(t, db, topicID, userID, "doomed", base.Add(time.Minute))

	repo := NewForumPostRepo(db)
	if err := repo.SoftDelete(ctx, doomed, modID); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}

	posts, total, err := repo.ListByTopic(ctx, topicID, 1, 25)
	if err != nil {
		t.Fatalf("ListByTopic: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2 — the tombstone still occupies a slot", total)
	}
	if len(posts) != 2 {
		t.Fatalf("len(posts) = %d, want 2", len(posts))
	}
	if posts[1].DeletedAt == nil {
		t.Error("expected the soft-deleted post to come back as a tombstone")
	}
	// One live post remains, so the author's count must be 1, not 2.
	if got := posts[0].UserPostCount; got != 1 {
		t.Errorf("UserPostCount = %d, want 1 — soft-deleted posts must not inflate it", got)
	}
}

// RecalculateLastPost was collapsed to a single statement whose scalar subquery
// yields NULL when nothing qualifies. Two behaviours are load-bearing: it must
// ignore soft-deleted posts, and it must NULL the field when none remain.
func TestForumRecalculateLastPostIgnoresDeletedAndNullsWhenEmpty(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	userID := seedUser(t, db, "dave")
	modID := seedUser(t, db, "dmod")
	forumID := seedForum(t, db)
	topicID := seedTopic(t, db, forumID, userID)

	base := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	kept := seedPost(t, db, topicID, userID, "kept", base)
	newest := seedPost(t, db, topicID, userID, "newest", base.Add(time.Hour))

	postRepo := NewForumPostRepo(db)
	forumRepo := NewForumRepo(db)

	// Newest post is soft-deleted: last_post_id must fall back to the older live one.
	if err := postRepo.SoftDelete(ctx, newest, modID); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}
	if err := forumRepo.RecalculateLastPost(ctx, forumID); err != nil {
		t.Fatalf("RecalculateLastPost: %v", err)
	}

	var lastPostID *int64
	if err := db.QueryRow(`SELECT last_post_id FROM forums WHERE id = $1`, forumID).Scan(&lastPostID); err != nil {
		t.Fatalf("reading last_post_id: %v", err)
	}
	if lastPostID == nil || *lastPostID != kept {
		t.Errorf("last_post_id = %v, want %d — a soft-deleted post must not be the last post", lastPostID, kept)
	}

	// Delete the remaining post: last_post_id must become NULL, not stay stale.
	if err := postRepo.SoftDelete(ctx, kept, modID); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}
	if err := forumRepo.RecalculateLastPost(ctx, forumID); err != nil {
		t.Fatalf("RecalculateLastPost: %v", err)
	}
	if err := db.QueryRow(`SELECT last_post_id FROM forums WHERE id = $1`, forumID).Scan(&lastPostID); err != nil {
		t.Fatalf("reading last_post_id: %v", err)
	}
	if lastPostID != nil {
		t.Errorf("last_post_id = %d, want NULL once every post is deleted", *lastPostID)
	}
}

// The same NULL-when-empty behaviour on topics, which sets two fields at once.
func TestTopicRecalculateLastPostNullsBothFieldsWhenEmpty(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	userID := seedUser(t, db, "erin")
	modID := seedUser(t, db, "emod")
	forumID := seedForum(t, db)
	topicID := seedTopic(t, db, forumID, userID)

	base := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	only := seedPost(t, db, topicID, userID, "only", base)

	postRepo := NewForumPostRepo(db)
	topicRepo := NewForumTopicRepo(db)

	if err := postRepo.SoftDelete(ctx, only, modID); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}
	if err := topicRepo.RecalculateLastPost(ctx, topicID); err != nil {
		t.Fatalf("RecalculateLastPost: %v", err)
	}

	var lastPostID *int64
	var lastPostAt *time.Time
	err := db.QueryRow(
		`SELECT last_post_id, last_post_at FROM forum_topics WHERE id = $1`, topicID,
	).Scan(&lastPostID, &lastPostAt)
	if err != nil {
		t.Fatalf("reading topic last_post fields: %v", err)
	}
	if lastPostID != nil || lastPostAt != nil {
		t.Errorf("last_post_id=%v last_post_at=%v, want both NULL when no live posts remain", lastPostID, lastPostAt)
	}
}

// GetFirstPostIDByTopic backs the "can't delete the opening post" rule, so it
// must return the opening post even after that post is soft-deleted — otherwise
// the rule would lapse the moment a moderator removed it.
func TestGetFirstPostIDByTopicIgnoresDeletion(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	userID := seedUser(t, db, "frank")
	modID := seedUser(t, db, "fmod")
	forumID := seedForum(t, db)
	topicID := seedTopic(t, db, forumID, userID)

	base := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	opening := seedPost(t, db, topicID, userID, "opening", base)
	seedPost(t, db, topicID, userID, "reply", base.Add(time.Minute))

	repo := NewForumPostRepo(db)

	got, err := repo.GetFirstPostIDByTopic(ctx, topicID)
	if err != nil {
		t.Fatalf("GetFirstPostIDByTopic: %v", err)
	}
	if got != opening {
		t.Fatalf("first post = %d, want %d", got, opening)
	}

	if err := repo.SoftDelete(ctx, opening, modID); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}
	got, err = repo.GetFirstPostIDByTopic(ctx, topicID)
	if err != nil {
		t.Fatalf("GetFirstPostIDByTopic after delete: %v", err)
	}
	if got != opening {
		t.Errorf("first post = %d after soft-delete, want %d — deletion must not change which post opened the topic", got, opening)
	}
}

// The edit-history join must survive the editing user being deleted. Migration
// 049 made edited_by nullable precisely so ON DELETE SET NULL could fire; a JOIN
// (rather than a LEFT JOIN) would silently drop those rows from the history.
func TestListEditsSurvivesDeletedEditor(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	authorID := seedUser(t, db, "gina")
	editorID := seedUser(t, db, "editor")
	forumID := seedForum(t, db)
	topicID := seedTopic(t, db, forumID, authorID)

	base := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	postID := seedPost(t, db, topicID, authorID, "original", base)

	repo := NewForumPostRepo(db)
	edit := &model.ForumPostEdit{
		PostID:   postID,
		EditedBy: &editorID,
		OldBody:  "original",
		NewBody:  "edited",
	}
	if err := repo.CreateEdit(ctx, edit); err != nil {
		t.Fatalf("CreateEdit: %v", err)
	}

	// Deleting the editor must not fail, and must not erase the history row.
	if _, err := db.Exec(`DELETE FROM users WHERE id = $1`, editorID); err != nil {
		t.Fatalf("deleting editor: %v — ON DELETE SET NULL should allow this", err)
	}

	edits, err := repo.ListEdits(ctx, postID)
	if err != nil {
		t.Fatalf("ListEdits: %v", err)
	}
	if len(edits) != 1 {
		t.Fatalf("len(edits) = %d, want 1 — the history row must survive its editor", len(edits))
	}
	if edits[0].EditedBy != nil {
		t.Errorf("EditedBy = %v, want NULL after the editor was deleted", *edits[0].EditedBy)
	}
}
