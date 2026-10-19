package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

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
	// Newer than keptPost but soft-deleted: it must not count, nor show as the
	// forum's latest post, once the recount runs.
	hiddenPost := seedPost(t, db, otherTopicID, replierID, "removed", now.Add(time.Minute))
	if _, err := db.Exec(`UPDATE forum_posts SET deleted_at = NOW() WHERE id = $1`, hiddenPost); err != nil {
		t.Fatalf("soft-deleting: %v", err)
	}
	// A reply in another topic quoting a post that is about to go.
	quoting := seedPost(t, db, otherTopicID, replierID, "quoting", now)
	if _, err := db.Exec(`UPDATE forum_posts SET reply_to_post_id = $1 WHERE id = $2`, firstPost, quoting); err != nil {
		t.Fatalf("linking reply: %v", err)
	}

	postRepo := NewForumPostRepo(db)
	if err := postRepo.CreateEdit(ctx, &model.ForumPostEdit{PostID: firstPost, EditedBy: &authorID, Diff: "@@ -1 +1 @@\n-a\n+b\n"}); err != nil {
		t.Fatalf("CreateEdit: %v", err)
	}
	if err := NewTopicSubscriptionRepo(db).Subscribe(ctx, replierID, topicID); err != nil {
		t.Fatalf("subscribing: %v", err)
	}

	svc := service.NewForumService(db, nil, NewForumRepo(db), NewForumTopicRepo(db), postRepo, NewUserRepo(db), nil, event.NewInMemoryBus())
	staff := model.Permissions{IsAdmin: true, CanForum: true}
	if err := svc.DeleteTopic(ctx, topicID, replierID, staff, event.Actor{ID: replierID}, "cleanup"); err != nil {
		t.Fatalf("DeleteTopic: %v", err)
	}

	var posts, edits, subs int
	if err := db.QueryRow(`SELECT
		(SELECT COUNT(*) FROM forum_posts WHERE topic_id = $1),
		(SELECT COUNT(*) FROM forum_post_edits WHERE post_id = $2),
		(SELECT COUNT(*) FROM topic_subscriptions WHERE topic_id = $1)`,
		topicID, firstPost).Scan(&posts, &edits, &subs); err != nil {
		t.Fatalf("count leftovers: %v", err)
	}
	if posts != 0 || edits != 0 || subs != 0 {
		t.Errorf("left behind posts=%d edits=%d subscriptions=%d, want all 0", posts, edits, subs)
	}

	var forumPosts, forumTopics int
	var lastPost *int64
	if err := db.QueryRow(`SELECT post_count, topic_count, last_post_id FROM forums WHERE id = $1`, forumID).Scan(&forumPosts, &forumTopics, &lastPost); err != nil {
		t.Fatalf("read forum counts: %v", err)
	}
	if forumTopics != 1 || forumPosts != 2 {
		t.Errorf("forum counts topics=%d posts=%d, want 1/2 (the other topic's live posts)", forumTopics, forumPosts)
	}
	if lastPost == nil || (*lastPost != keptPost && *lastPost != quoting) {
		t.Errorf("forum last_post_id = %v, want a live post of the other topic, never the soft-deleted %d", lastPost, hiddenPost)
	}
	var replyTo *int64
	if err := db.QueryRow(`SELECT reply_to_post_id FROM forum_posts WHERE id = $1`, quoting).Scan(&replyTo); err != nil {
		t.Fatalf("read quoting reply: %v", err)
	}
	if replyTo != nil {
		t.Errorf("reply_to_post_id = %d, want NULL once the quoted post is gone", *replyTo)
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
	if err := db.QueryRow(`SELECT
		(SELECT edited_by FROM forum_posts WHERE id = $1),
		(SELECT last_post_by FROM forum_topics WHERE id = $2)`,
		postID, topicID).Scan(&editedBy, &lastPostBy); err != nil {
		t.Fatalf("read references: %v", err)
	}
	if editedBy != nil || lastPostBy != nil {
		t.Errorf("edited_by=%v last_post_by=%v, want both NULL", editedBy, lastPostBy)
	}
}

// Removing a member who wrote in the forum stays refused rather than
// cascading: their topics hold other members' replies, which must not go with
// them.
func TestForumAuthorCannotBeHardDeleted(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)

	authorID := seedUser(t, db, "author")
	replierID := seedUser(t, db, "replier")
	topicID := seedTopic(t, db, seedForum(t, db), authorID)
	seedPost(t, db, topicID, replierID, "someone else's reply", time.Now())

	_, err := db.Exec(`DELETE FROM users WHERE id = $1`, authorID)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" || pgErr.ConstraintName != "forum_topics_user_id_fkey" {
		t.Fatalf("deleting a topic's author: err = %v, want a foreign-key refusal on forum_topics_user_id_fkey", err)
	}
	var replies int
	if err := db.QueryRow(`SELECT COUNT(*) FROM forum_posts WHERE topic_id = $1`, topicID).Scan(&replies); err != nil {
		t.Fatalf("count: %v", err)
	}
	if replies != 1 {
		t.Errorf("replies = %d, want the other member's reply kept", replies)
	}
}

// Who may delete a topic, against the real schema: the author within the
// grace period, staff over a non-admin's topic — and not the author once the
// grace period is over.
func TestForumServiceDeleteTopicPermissions(t *testing.T) {
	db := requireDB(t)
	ctx := context.Background()

	cases := []struct {
		name    string
		byOwner bool
		perms   model.Permissions
		age     time.Duration
		wantErr error
	}{
		{"owner within grace", true, model.Permissions{CanForum: true}, 0, nil},
		{"owner after grace", true, model.Permissions{CanForum: true}, 2 * time.Hour, service.ErrTopicDeleteDenied},
		{"moderator", false, model.Permissions{IsModerator: true, CanForum: true}, 2 * time.Hour, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetTestData(t, db)
			authorID := seedUser(t, db, "author")
			modID := seedUser(t, db, "mod")
			if _, err := db.Exec(`UPDATE users SET group_id = (SELECT id FROM groups WHERE NOT is_admin AND NOT is_moderator ORDER BY level LIMIT 1) WHERE id = $1`, authorID); err != nil {
				t.Fatalf("demoting author: %v", err)
			}
			topicID := seedTopic(t, db, seedForum(t, db), authorID)
			seedPost(t, db, topicID, authorID, "first", time.Now())
			if _, err := db.Exec(`UPDATE forum_topics SET created_at = NOW() - $1::interval WHERE id = $2`, tc.age.String(), topicID); err != nil {
				t.Fatalf("ageing topic: %v", err)
			}

			svc := service.NewForumService(db, nil, NewForumRepo(db), NewForumTopicRepo(db), NewForumPostRepo(db), NewUserRepo(db), NewGroupRepo(db), event.NewInMemoryBus())
			actor := modID
			if tc.byOwner {
				actor = authorID
			}
			err := svc.DeleteTopic(ctx, topicID, actor, tc.perms, event.Actor{ID: actor}, "")
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("DeleteTopic err = %v, want %v", err, tc.wantErr)
			}
			var left int
			_ = db.QueryRow(`SELECT COUNT(*) FROM forum_topics WHERE id = $1`, topicID).Scan(&left)
			if (tc.wantErr == nil) != (left == 0) {
				t.Errorf("topic rows left = %d with err = %v", left, err)
			}
		})
	}
}
