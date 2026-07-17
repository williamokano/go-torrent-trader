package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// The deep-link page number in forum search is derived from post_number, an
// id-rank (COUNT of posts in the topic with id <= this post's id). ListByTopic
// must paginate in that same order or search results link to the wrong page,
// so the created_at ordering needs the id tiebreaker to stay deterministic.
func TestListByTopicOrdersByCreatedAtThenID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := NewForumPostRepo(db)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM forum_posts WHERE topic_id = \$1`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(2)))

	// Both posts share a created_at: without the id tiebreaker Postgres may
	// return them in either order, which is exactly the case that breaks
	// pagination stability and the deep link.
	sameInstant := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"id", "topic_id", "user_id", "body", "mentioned_usernames", "reply_to_post_id",
		"edited_at", "edited_by", "deleted_at", "deleted_by", "created_at",
		"username", "avatar", "group_name", "user_created_at", "user_post_count",
	}).
		AddRow(int64(10), int64(7), int64(1), "first", []byte("[]"), nil,
			nil, nil, nil, nil, sameInstant,
			"alice", nil, "User", sameInstant, 5).
		AddRow(int64(11), int64(7), int64(2), "second", []byte(`["alice"]`), nil,
			nil, nil, nil, nil, sameInstant,
			"bob", nil, "User", sameInstant, 3)

	mock.ExpectQuery(`ORDER BY p\.created_at ASC, p\.id ASC`).
		WithArgs(int64(7), 25, 0).
		WillReturnRows(rows)

	posts, total, err := repo.ListByTopic(context.Background(), 7, 1, 25)
	if err != nil {
		t.Fatalf("ListByTopic returned error: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(posts) != 2 {
		t.Fatalf("len(posts) = %d, want 2", len(posts))
	}
	if posts[0].ID != 10 || posts[1].ID != 11 {
		t.Errorf("post IDs = [%d %d], want [10 11] (ascending id on created_at tie)", posts[0].ID, posts[1].ID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

// The count backing pagination must include soft-deleted posts, because
// ListByTopic returns them as tombstones. If the count filtered them out, the
// page count would disagree with the rows actually rendered.
func TestListByTopicCountIncludesSoftDeletedPosts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := NewForumPostRepo(db)

	deletedAt := time.Date(2026, 7, 14, 13, 0, 0, 0, time.UTC)
	createdAt := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	deleter := int64(99)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM forum_posts WHERE topic_id = \$1`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	rows := sqlmock.NewRows([]string{
		"id", "topic_id", "user_id", "body", "mentioned_usernames", "reply_to_post_id",
		"edited_at", "edited_by", "deleted_at", "deleted_by", "created_at",
		"username", "avatar", "group_name", "user_created_at", "user_post_count",
	}).AddRow(int64(10), int64(7), int64(1), "removed", []byte("[]"), nil,
		nil, nil, deletedAt, deleter, createdAt,
		"alice", nil, "User", createdAt, 5)

	mock.ExpectQuery(`FROM forum_posts p`).
		WithArgs(int64(7), 25, 0).
		WillReturnRows(rows)

	posts, total, err := repo.ListByTopic(context.Background(), 7, 1, 25)
	if err != nil {
		t.Fatalf("ListByTopic returned error: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1 (soft-deleted posts still occupy a slot)", total)
	}
	if len(posts) != 1 || posts[0].DeletedAt == nil {
		t.Fatalf("expected the soft-deleted post to be returned as a tombstone")
	}
}
