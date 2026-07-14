package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

func TestTorrentRepoCreateGetUpdateDelete(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewTorrentRepo(db)
	u := newUser(t, db)
	tor := newTorrent(t, db, u.ID)

	t.Run("get by id resolves the uploader", func(t *testing.T) {
		got, err := repo.GetByID(ctx, tor.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.Name != tor.Name {
			t.Errorf("name = %q, want %q", got.Name, tor.Name)
		}
		if got.UploaderName != u.Username {
			t.Errorf("uploader = %q, want %q (resolved via JOIN)", got.UploaderName, u.Username)
		}
	})

	t.Run("get by info hash", func(t *testing.T) {
		got, err := repo.GetByInfoHash(ctx, tor.InfoHash)
		if err != nil {
			t.Fatalf("GetByInfoHash: %v", err)
		}
		if got.ID != tor.ID {
			t.Errorf("id = %d, want %d", got.ID, tor.ID)
		}
	})

	t.Run("update persists", func(t *testing.T) {
		tor.Name = "Renamed.Release"
		tor.Free = true
		if err := repo.Update(ctx, tor); err != nil {
			t.Fatalf("Update: %v", err)
		}
		got, err := repo.GetByID(ctx, tor.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.Name != "Renamed.Release" || !got.Free {
			t.Errorf("update did not persist: name=%q free=%v", got.Name, got.Free)
		}
	})

	t.Run("delete removes it", func(t *testing.T) {
		if err := repo.Delete(ctx, tor.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := repo.GetByID(ctx, tor.ID); err == nil {
			t.Error("GetByID succeeded after Delete")
		}
	})
}

// An anonymous torrent must not leak its uploader's username through the JOIN.
func TestTorrentRepoAnonymousHidesUploader(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewTorrentRepo(db)
	u := newUser(t, db)
	tor := newTorrent(t, db, u.ID)

	tor.Anonymous = true
	if err := repo.Update(ctx, tor); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByID(ctx, tor.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.UploaderName == u.Username {
		t.Errorf("uploader name = %q — an anonymous torrent must not expose the real uploader", got.UploaderName)
	}
}

func TestTorrentRepoCounterIncrements(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewTorrentRepo(db)
	u := newUser(t, db)
	tor := newTorrent(t, db, u.ID)

	if err := repo.IncrementSeeders(ctx, tor.ID, 2); err != nil {
		t.Fatalf("IncrementSeeders: %v", err)
	}
	if err := repo.IncrementLeechers(ctx, tor.ID, 3); err != nil {
		t.Fatalf("IncrementLeechers: %v", err)
	}
	if err := repo.IncrementTimesCompleted(ctx, tor.ID); err != nil {
		t.Fatalf("IncrementTimesCompleted: %v", err)
	}

	got, err := repo.GetByID(ctx, tor.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Seeders != 2 || got.Leechers != 3 || got.TimesCompleted != 1 {
		t.Errorf("seeders=%d leechers=%d completed=%d, want 2/3/1", got.Seeders, got.Leechers, got.TimesCompleted)
	}

	// A negative delta must decrement, and must not drive the count below zero.
	if err := repo.IncrementSeeders(ctx, tor.ID, -5); err != nil {
		t.Fatalf("IncrementSeeders(-5): %v", err)
	}
	got, err = repo.GetByID(ctx, tor.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Seeders < 0 {
		t.Errorf("seeders = %d — a counter must never go negative", got.Seeders)
	}
}

func TestTorrentRepoListFilters(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewTorrentRepo(db)
	alice := newUser(t, db)
	bob := newUser(t, db)

	a := newTorrent(t, db, alice.ID)
	b := newTorrent(t, db, bob.ID)

	t.Run("lists everything visible", func(t *testing.T) {
		list, total, err := repo.List(ctx, repository.ListTorrentsOptions{Page: 1, PerPage: 10})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if total != 2 || len(list) != 2 {
			t.Errorf("total=%d len=%d, want 2 visible torrents", total, len(list))
		}
	})

	t.Run("filters by uploader", func(t *testing.T) {
		list, _, err := repo.List(ctx, repository.ListTorrentsOptions{
			UploaderID: ptr(alice.ID), Page: 1, PerPage: 10,
		})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(list) != 1 || list[0].ID != a.ID {
			t.Errorf("uploader filter returned %d torrents, want only alice's", len(list))
		}
	})

	t.Run("filters by category", func(t *testing.T) {
		list, _, err := repo.List(ctx, repository.ListTorrentsOptions{
			CategoryID: ptr(b.CategoryID), Page: 1, PerPage: 10,
		})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(list) != 1 || list[0].ID != b.ID {
			t.Errorf("category filter returned %d torrents, want only b", len(list))
		}
	})

	t.Run("searches by name", func(t *testing.T) {
		list, _, err := repo.List(ctx, repository.ListTorrentsOptions{
			Search: a.Name, Page: 1, PerPage: 10,
		})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(list) != 1 || list[0].ID != a.ID {
			t.Errorf("search for %q returned %d torrents, want 1", a.Name, len(list))
		}
	})

	t.Run("MaxSeeders backs the need-seed view", func(t *testing.T) {
		if err := repo.IncrementSeeders(ctx, a.ID, 5); err != nil {
			t.Fatalf("IncrementSeeders: %v", err)
		}
		list, _, err := repo.List(ctx, repository.ListTorrentsOptions{
			MaxSeeders: ptr(0), Page: 1, PerPage: 10,
		})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(list) != 1 || list[0].ID != b.ID {
			t.Errorf("MaxSeeders=0 returned %d torrents, want only the unseeded one", len(list))
		}
	})

	t.Run("CreatedAfter backs today's torrents", func(t *testing.T) {
		list, _, err := repo.List(ctx, repository.ListTorrentsOptions{
			CreatedAfter: ptr(time.Now().Add(-time.Hour)), Page: 1, PerPage: 10,
		})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(list) != 2 {
			t.Errorf("CreatedAfter(1h ago) returned %d, want both freshly created torrents", len(list))
		}

		none, _, err := repo.List(ctx, repository.ListTorrentsOptions{
			CreatedAfter: ptr(time.Now().Add(time.Hour)), Page: 1, PerPage: 10,
		})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(none) != 0 {
			t.Errorf("CreatedAfter(1h ahead) returned %d, want 0", len(none))
		}
	})
}

// A hidden or banned torrent must stay out of the normal listing but remain
// visible to admin callers who ask for it.
func TestTorrentRepoListHidesInvisibleUnlessIncludeHidden(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewTorrentRepo(db)
	u := newUser(t, db)
	tor := newTorrent(t, db, u.ID)

	tor.Visible = false
	if err := repo.Update(ctx, tor); err != nil {
		t.Fatalf("Update: %v", err)
	}

	list, _, err := repo.List(ctx, repository.ListTorrentsOptions{Page: 1, PerPage: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("invisible torrent appeared in the default listing (%d rows)", len(list))
	}

	admin, _, err := repo.List(ctx, repository.ListTorrentsOptions{
		IncludeHidden: true, Page: 1, PerPage: 10,
	})
	if err != nil {
		t.Fatalf("List(IncludeHidden): %v", err)
	}
	if len(admin) != 1 {
		t.Errorf("IncludeHidden returned %d, want the hidden torrent", len(admin))
	}
}

func TestTorrentRepoListByUploader(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewTorrentRepo(db)
	u := newUser(t, db)
	newTorrent(t, db, u.ID)
	newTorrent(t, db, u.ID)

	list, err := repo.ListByUploader(ctx, u.ID, 10)
	if err != nil {
		t.Fatalf("ListByUploader: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("got %d torrents, want 2", len(list))
	}
}
