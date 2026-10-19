package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// Every topic has at least its first post, so a delete that the schema
// refuses fails for every topic there is. The unit test of DeleteTopic used
// sqlmock and never met the real constraints; this one runs against the
// migrated schema.
func TestForumServiceDeleteTopicRemovesItsPosts(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	authorID := seedUser(t, db, "author")
	replierID := seedUser(t, db, "replier")
	forumID := seedForum(t, db)
	topicID := seedTopic(t, db, forumID, authorID)
	otherTopicID := seedTopic(t, db, forumID, authorID)

	now := time.Now()
	firstPost := seedPost(t, db, topicID, authorID, "first", now)
	seedPost(t, db, topicID, replierID, "reply", now.Add(time.Second))
	keptPost := seedPost(t, db, otherTopicID, replierID, "elsewhere", now)

	postRepo := NewForumPostRepo(db)
	if err := postRepo.CreateEdit(ctx, &model.ForumPostEdit{PostID: firstPost, EditedBy: &authorID, Diff: "@@ -1 +1 @@\n-a\n+b\n"}); err != nil {
		t.Fatalf("CreateEdit: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO topic_subscriptions (user_id, topic_id) VALUES ($1, $2)`, replierID, topicID); err != nil {
		t.Fatalf("subscribing: %v", err)
	}

	svc := service.NewForumService(db, nil, NewForumRepo(db), NewForumTopicRepo(db), postRepo, NewUserRepo(db), nil, event.NewInMemoryBus())
	staff := model.Permissions{IsAdmin: true, CanForum: true}
	if err := svc.DeleteTopic(ctx, topicID, replierID, staff, event.Actor{ID: replierID}, "cleanup"); err != nil {
		t.Fatalf("DeleteTopic: %v", err)
	}

	var posts, edits, subs int
	if err := db.QueryRow(`SELECT COUNT(*) FROM forum_posts WHERE topic_id = $1`, topicID).Scan(&posts); err != nil {
		t.Fatalf("count posts: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM forum_post_edits WHERE post_id = $1`, firstPost).Scan(&edits); err != nil {
		t.Fatalf("count edits: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM topic_subscriptions WHERE topic_id = $1`, topicID).Scan(&subs); err != nil {
		t.Fatalf("count subscriptions: %v", err)
	}
	if posts != 0 || edits != 0 || subs != 0 {
		t.Errorf("left behind posts=%d edits=%d subscriptions=%d, want all 0", posts, edits, subs)
	}

	var forumPosts, forumTopics int
	if err := db.QueryRow(`SELECT post_count, topic_count FROM forums WHERE id = $1`, forumID).Scan(&forumPosts, &forumTopics); err != nil {
		t.Fatalf("read forum counts: %v", err)
	}
	if forumTopics != 1 || forumPosts != 1 {
		t.Errorf("forum counts topics=%d posts=%d, want 1/1 (the other topic)", forumTopics, forumPosts)
	}
	if _, err := postRepo.GetByID(ctx, keptPost); err != nil {
		t.Errorf("a post in another topic went with it: %v", err)
	}
}

// Removing a user who edited a post, or wrote a topic's latest post, clears
// those references instead of being refused.
func TestForumUserReferencesClearOnUserDelete(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)

	authorID := seedUser(t, db, "author")
	modID := seedUser(t, db, "mod")
	forumID := seedForum(t, db)
	topicID := seedTopic(t, db, forumID, authorID)
	postID := seedPost(t, db, topicID, authorID, "body", time.Now())

	if _, err := db.Exec(`UPDATE forum_posts SET edited_by = $1 WHERE id = $2`, modID, postID); err != nil {
		t.Fatalf("set edited_by: %v", err)
	}
	if _, err := db.Exec(`UPDATE forum_topics SET last_post_by = $1 WHERE id = $2`, modID, topicID); err != nil {
		t.Fatalf("set last_post_by: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM users WHERE id = $1`, modID); err != nil {
		t.Fatalf("deleting the user was refused: %v", err)
	}

	var editedBy, lastPostBy *int64
	if err := db.QueryRow(`SELECT edited_by FROM forum_posts WHERE id = $1`, postID).Scan(&editedBy); err != nil {
		t.Fatalf("read post: %v", err)
	}
	if err := db.QueryRow(`SELECT last_post_by FROM forum_topics WHERE id = $1`, topicID).Scan(&lastPostBy); err != nil {
		t.Fatalf("read topic: %v", err)
	}
	if editedBy != nil || lastPostBy != nil {
		t.Errorf("edited_by=%v last_post_by=%v, want both NULL", editedBy, lastPostBy)
	}
}
