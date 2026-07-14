package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

func newPeer(t *testing.T, db *sql.DB, torrentID, userID int64, seeder bool) *model.Peer {
	t.Helper()

	now := time.Now()
	p := &model.Peer{
		TorrentID: torrentID,
		UserID:    userID,
		PeerID:    infoHash(uniq("peer")),
		IP:        "10.0.0.1",
		Port:      6881,
		Seeder:    seeder,
		// Upsert stores these verbatim — the tracker sets them on each announce,
		// so a fixture that leaves them zero makes every peer look ancient.
		StartedAt:    now,
		LastAnnounce: now,
	}
	if !seeder {
		p.LeftBytes = 512 // a leecher still has bytes outstanding
	}
	if err := NewPeerRepo(db).Upsert(context.Background(), p); err != nil {
		t.Fatalf("upserting peer: %v", err)
	}
	return p
}

func TestPeerRepoUpsertInsertsThenUpdates(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewPeerRepo(db)
	u := newUser(t, db)
	tor := newTorrent(t, db, u.ID)

	p := newPeer(t, db, tor.ID, u.ID, false)

	got, err := repo.GetByTorrentAndUser(ctx, tor.ID, u.ID)
	if err != nil {
		t.Fatalf("GetByTorrentAndUser: %v", err)
	}
	if got.Port != 6881 {
		t.Errorf("port = %d, want 6881", got.Port)
	}

	// Re-announcing the same peer must update the existing row, not add a second.
	p.Uploaded = 4096
	p.Seeder = true
	p.LeftBytes = 0
	if err := repo.Upsert(ctx, p); err != nil {
		t.Fatalf("Upsert (second announce): %v", err)
	}

	count, err := repo.CountByTorrent(ctx, tor.ID)
	if err != nil {
		t.Fatalf("CountByTorrent: %v", err)
	}
	if count != 1 {
		t.Errorf("peer rows = %d, want 1 — a re-announce must upsert, not duplicate", count)
	}

	got, err = repo.GetByTorrentUserAndPeerID(ctx, tor.ID, u.ID, p.PeerID)
	if err != nil {
		t.Fatalf("GetByTorrentUserAndPeerID: %v", err)
	}
	if got.Uploaded != 4096 || !got.Seeder {
		t.Errorf("uploaded=%d seeder=%v, want the announce to have been applied", got.Uploaded, got.Seeder)
	}
}

func TestPeerRepoCountByUserSplitsSeedingAndLeeching(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewPeerRepo(db)
	u := newUser(t, db)

	seeding := newTorrent(t, db, u.ID)
	leeching := newTorrent(t, db, u.ID)
	newPeer(t, db, seeding.ID, u.ID, true)
	newPeer(t, db, leeching.ID, u.ID, false)

	seed, leech, err := repo.CountByUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("CountByUser: %v", err)
	}
	if seed != 1 || leech != 1 {
		t.Errorf("seeding=%d leeching=%d, want 1/1", seed, leech)
	}

	total, err := repo.CountTotalByUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("CountTotalByUser: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
}

func TestPeerRepoListByTorrentAndUser(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewPeerRepo(db)
	u := newUser(t, db)
	seeding := newTorrent(t, db, u.ID)
	leeching := newTorrent(t, db, u.ID)
	newPeer(t, db, seeding.ID, u.ID, true)
	newPeer(t, db, leeching.ID, u.ID, false)

	peers, err := repo.ListByTorrent(ctx, seeding.ID, 50)
	if err != nil {
		t.Fatalf("ListByTorrent: %v", err)
	}
	if len(peers) != 1 {
		t.Errorf("ListByTorrent returned %d, want 1", len(peers))
	}

	seeds, total, err := repo.ListByUserSeeding(ctx, u.ID, 1, 10)
	if err != nil {
		t.Fatalf("ListByUserSeeding: %v", err)
	}
	if total != 1 || len(seeds) != 1 || seeds[0].TorrentName != seeding.Name {
		t.Errorf("seeding list = %d rows (total %d), want the seeding torrent by name", len(seeds), total)
	}

	leeches, total, err := repo.ListByUserLeeching(ctx, u.ID, 1, 10)
	if err != nil {
		t.Fatalf("ListByUserLeeching: %v", err)
	}
	if total != 1 || len(leeches) != 1 || leeches[0].TorrentName != leeching.Name {
		t.Errorf("leeching list = %d rows (total %d), want the leeching torrent by name", len(leeches), total)
	}
}

func TestPeerRepoDelete(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewPeerRepo(db)
	u := newUser(t, db)
	tor := newTorrent(t, db, u.ID)
	p := newPeer(t, db, tor.ID, u.ID, true)

	if err := repo.Delete(ctx, tor.ID, u.ID, p.PeerID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	count, err := repo.CountByTorrent(ctx, tor.ID)
	if err != nil {
		t.Fatalf("CountByTorrent: %v", err)
	}
	if count != 0 {
		t.Errorf("peers = %d after Delete, want 0", count)
	}
}

// DeleteStale reaps peers that stopped announcing. It must remove only those
// past the cutoff — a peer that announced recently is still live.
func TestPeerRepoDeleteStaleOnlyReapsOldPeers(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewPeerRepo(db)
	u := newUser(t, db)
	fresh := newTorrent(t, db, u.ID)
	stale := newTorrent(t, db, u.ID)

	newPeer(t, db, fresh.ID, u.ID, true)
	stalePeer := newPeer(t, db, stale.ID, u.ID, true)

	// Backdate the stale peer's last announce.
	if _, err := db.Exec(
		`UPDATE peers SET last_announce = $1 WHERE torrent_id = $2`,
		time.Now().Add(-2*time.Hour), stalePeer.TorrentID,
	); err != nil {
		t.Fatalf("backdating peer: %v", err)
	}

	deleted, err := repo.DeleteStale(ctx, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("DeleteStale: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted %d peers, want 1", deleted)
	}

	if n, _ := repo.CountByTorrent(ctx, fresh.ID); n != 1 {
		t.Errorf("fresh torrent has %d peers, want 1 — a live peer must not be reaped", n)
	}
	if n, _ := repo.CountByTorrent(ctx, stale.ID); n != 0 {
		t.Errorf("stale torrent has %d peers, want 0", n)
	}
}

// --- categories -------------------------------------------------------------

func TestCategoryRepoCRUD(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewCategoryRepo(db)

	cat := &model.Category{Name: "Movies", Slug: "movies", SortOrder: 1}
	if err := repo.Create(ctx, cat); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if cat.ID == 0 {
		t.Fatal("Create did not populate ID")
	}

	got, err := repo.GetByID(ctx, cat.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "Movies" || got.Slug != "movies" {
		t.Errorf("got %q/%q, want Movies/movies", got.Name, got.Slug)
	}

	cat.Name = "Films"
	if err := repo.Update(ctx, cat); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err = repo.GetByID(ctx, cat.ID)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if got.Name != "Films" {
		t.Errorf("name = %q, want Films", got.Name)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) == 0 {
		t.Error("List returned nothing")
	}

	if err := repo.Delete(ctx, cat.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.GetByID(ctx, cat.ID); err == nil {
		t.Error("GetByID succeeded after Delete")
	}
}

// The count backs the guard that stops a category being deleted while torrents
// still reference it.
func TestCategoryRepoCountTorrentsByCategory(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewCategoryRepo(db)
	u := newUser(t, db)
	tor := newTorrent(t, db, u.ID)

	n, err := repo.CountTorrentsByCategory(ctx, tor.CategoryID)
	if err != nil {
		t.Fatalf("CountTorrentsByCategory: %v", err)
	}
	if n != 1 {
		t.Errorf("count = %d, want 1", n)
	}

	empty := newCategoryID(t, db)
	n, err = repo.CountTorrentsByCategory(ctx, empty)
	if err != nil {
		t.Fatalf("CountTorrentsByCategory (empty): %v", err)
	}
	if n != 0 {
		t.Errorf("count = %d for an unused category, want 0", n)
	}
}
