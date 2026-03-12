// forum_integration_test.go contains sqlmock-based tests that exercise the transactional
// (s.db != nil) code paths in ForumService. These are NOT true database integration tests —
// they validate transaction control flow (BEGIN/COMMIT/ROLLBACK) and SQL execution order
// using mock expectations. The actual SQL correctness is not verified against PostgreSQL.
// For the repository interface paths (s.db == nil), see forum_test.go.
package service

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// txStaffPerms is a Permissions value with admin flag set and a high level
// so that all access checks pass in transactional tests.
var txStaffPerms = model.Permissions{Level: 999, IsAdmin: true}

// txActor returns a minimal event.Actor for transactional tests that require one.
func txActor() event.Actor {
	return event.Actor{ID: 1, Username: "admin"}
}

// ---------------------------------------------------------------------------
// DeletePost -- transactional happy path
// ---------------------------------------------------------------------------

func TestDeletePost_Transactional(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	postID := int64(10)
	topicID := int64(1)
	forumID := int64(2)

	posts := &mockForumPostRepo{
		postByID:    map[int64]*model.ForumPost{postID: {ID: postID, TopicID: topicID, UserID: 1}},
		firstPostID: 5, // different from postID so it is not the first post
	}
	topics := &mockForumTopicRepo{
		topicByID: map[int64]*model.ForumTopic{topicID: {ID: topicID, ForumID: forumID}},
	}
	forums := &mockForumRepo{
		forumByID: map[int64]*model.Forum{forumID: {ID: forumID, MinGroupLevel: 0}},
	}

	users := &mockForumUserRepo{user: &model.User{ID: 1, Username: "admin", CanForum: true}}
	svc := NewForumService(db, nil, forums, topics, posts, users, nil)

	// Expect: BEGIN -> DELETE post -> decrement topic post_count ->
	// recalculate topic last_post -> decrement forum post_count ->
	// recalculate forum last_post -> COMMIT
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM forum_posts WHERE id =").
		WithArgs(postID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE forum_topics SET post_count =").
		WithArgs(topicID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE forum_topics SET").
		WithArgs(topicID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE forums SET post_count =").
		WithArgs(forumID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE forums SET last_post_id =").
		WithArgs(forumID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = svc.DeletePost(context.Background(), postID, 1, txStaffPerms)
	if err != nil {
		t.Fatalf("DeletePost: unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// DeletePost -- rollback on failure mid-transaction
// ---------------------------------------------------------------------------

func TestDeletePost_Transactional_Rollback(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	postID := int64(10)
	topicID := int64(1)
	forumID := int64(2)

	posts := &mockForumPostRepo{
		postByID:    map[int64]*model.ForumPost{postID: {ID: postID, TopicID: topicID, UserID: 1}},
		firstPostID: 5,
	}
	topics := &mockForumTopicRepo{
		topicByID: map[int64]*model.ForumTopic{topicID: {ID: topicID, ForumID: forumID}},
	}
	forums := &mockForumRepo{
		forumByID: map[int64]*model.Forum{forumID: {ID: forumID, MinGroupLevel: 0}},
	}

	users := &mockForumUserRepo{user: &model.User{ID: 1, Username: "admin", CanForum: true}}
	svc := NewForumService(db, nil, forums, topics, posts, users, nil)

	// DELETE succeeds but the topic post_count update fails -> ROLLBACK
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM forum_posts WHERE id =").
		WithArgs(postID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE forum_topics SET post_count =").
		WithArgs(topicID).
		WillReturnError(fmt.Errorf("db: connection lost"))
	mock.ExpectRollback()

	err = svc.DeletePost(context.Background(), postID, 1, txStaffPerms)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "decrement topic post count") {
		t.Errorf("expected error to contain 'decrement topic post count', got: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// DeletePost -- rollback on BEGIN failure
// ---------------------------------------------------------------------------

func TestDeletePost_Transactional_BeginFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	postID := int64(10)
	topicID := int64(1)
	forumID := int64(2)

	posts := &mockForumPostRepo{
		postByID:    map[int64]*model.ForumPost{postID: {ID: postID, TopicID: topicID, UserID: 1}},
		firstPostID: 5,
	}
	topics := &mockForumTopicRepo{
		topicByID: map[int64]*model.ForumTopic{topicID: {ID: topicID, ForumID: forumID}},
	}
	forums := &mockForumRepo{
		forumByID: map[int64]*model.Forum{forumID: {ID: forumID, MinGroupLevel: 0}},
	}

	users := &mockForumUserRepo{user: &model.User{ID: 1, Username: "admin", CanForum: true}}
	svc := NewForumService(db, nil, forums, topics, posts, users, nil)

	mock.ExpectBegin().WillReturnError(fmt.Errorf("db: too many connections"))

	err = svc.DeletePost(context.Background(), postID, 1, txStaffPerms)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "beginning transaction") {
		t.Errorf("expected error to contain 'beginning transaction', got: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// DeletePost -- COMMIT failure
// ---------------------------------------------------------------------------

func TestDeletePost_Transactional_CommitFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	postID := int64(10)
	topicID := int64(1)
	forumID := int64(2)

	posts := &mockForumPostRepo{
		postByID:    map[int64]*model.ForumPost{postID: {ID: postID, TopicID: topicID, UserID: 1}},
		firstPostID: 5,
	}
	topics := &mockForumTopicRepo{
		topicByID: map[int64]*model.ForumTopic{topicID: {ID: topicID, ForumID: forumID}},
	}
	forums := &mockForumRepo{
		forumByID: map[int64]*model.Forum{forumID: {ID: forumID, MinGroupLevel: 0}},
	}
	users := &mockForumUserRepo{user: &model.User{ID: 1, Username: "admin", CanForum: true}}

	svc := NewForumService(db, nil, forums, topics, posts, users, nil)

	// All SQL succeeds but COMMIT fails
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM forum_posts WHERE id =").
		WithArgs(postID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE forum_topics SET post_count =").
		WithArgs(topicID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE forum_topics SET").
		WithArgs(topicID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE forums SET post_count =").
		WithArgs(forumID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE forums SET last_post_id =").
		WithArgs(forumID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit().WillReturnError(fmt.Errorf("commit failed"))

	err = svc.DeletePost(context.Background(), postID, 1, txStaffPerms)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "committing transaction") {
		t.Errorf("expected error to contain 'committing transaction', got: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// DeletePost -- non-admin owner happy path
// ---------------------------------------------------------------------------

func TestDeletePost_Transactional_OwnerNonStaff(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	postID := int64(10)
	topicID := int64(1)
	forumID := int64(2)
	userID := int64(42)

	posts := &mockForumPostRepo{
		postByID:    map[int64]*model.ForumPost{postID: {ID: postID, TopicID: topicID, UserID: userID}},
		firstPostID: 5, // different from postID so it is not the first post
	}
	topics := &mockForumTopicRepo{
		topicByID: map[int64]*model.ForumTopic{topicID: {ID: topicID, ForumID: forumID}},
	}
	forums := &mockForumRepo{
		forumByID: map[int64]*model.Forum{forumID: {ID: forumID, MinGroupLevel: 0}},
	}
	users := &mockForumUserRepo{user: &model.User{ID: userID, Username: "alice", CanForum: true}}

	svc := NewForumService(db, nil, forums, topics, posts, users, nil)

	nonStaffPerms := model.Permissions{Level: 1, IsAdmin: false, CanForum: true}

	// Expect same transaction flow as staff happy path
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM forum_posts WHERE id =").
		WithArgs(postID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE forum_topics SET post_count =").
		WithArgs(topicID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE forum_topics SET").
		WithArgs(topicID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE forums SET post_count =").
		WithArgs(forumID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE forums SET last_post_id =").
		WithArgs(forumID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = svc.DeletePost(context.Background(), postID, userID, nonStaffPerms)
	if err != nil {
		t.Fatalf("DeletePost: unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// MoveTopic -- transactional happy path
// ---------------------------------------------------------------------------

func TestMoveTopic_Transactional(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	topicID := int64(1)
	oldForumID := int64(10)
	newForumID := int64(20)

	topics := &mockForumTopicRepo{
		topicByID: map[int64]*model.ForumTopic{topicID: {ID: topicID, ForumID: oldForumID, Title: "Test Topic"}},
	}
	forums := &mockForumRepo{
		forumByID: map[int64]*model.Forum{
			oldForumID: {ID: oldForumID, MinGroupLevel: 0},
			newForumID: {ID: newForumID, MinGroupLevel: 0},
		},
	}

	svc := NewForumService(db, nil, forums, topics, nil, nil, nil)

	// Expect: BEGIN -> UPDATE forum_id -> recalculate old forum -> recalculate new forum -> COMMIT
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE forum_topics SET forum_id =").
		WithArgs(newForumID, topicID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE forums SET").
		WithArgs(oldForumID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE forums SET").
		WithArgs(newForumID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = svc.MoveTopic(context.Background(), topicID, txStaffPerms, newForumID, txActor())
	if err != nil {
		t.Fatalf("MoveTopic: unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// MoveTopic -- rollback on failure mid-transaction
// ---------------------------------------------------------------------------

func TestMoveTopic_Transactional_Rollback(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	topicID := int64(1)
	oldForumID := int64(10)
	newForumID := int64(20)

	topics := &mockForumTopicRepo{
		topicByID: map[int64]*model.ForumTopic{topicID: {ID: topicID, ForumID: oldForumID, Title: "Test Topic"}},
	}
	forums := &mockForumRepo{
		forumByID: map[int64]*model.Forum{
			oldForumID: {ID: oldForumID, MinGroupLevel: 0},
			newForumID: {ID: newForumID, MinGroupLevel: 0},
		},
	}

	svc := NewForumService(db, nil, forums, topics, nil, nil, nil)

	// UPDATE forum_id succeeds but old-forum recalculation fails -> ROLLBACK
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE forum_topics SET forum_id =").
		WithArgs(newForumID, topicID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE forums SET").
		WithArgs(oldForumID).
		WillReturnError(fmt.Errorf("db: disk full"))
	mock.ExpectRollback()

	err = svc.MoveTopic(context.Background(), topicID, txStaffPerms, newForumID, txActor())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "recalculate old forum") {
		t.Errorf("expected error to contain 'recalculate old forum', got: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// DeleteTopic -- transactional happy path
// ---------------------------------------------------------------------------

func TestDeleteTopic_Transactional(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	topicID := int64(1)
	forumID := int64(5)

	topics := &mockForumTopicRepo{
		topicByID: map[int64]*model.ForumTopic{topicID: {ID: topicID, ForumID: forumID, Title: "Doomed Topic"}},
	}

	svc := NewForumService(db, nil, nil, topics, nil, nil, nil)

	// Expect: BEGIN -> DELETE topic -> recalculate forum counts -> COMMIT
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM forum_topics WHERE id =").
		WithArgs(topicID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE forums SET").
		WithArgs(forumID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = svc.DeleteTopic(context.Background(), topicID, txStaffPerms, txActor())
	if err != nil {
		t.Fatalf("DeleteTopic: unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// DeleteTopic -- rollback on failure mid-transaction
// ---------------------------------------------------------------------------

func TestDeleteTopic_Transactional_Rollback(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	topicID := int64(1)
	forumID := int64(5)

	topics := &mockForumTopicRepo{
		topicByID: map[int64]*model.ForumTopic{topicID: {ID: topicID, ForumID: forumID, Title: "Doomed Topic"}},
	}

	svc := NewForumService(db, nil, nil, topics, nil, nil, nil)

	// DELETE topic succeeds but forum recalculation fails -> ROLLBACK
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM forum_topics WHERE id =").
		WithArgs(topicID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE forums SET").
		WithArgs(forumID).
		WillReturnError(fmt.Errorf("db: timeout"))
	mock.ExpectRollback()

	err = svc.DeleteTopic(context.Background(), topicID, txStaffPerms, txActor())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "recalculate forum") {
		t.Errorf("expected error to contain 'recalculate forum', got: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// CreateTopic -- transactional happy path
// ---------------------------------------------------------------------------

func TestCreateTopic_Transactional(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	forumID := int64(1)
	userID := int64(42)
	now := time.Now()

	forums := &mockForumRepo{
		forumByID: map[int64]*model.Forum{forumID: {ID: forumID, MinGroupLevel: 0, MinPostLevel: 0}},
	}
	users := &mockForumUserRepo{user: &model.User{ID: userID, Username: "alice", CanForum: true}}

	svc := NewForumService(db, nil, forums, nil, nil, users, nil)

	// Expect: BEGIN -> INSERT topic -> INSERT post -> UPDATE topic counts -> UPDATE forum counts -> COMMIT
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO forum_topics").
		WithArgs(forumID, userID, "New Topic").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
			AddRow(100, now, now))
	mock.ExpectQuery("INSERT INTO forum_posts").
		WithArgs(int64(100), userID, "Hello world", nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).
			AddRow(200, now))
	mock.ExpectExec("UPDATE forum_topics SET post_count =").
		WithArgs(int64(200), now, int64(100)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE forums SET topic_count =").
		WithArgs(int64(200), forumID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	topic, post, err := svc.CreateTopic(context.Background(), forumID, userID, model.Permissions{Level: 10}, "New Topic", "Hello world")
	if err != nil {
		t.Fatalf("CreateTopic: unexpected error: %v", err)
	}
	if topic.ID != 100 {
		t.Errorf("expected topic.ID=100, got %d", topic.ID)
	}
	if post.ID != 200 {
		t.Errorf("expected post.ID=200, got %d", post.ID)
	}
	if topic.Title != "New Topic" {
		t.Errorf("expected topic.Title='New Topic', got %q", topic.Title)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// CreateTopic -- rollback when INSERT post fails
// ---------------------------------------------------------------------------

func TestCreateTopic_Transactional_Rollback(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	forumID := int64(1)
	userID := int64(42)
	now := time.Now()

	forums := &mockForumRepo{
		forumByID: map[int64]*model.Forum{forumID: {ID: forumID, MinGroupLevel: 0, MinPostLevel: 0}},
	}
	users := &mockForumUserRepo{user: &model.User{ID: userID, Username: "alice", CanForum: true}}

	svc := NewForumService(db, nil, forums, nil, nil, users, nil)

	// INSERT topic succeeds, INSERT post fails -> ROLLBACK
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO forum_topics").
		WithArgs(forumID, userID, "New Topic").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
			AddRow(100, now, now))
	mock.ExpectQuery("INSERT INTO forum_posts").
		WithArgs(int64(100), userID, "Hello world", nil).
		WillReturnError(fmt.Errorf("db: unique violation"))
	mock.ExpectRollback()

	_, _, err = svc.CreateTopic(context.Background(), forumID, userID, model.Permissions{Level: 10}, "New Topic", "Hello world")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "create post") {
		t.Errorf("expected error to contain 'create post', got: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// CreatePost -- transactional happy path
// ---------------------------------------------------------------------------

func TestCreatePost_Transactional(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	topicID := int64(1)
	forumID := int64(5)
	userID := int64(42)
	now := time.Now()
	postID := int64(300)

	topics := &mockForumTopicRepo{
		topicByID: map[int64]*model.ForumTopic{topicID: {ID: topicID, ForumID: forumID, Title: "Existing Topic"}},
	}
	forums := &mockForumRepo{
		forumByID: map[int64]*model.Forum{forumID: {ID: forumID, MinGroupLevel: 0}},
	}
	users := &mockForumUserRepo{user: &model.User{ID: userID, Username: "bob", CanForum: true}}
	posts := &mockForumPostRepo{
		postByID: map[int64]*model.ForumPost{
			postID: {ID: postID, TopicID: topicID, UserID: userID, Username: "bob", GroupName: "User"},
		},
	}

	svc := NewForumService(db, nil, forums, topics, posts, users, nil)

	// Expect: BEGIN -> INSERT post -> UPDATE topic counts -> UPDATE forum counts -> COMMIT
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO forum_posts").
		WithArgs(topicID, userID, "Reply body", nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).
			AddRow(postID, now))
	mock.ExpectExec("UPDATE forum_topics SET post_count =").
		WithArgs(postID, now, topicID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE forums SET post_count =").
		WithArgs(postID, forumID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := svc.CreatePost(context.Background(), topicID, userID, model.Permissions{Level: 10}, "Reply body", nil)
	if err != nil {
		t.Fatalf("CreatePost: unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// CreatePost -- rollback when topic counter update fails
// ---------------------------------------------------------------------------

func TestCreatePost_Transactional_Rollback(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	topicID := int64(1)
	forumID := int64(5)
	userID := int64(42)
	now := time.Now()
	postID := int64(300)

	topics := &mockForumTopicRepo{
		topicByID: map[int64]*model.ForumTopic{topicID: {ID: topicID, ForumID: forumID, Title: "Existing Topic"}},
	}
	forums := &mockForumRepo{
		forumByID: map[int64]*model.Forum{forumID: {ID: forumID, MinGroupLevel: 0}},
	}
	users := &mockForumUserRepo{user: &model.User{ID: userID, Username: "bob", CanForum: true}}

	svc := NewForumService(db, nil, forums, topics, nil, users, nil)

	// INSERT post succeeds, UPDATE topic counts fails -> ROLLBACK
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO forum_posts").
		WithArgs(topicID, userID, "Reply body", nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).
			AddRow(postID, now))
	mock.ExpectExec("UPDATE forum_topics SET post_count =").
		WithArgs(postID, now, topicID).
		WillReturnError(fmt.Errorf("db: lock timeout"))
	mock.ExpectRollback()

	_, err = svc.CreatePost(context.Background(), topicID, userID, model.Permissions{Level: 10}, "Reply body", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "update topic counts") {
		t.Errorf("expected error to contain 'update topic counts', got: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
