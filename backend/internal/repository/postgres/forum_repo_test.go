package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// --- forums -----------------------------------------------------------------

func TestForumRepoCRUD(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewForumRepo(db)
	f := newForumFixture(t, db)

	forum := &model.Forum{
		CategoryID:  f.CategoryID,
		Name:        "Announcements",
		Description: "site news",
		SortOrder:   1,
	}
	if err := repo.Create(ctx, forum); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, forum.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "Announcements" || got.Description != "site news" {
		t.Errorf("name=%q description=%q", got.Name, got.Description)
	}

	forum.Name = "News"
	forum.MinGroupLevel = 5
	if err := repo.Update(ctx, forum); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err = repo.GetByID(ctx, forum.ID)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if got.Name != "News" || got.MinGroupLevel != 5 {
		t.Errorf("update did not persist: name=%q level=%d", got.Name, got.MinGroupLevel)
	}

	byCat, err := repo.ListByCategory(ctx, f.CategoryID)
	if err != nil {
		t.Fatalf("ListByCategory: %v", err)
	}
	// The fixture creates one forum, and this test added another.
	if len(byCat) != 2 {
		t.Errorf("got %d forums in the category, want 2", len(byCat))
	}

	all, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) < 2 {
		t.Errorf("List returned %d forums, want at least 2", len(all))
	}

	if err := repo.Delete(ctx, forum.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.GetByID(ctx, forum.ID); err == nil {
		t.Error("GetByID succeeded after Delete")
	}
}

func TestForumRepoCountersAndRecalculate(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewForumRepo(db)
	postRepo := NewForumPostRepo(db)
	f := newForumFixture(t, db)

	if err := repo.IncrementTopicCount(ctx, f.ForumID, 1); err != nil {
		t.Fatalf("IncrementTopicCount: %v", err)
	}
	if err := repo.IncrementPostCount(ctx, f.ForumID, 2); err != nil {
		t.Fatalf("IncrementPostCount: %v", err)
	}

	got, err := repo.GetByID(ctx, f.ForumID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.TopicCount != 1 || got.PostCount != 2 {
		t.Errorf("topics=%d posts=%d, want 1/2", got.TopicCount, got.PostCount)
	}

	n, err := repo.CountTopicsByForum(ctx, f.ForumID)
	if err != nil {
		t.Fatalf("CountTopicsByForum: %v", err)
	}
	if n != 1 {
		t.Errorf("actual topics = %d, want 1 (the fixture's topic)", n)
	}

	// Seed two real posts, then let the repo recompute from the rows.
	post := &model.ForumPost{TopicID: f.TopicID, UserID: f.UserID, Body: "first"}
	if err := postRepo.Create(ctx, post); err != nil {
		t.Fatalf("creating post: %v", err)
	}
	newest := &model.ForumPost{TopicID: f.TopicID, UserID: f.UserID, Body: "second"}
	if err := postRepo.Create(ctx, newest); err != nil {
		t.Fatalf("creating post: %v", err)
	}

	if err := repo.UpdateLastPost(ctx, f.ForumID, post.ID); err != nil {
		t.Fatalf("UpdateLastPost: %v", err)
	}
	got, err = repo.GetByID(ctx, f.ForumID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.LastPostID == nil || *got.LastPostID != post.ID {
		t.Errorf("last_post_id = %v, want %d", got.LastPostID, post.ID)
	}

	// RecalculateCounts derives the counters from the rows, correcting the
	// deliberately wrong values set above.
	if err := repo.RecalculateCounts(ctx, f.ForumID); err != nil {
		t.Fatalf("RecalculateCounts: %v", err)
	}
	got, err = repo.GetByID(ctx, f.ForumID)
	if err != nil {
		t.Fatalf("GetByID after recalc: %v", err)
	}
	if got.TopicCount != 1 || got.PostCount != 2 {
		t.Errorf("after recalc topics=%d posts=%d, want 1/2 derived from the rows", got.TopicCount, got.PostCount)
	}
	if got.LastPostID == nil || *got.LastPostID != newest.ID {
		t.Errorf("last_post_id = %v, want the newest post %d", got.LastPostID, newest.ID)
	}
}

// --- forum topics -----------------------------------------------------------

func TestForumTopicRepoCRUDAndFlags(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewForumTopicRepo(db)
	f := newForumFixture(t, db)

	topic := &model.ForumTopic{ForumID: f.ForumID, UserID: f.UserID, Title: "How do I seed?"}
	if err := repo.Create(ctx, topic); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByID(ctx, topic.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Title != "How do I seed?" || got.Username == "" {
		t.Errorf("title=%q username=%q — the author JOIN should resolve", got.Title, got.Username)
	}

	list, total, err := repo.ListByForum(ctx, f.ForumID, 1, 10)
	if err != nil {
		t.Fatalf("ListByForum: %v", err)
	}
	// The fixture's topic plus this one.
	if total != 2 || len(list) != 2 {
		t.Errorf("got %d topics (total %d), want 2", len(list), total)
	}

	t.Run("lock and pin", func(t *testing.T) {
		if err := repo.SetLocked(ctx, topic.ID, true); err != nil {
			t.Fatalf("SetLocked: %v", err)
		}
		if err := repo.SetPinned(ctx, topic.ID, true); err != nil {
			t.Fatalf("SetPinned: %v", err)
		}
		got, err := repo.GetByID(ctx, topic.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if !got.Locked || !got.Pinned {
			t.Errorf("locked=%v pinned=%v, want both true", got.Locked, got.Pinned)
		}
	})

	t.Run("rename", func(t *testing.T) {
		if err := repo.UpdateTitle(ctx, topic.ID, "Seeding help"); err != nil {
			t.Fatalf("UpdateTitle: %v", err)
		}
		got, err := repo.GetByID(ctx, topic.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.Title != "Seeding help" {
			t.Errorf("title = %q, want Seeding help", got.Title)
		}
	})

	t.Run("move to another forum", func(t *testing.T) {
		other := newForumFixture(t, db)
		if err := repo.UpdateForumID(ctx, topic.ID, other.ForumID); err != nil {
			t.Fatalf("UpdateForumID: %v", err)
		}
		got, err := repo.GetByID(ctx, topic.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.ForumID != other.ForumID {
			t.Errorf("forum = %d, want %d", got.ForumID, other.ForumID)
		}
	})

	t.Run("delete", func(t *testing.T) {
		if err := repo.Delete(ctx, topic.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := repo.GetByID(ctx, topic.ID); err == nil {
			t.Error("GetByID succeeded after Delete")
		}
	})
}

func TestForumTopicRepoCountersAndLastPost(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewForumTopicRepo(db)
	postRepo := NewForumPostRepo(db)
	f := newForumFixture(t, db)

	if err := repo.IncrementViewCount(ctx, f.TopicID); err != nil {
		t.Fatalf("IncrementViewCount: %v", err)
	}
	if err := repo.IncrementPostCount(ctx, f.TopicID, 3); err != nil {
		t.Fatalf("IncrementPostCount: %v", err)
	}

	got, err := repo.GetByID(ctx, f.TopicID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ViewCount != 1 || got.PostCount != 3 {
		t.Errorf("views=%d posts=%d, want 1/3", got.ViewCount, got.PostCount)
	}

	post := &model.ForumPost{TopicID: f.TopicID, UserID: f.UserID, Body: "hello"}
	if err := postRepo.Create(ctx, post); err != nil {
		t.Fatalf("creating post: %v", err)
	}

	at := time.Now()
	if err := repo.UpdateLastPost(ctx, f.TopicID, post.ID, at); err != nil {
		t.Fatalf("UpdateLastPost: %v", err)
	}
	got, err = repo.GetByID(ctx, f.TopicID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.LastPostID == nil || *got.LastPostID != post.ID || got.LastPostAt == nil {
		t.Errorf("last post = %v at %v, want %d", got.LastPostID, got.LastPostAt, post.ID)
	}

	// Recalculating from the rows must agree with what was set explicitly.
	if err := repo.RecalculateLastPost(ctx, f.TopicID); err != nil {
		t.Fatalf("RecalculateLastPost: %v", err)
	}
	got, err = repo.GetByID(ctx, f.TopicID)
	if err != nil {
		t.Fatalf("GetByID after recalc: %v", err)
	}
	if got.LastPostID == nil || *got.LastPostID != post.ID {
		t.Errorf("last_post_id = %v after recalc, want %d", got.LastPostID, post.ID)
	}
}
