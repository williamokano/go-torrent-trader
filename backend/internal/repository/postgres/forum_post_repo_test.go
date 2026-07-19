package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// newPost inserts a post through the repository under test and returns it, so a
// broken Create surfaces here rather than being papered over by bespoke SQL.
func newPost(t *testing.T, db *sql.DB, topicID, userID int64, body string) int64 {
	t.Helper()

	post := &model.ForumPost{TopicID: topicID, UserID: userID, Body: body}
	if err := NewForumPostRepo(db).Create(context.Background(), post); err != nil {
		t.Fatalf("creating post: %v", err)
	}
	return post.ID
}

func TestForumPostRepoGetByIDEnrichesAuthor(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewForumPostRepo(db)
	f := newForumFixture(t, db)

	// Two posts by the same author: UserPostCount is a correlated subquery over
	// the author's posts, so it must report 2 on either of them.
	id := newPost(t, db, f.TopicID, f.UserID, "first body")
	newPost(t, db, f.TopicID, f.UserID, "second body")

	got, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Body != "first body" {
		t.Errorf("Body = %q, want %q", got.Body, "first body")
	}
	if got.TopicID != f.TopicID || got.UserID != f.UserID {
		t.Errorf("got topic=%d user=%d, want topic=%d user=%d", got.TopicID, got.UserID, f.TopicID, f.UserID)
	}
	// Username and GroupName come from the JOINs, not from the caller.
	if got.Username == "" {
		t.Error("Username is empty; the users JOIN did not resolve")
	}
	if got.GroupName == "" {
		t.Error("GroupName is empty; the groups JOIN did not resolve")
	}
	if got.UserPostCount != 2 {
		t.Errorf("UserPostCount = %d, want 2", got.UserPostCount)
	}
	if got.DeletedAt != nil {
		t.Errorf("DeletedAt = %v on a live post, want nil", got.DeletedAt)
	}
}

func TestForumPostRepoGetByIDMissingIsErrNoRows(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)

	// GetByID surfaces absence as sql.ErrNoRows rather than a nil post with a
	// nil error, so callers can distinguish "gone" from "empty".
	got, err := NewForumPostRepo(db).GetByID(context.Background(), 999999)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("err = %v, want sql.ErrNoRows", err)
	}
	if got != nil {
		t.Errorf("post = %+v, want nil", got)
	}
}

func TestForumPostRepoUpdateStampsEditor(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewForumPostRepo(db)
	f := newForumFixture(t, db)
	editor := newUser(t, db)
	id := newPost(t, db, f.TopicID, f.UserID, "original")

	before, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID before update: %v", err)
	}
	if before.EditedAt != nil || before.EditedBy != nil {
		t.Fatalf("fresh post already marked edited: at=%v by=%v", before.EditedAt, before.EditedBy)
	}

	post := &model.ForumPost{ID: id, Body: "edited", EditedBy: &editor.ID}
	if err := repo.Update(ctx, post, "original"); err != nil {
		t.Fatalf("Update: %v", err)
	}

	after, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if after.Body != "edited" {
		t.Errorf("Body = %q, want %q", after.Body, "edited")
	}
	// Update sets edited_at = NOW() and edited_by in SQL; a caller that only
	// passes the body must still end up with an attributed edit.
	if after.EditedAt == nil {
		t.Error("EditedAt is nil after Update; the edit was not stamped")
	}
	if after.EditedBy == nil || *after.EditedBy != editor.ID {
		t.Errorf("EditedBy = %v, want %d", after.EditedBy, editor.ID)
	}
}

// BE-9.25: Update's WHERE clause is conditional on the caller's oldBody
// still matching the row. A stale oldBody (what a concurrent edit landing
// first would produce) must be rejected rather than silently overwriting
// whatever the row currently holds.
func TestForumPostRepoUpdateRejectsStaleOldBody(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewForumPostRepo(db)
	f := newForumFixture(t, db)
	editor := newUser(t, db)
	id := newPost(t, db, f.TopicID, f.UserID, "current body")

	post := &model.ForumPost{ID: id, Body: "clobbered", EditedBy: &editor.ID}
	err := repo.Update(ctx, post, "a stale body a concurrent reader saw earlier")
	if !errors.Is(err, repository.ErrEditConflict) {
		t.Fatalf("err = %v, want repository.ErrEditConflict", err)
	}

	// The row must be untouched: no partial write, no edited_at/edited_by
	// stamped for an update that didn't actually happen.
	after, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if after.Body != "current body" {
		t.Errorf("Body = %q after a rejected update, want the original %q untouched", after.Body, "current body")
	}
	if after.EditedAt != nil || after.EditedBy != nil {
		t.Errorf("post stamped as edited (at=%v by=%v) despite the conditional update being rejected", after.EditedAt, after.EditedBy)
	}
}

func TestForumPostRepoDeleteRemovesRowEntirely(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewForumPostRepo(db)
	f := newForumFixture(t, db)
	id := newPost(t, db, f.TopicID, f.UserID, "doomed")

	if err := repo.Delete(ctx, id); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Delete is a hard delete, unlike SoftDelete: the row is gone, not
	// tombstoned, so GetByID cannot find it at all.
	if _, err := repo.GetByID(ctx, id); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("GetByID after Delete: err = %v, want sql.ErrNoRows", err)
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM forum_posts WHERE id = $1`, id).Scan(&n); err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	if n != 0 {
		t.Errorf("%d rows remain after Delete, want 0", n)
	}
}

func TestForumPostRepoCountByUserExcludesSoftDeleted(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewForumPostRepo(db)
	f := newForumFixture(t, db)
	other := newUser(t, db)

	newPost(t, db, f.TopicID, f.UserID, "kept one")
	tombstoned := newPost(t, db, f.TopicID, f.UserID, "will be tombstoned")
	newPost(t, db, f.TopicID, other.ID, "another user's post")

	if err := repo.SoftDelete(ctx, tombstoned, other.ID); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}

	// A tombstoned post still occupies a row but must not inflate the author's
	// post count.
	count, err := repo.CountByUser(ctx, f.UserID)
	if err != nil {
		t.Fatalf("CountByUser: %v", err)
	}
	if count != 1 {
		t.Errorf("CountByUser = %d, want 1 (the soft-deleted post must not count)", count)
	}

	// The count is scoped per user, not global.
	count, err = repo.CountByUser(ctx, other.ID)
	if err != nil {
		t.Fatalf("CountByUser(other): %v", err)
	}
	if count != 1 {
		t.Errorf("CountByUser(other) = %d, want 1", count)
	}

	// A user with no posts counts zero rather than erroring.
	count, err = repo.CountByUser(ctx, 999999)
	if err != nil {
		t.Fatalf("CountByUser(unknown): %v", err)
	}
	if count != 0 {
		t.Errorf("CountByUser(unknown) = %d, want 0", count)
	}
}

func TestForumPostRepoSoftDeleteThenRestore(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewForumPostRepo(db)
	f := newForumFixture(t, db)
	mod := newUser(t, db)
	id := newPost(t, db, f.TopicID, f.UserID, "contentious")

	if err := repo.SoftDelete(ctx, id, mod.ID); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}

	deleted, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID after SoftDelete: %v", err)
	}
	if deleted.DeletedAt == nil {
		t.Error("DeletedAt is nil after SoftDelete")
	}
	if deleted.DeletedBy == nil || *deleted.DeletedBy != mod.ID {
		t.Errorf("DeletedBy = %v, want %d", deleted.DeletedBy, mod.ID)
	}

	// SoftDelete guards on `deleted_at IS NULL`, so deleting an already-deleted
	// post affects no rows and must report that rather than silently succeeding.
	if err := repo.SoftDelete(ctx, id, mod.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("second SoftDelete: err = %v, want sql.ErrNoRows", err)
	}

	if err := repo.Restore(ctx, id); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	restored, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("GetByID after Restore: %v", err)
	}
	// Restore must clear both columns: leaving deleted_by set would make the
	// post look live while still attributing a deletion to a moderator.
	if restored.DeletedAt != nil {
		t.Errorf("DeletedAt = %v after Restore, want nil", restored.DeletedAt)
	}
	if restored.DeletedBy != nil {
		t.Errorf("DeletedBy = %v after Restore, want nil", restored.DeletedBy)
	}

	// Restoring makes the post soft-deletable again.
	if err := repo.SoftDelete(ctx, id, mod.ID); err != nil {
		t.Errorf("SoftDelete after Restore: %v", err)
	}
}

func TestForumPostRepoSoftDeleteMissingIsErrNoRows(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)

	err := NewForumPostRepo(db).SoftDelete(context.Background(), 999999, 1)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("SoftDelete(unknown id): err = %v, want sql.ErrNoRows", err)
	}
}
