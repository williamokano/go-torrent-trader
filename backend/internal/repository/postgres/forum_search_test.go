package postgres

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

// newForumWithLevel creates a forum gated at minGroupLevel inside its own
// category, plus a topic in it, and returns the forum and topic IDs.
func newForumWithLevel(t *testing.T, db *sql.DB, userID int64, minGroupLevel int) (forumID, topicID int64) {
	t.Helper()

	var catID int64
	if err := db.QueryRow(
		`INSERT INTO forum_categories (name) VALUES ($1) RETURNING id`, uniq("fcat"),
	).Scan(&catID); err != nil {
		t.Fatalf("creating forum category: %v", err)
	}
	if err := db.QueryRow(
		`INSERT INTO forums (category_id, name, min_group_level) VALUES ($1, $2, $3) RETURNING id`,
		catID, uniq("forum"), minGroupLevel,
	).Scan(&forumID); err != nil {
		t.Fatalf("creating forum: %v", err)
	}
	if err := db.QueryRow(
		`INSERT INTO forum_topics (forum_id, user_id, title) VALUES ($1, $2, $3) RETURNING id`,
		forumID, userID, uniq("topic"),
	).Scan(&topicID); err != nil {
		t.Fatalf("creating forum topic: %v", err)
	}
	return forumID, topicID
}

func TestForumPostSearchEmptyQueryReturnsNothing(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewForumPostRepo(db)
	f := newForumFixture(t, db)
	newPost(t, db, f.TopicID, f.UserID, "pineapple casserole")

	// BuildPrefixQuery yields "" for input with no usable lexemes. Search must
	// short-circuit rather than hand an empty tsquery to Postgres, which would
	// error. Both a blank string and pure punctuation take that path.
	for _, q := range []string{"", "   ", "!!!"} {
		results, total, err := repo.Search(ctx, q, nil, 100, 1, 25)
		if err != nil {
			t.Fatalf("Search(%q): %v", q, err)
		}
		if results != nil || total != 0 {
			t.Errorf("Search(%q) = %v (total %d), want nil/0", q, results, total)
		}
	}
}

func TestForumPostSearchMatchesBodyAndFillsSnippet(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewForumPostRepo(db)
	f := newForumFixture(t, db)
	want := newPost(t, db, f.TopicID, f.UserID, "the pineapple casserole recipe is excellent")
	newPost(t, db, f.TopicID, f.UserID, "completely unrelated chatter about weather")

	results, total, err := repo.Search(ctx, "pineapple", nil, 100, 1, 25)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if total != 1 || len(results) != 1 {
		t.Fatalf("got %d results (total %d), want exactly the matching post", len(results), total)
	}

	got := results[0]
	if got.PostID != want {
		t.Errorf("PostID = %d, want %d", got.PostID, want)
	}
	if got.TopicID != f.TopicID || got.ForumID != f.ForumID {
		t.Errorf("got topic=%d forum=%d, want topic=%d forum=%d", got.TopicID, got.ForumID, f.TopicID, f.ForumID)
	}
	if got.Username == "" || got.ForumName == "" || got.TopicTitle == "" {
		t.Errorf("denormalised JOIN fields not populated: user=%q forum=%q topic=%q",
			got.Username, got.ForumName, got.TopicTitle)
	}
	// ts_headline wraps the hit in the sentinel markers the service layer later
	// converts to markup. Without them the frontend has nothing to highlight.
	if !strings.Contains(got.Snippet, "!!MARK_START!!pineapple!!MARK_END!!") {
		t.Errorf("Snippet = %q, want the matched term wrapped in !!MARK_*!! sentinels", got.Snippet)
	}
	// PostNumber is the post's rank within its topic, used to build a deep link.
	if got.PostNumber != 1 {
		t.Errorf("PostNumber = %d, want 1 (first post in the topic)", got.PostNumber)
	}
}

func TestForumPostSearchExcludesSoftDeletedPosts(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewForumPostRepo(db)
	f := newForumFixture(t, db)
	mod := newUser(t, db)

	kept := newPost(t, db, f.TopicID, f.UserID, "zebra husbandry tips")
	gone := newPost(t, db, f.TopicID, f.UserID, "zebra husbandry rebuttal")

	if err := repo.SoftDelete(ctx, gone, mod.ID); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}

	results, total, err := repo.Search(ctx, "zebra", nil, 100, 1, 25)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// The tombstone must vanish from search: the count and the page have to
	// agree, or the UI shows "2 results" and renders one.
	if total != 1 || len(results) != 1 {
		t.Fatalf("got %d results (total %d), want 1 — the soft-deleted post must be excluded", len(results), total)
	}
	if results[0].PostID != kept {
		t.Errorf("PostID = %d, want %d (the surviving post)", results[0].PostID, kept)
	}
}

func TestForumPostSearchRespectsForumFilter(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewForumPostRepo(db)
	f := newForumFixture(t, db)
	otherForumID, otherTopicID := newForumWithLevel(t, db, f.UserID, 0)

	here := newPost(t, db, f.TopicID, f.UserID, "walrus migration patterns")
	newPost(t, db, otherTopicID, f.UserID, "walrus migration digest")

	// Unfiltered, both forums match.
	_, total, err := repo.Search(ctx, "walrus", nil, 100, 1, 25)
	if err != nil {
		t.Fatalf("Search(unfiltered): %v", err)
	}
	if total != 2 {
		t.Fatalf("unfiltered total = %d, want 2", total)
	}

	// Scoped to one forum, only that forum's post survives.
	results, total, err := repo.Search(ctx, "walrus", &f.ForumID, 100, 1, 25)
	if err != nil {
		t.Fatalf("Search(forum filter): %v", err)
	}
	if total != 1 || len(results) != 1 {
		t.Fatalf("got %d results (total %d), want 1 scoped to forum %d", len(results), total, f.ForumID)
	}
	if results[0].PostID != here {
		t.Errorf("PostID = %d, want %d", results[0].PostID, here)
	}
	if results[0].ForumID == otherForumID {
		t.Error("filter leaked a post from the other forum")
	}
}

func TestForumPostSearchHidesForumsAboveGroupLevel(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewForumPostRepo(db)
	user := newUser(t, db)

	// A staff-only forum: min_group_level 80 means a level-20 member must not
	// see its posts in search results, even though the text matches.
	_, staffTopic := newForumWithLevel(t, db, user.ID, 80)
	newPost(t, db, staffTopic, user.ID, "narwhal disciplinary policy")

	_, publicTopic := newForumWithLevel(t, db, user.ID, 0)
	publicPost := newPost(t, db, publicTopic, user.ID, "narwhal appreciation thread")

	// A regular member (level 20) sees only the public forum.
	results, total, err := repo.Search(ctx, "narwhal", nil, 20, 1, 25)
	if err != nil {
		t.Fatalf("Search(member): %v", err)
	}
	if total != 1 || len(results) != 1 {
		t.Fatalf("member got %d results (total %d), want 1 — the staff forum must be hidden", len(results), total)
	}
	if results[0].PostID != publicPost {
		t.Errorf("member saw post %d, want the public post %d", results[0].PostID, publicPost)
	}

	// A moderator (level 80) clears the gate and sees both.
	_, total, err = repo.Search(ctx, "narwhal", nil, 80, 1, 25)
	if err != nil {
		t.Fatalf("Search(moderator): %v", err)
	}
	if total != 2 {
		t.Errorf("moderator total = %d, want 2", total)
	}
}

func TestForumPostSearchMatchesTopicTitleViaFirstPost(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewForumPostRepo(db)
	user := newUser(t, db)

	var catID, forumID, topicID int64
	if err := db.QueryRow(`INSERT INTO forum_categories (name) VALUES ($1) RETURNING id`, uniq("fcat")).Scan(&catID); err != nil {
		t.Fatalf("creating forum category: %v", err)
	}
	if err := db.QueryRow(
		`INSERT INTO forums (category_id, name) VALUES ($1, $2) RETURNING id`, catID, uniq("forum"),
	).Scan(&forumID); err != nil {
		t.Fatalf("creating forum: %v", err)
	}
	// The term lives only in the title, never in any post body.
	if err := db.QueryRow(
		`INSERT INTO forum_topics (forum_id, user_id, title) VALUES ($1, $2, $3) RETURNING id`,
		forumID, user.ID, "quokka sightings megathread",
	).Scan(&topicID); err != nil {
		t.Fatalf("creating forum topic: %v", err)
	}

	first := newPost(t, db, topicID, user.ID, "body with no matching term")
	newPost(t, db, topicID, user.ID, "another body with no matching term")

	results, total, err := repo.Search(ctx, "quokka", nil, 100, 1, 25)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// A title hit must surface the topic exactly once, attributed to its first
	// post — not once per post in the topic.
	if total != 1 || len(results) != 1 {
		t.Fatalf("got %d results (total %d), want 1 — a title match must not fan out over every post", len(results), total)
	}
	if results[0].PostID != first {
		t.Errorf("PostID = %d, want the first post %d", results[0].PostID, first)
	}
}

func TestForumPostSearchPaginates(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewForumPostRepo(db)
	f := newForumFixture(t, db)

	for i := 0; i < 5; i++ {
		newPost(t, db, f.TopicID, f.UserID, uniq("axolotl fact "))
	}

	page1, total, err := repo.Search(ctx, "axolotl", nil, 100, 1, 2)
	if err != nil {
		t.Fatalf("Search(page 1): %v", err)
	}
	// total counts every match, independent of the page window.
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(page1) != 2 {
		t.Errorf("page 1 returned %d, want 2", len(page1))
	}

	page3, _, err := repo.Search(ctx, "axolotl", nil, 100, 3, 2)
	if err != nil {
		t.Fatalf("Search(page 3): %v", err)
	}
	if len(page3) != 1 {
		t.Errorf("page 3 returned %d, want the 1 remainder", len(page3))
	}

	// Reading past the end is empty, not an error.
	page9, total, err := repo.Search(ctx, "axolotl", nil, 100, 9, 2)
	if err != nil {
		t.Fatalf("Search(page 9): %v", err)
	}
	if len(page9) != 0 {
		t.Errorf("page 9 returned %d rows, want 0", len(page9))
	}
	if total != 5 {
		t.Errorf("total past the end = %d, want 5", total)
	}
}
