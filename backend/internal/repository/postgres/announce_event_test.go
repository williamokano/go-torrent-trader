package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

func TestAnnounceEventRepo_CreateAndListByUser(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewAnnounceEventRepo(db)
	user := newUser(t, db)
	other := newUser(t, db)
	torrent := newTorrent(t, db, user.ID)

	// Two announces for `user` on the same torrent — an append-only log keeps
	// both, unlike transfer_history which would overwrite.
	base := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	first := &model.AnnounceEvent{
		UserID: user.ID, TorrentID: &torrent.ID, PeerID: []byte("peer-aaaaaaaaaaaaaa"),
		IP: "10.0.0.1", Port: 6881, Event: "started",
		Uploaded: 0, Downloaded: 0, LeftBytes: 1000,
		UploadedDelta: 0, DownloadedDelta: 0, CountedDownloadedDelta: 0,
		Seeder: false, AnnouncedAt: base,
	}
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("Create(first): %v", err)
	}
	if first.ID == 0 {
		t.Fatal("Create did not populate the generated ID")
	}

	second := &model.AnnounceEvent{
		UserID: user.ID, TorrentID: &torrent.ID, PeerID: []byte("peer-aaaaaaaaaaaaaa"),
		IP: "10.0.0.1", Port: 6881, Event: "completed",
		Uploaded: 500, Downloaded: 1000, LeftBytes: 0,
		UploadedDelta: 500, DownloadedDelta: 1000, CountedDownloadedDelta: 500,
		Seeder: true, AnnouncedAt: base.Add(30 * time.Minute),
	}
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("Create(second): %v", err)
	}

	// A different user's event must not leak into `user`'s history.
	if err := repo.Create(ctx, &model.AnnounceEvent{
		UserID: other.ID, TorrentID: &torrent.ID, PeerID: []byte("peer-bbbbbbbbbbbbbb"),
		IP: "10.0.0.2", Port: 6882, Event: "announce", AnnouncedAt: base,
	}); err != nil {
		t.Fatalf("Create(other): %v", err)
	}

	events, total, err := repo.ListByUser(ctx, user.ID, 1, 25)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if total != 2 {
		t.Fatalf("total = %d, want 2 (other user's event must be excluded)", total)
	}
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}

	// Newest first: the completed event was 30 min after the started one.
	if events[0].Event != "completed" {
		t.Errorf("events[0].Event = %q, want completed (newest first)", events[0].Event)
	}
	got := events[0]
	if got.CountedDownloadedDelta != 500 || got.DownloadedDelta != 1000 {
		t.Errorf("deltas not persisted: counted=%d raw=%d", got.CountedDownloadedDelta, got.DownloadedDelta)
	}
	if !got.Seeder || got.LeftBytes != 0 {
		t.Errorf("seeder=%v left=%d, want true/0", got.Seeder, got.LeftBytes)
	}
	if got.TorrentName != torrent.Name {
		t.Errorf("TorrentName = %q, want %q", got.TorrentName, torrent.Name)
	}
	if got.IP != "10.0.0.1" {
		t.Errorf("IP = %q, want 10.0.0.1", got.IP)
	}
}

func TestAnnounceEventRepo_ListByUserPagination(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewAnnounceEventRepo(db)
	user := newUser(t, db)
	torrent := newTorrent(t, db, user.ID)

	base := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	for i := 0; i < 5; i++ {
		if err := repo.Create(ctx, &model.AnnounceEvent{
			UserID: user.ID, TorrentID: &torrent.ID, PeerID: []byte("peer-aaaaaaaaaaaaaa"),
			IP: "10.0.0.1", Port: 6881, Event: "announce",
			AnnouncedAt: base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("Create(%d): %v", i, err)
		}
	}

	page1, total, err := repo.ListByUser(ctx, user.ID, 1, 2)
	if err != nil {
		t.Fatalf("ListByUser(page1): %v", err)
	}
	if total != 5 {
		t.Fatalf("total = %d, want 5", total)
	}
	if len(page1) != 2 {
		t.Fatalf("len(page1) = %d, want 2", len(page1))
	}

	page3, _, err := repo.ListByUser(ctx, user.ID, 3, 2)
	if err != nil {
		t.Fatalf("ListByUser(page3): %v", err)
	}
	if len(page3) != 1 {
		t.Fatalf("len(page3) = %d, want 1 (5 rows, 2 per page)", len(page3))
	}
}

// TestAnnounceEventRepo_TorrentDeletionKeepsEvent verifies the ON DELETE SET NULL
// behaviour: deleting a torrent preserves the user's announce history.
func TestAnnounceEventRepo_TorrentDeletionKeepsEvent(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewAnnounceEventRepo(db)
	user := newUser(t, db)
	torrent := newTorrent(t, db, user.ID)

	if err := repo.Create(ctx, &model.AnnounceEvent{
		UserID: user.ID, TorrentID: &torrent.ID, PeerID: []byte("peer-aaaaaaaaaaaaaa"),
		IP: "10.0.0.1", Port: 6881, Event: "completed", AnnouncedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := NewTorrentRepo(db).Delete(ctx, torrent.ID); err != nil {
		t.Fatalf("deleting torrent: %v", err)
	}

	events, total, err := repo.ListByUser(ctx, user.ID, 1, 25)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if total != 1 || len(events) != 1 {
		t.Fatalf("event lost after torrent deletion: total=%d len=%d", total, len(events))
	}
	if events[0].TorrentID != nil {
		t.Errorf("TorrentID = %v, want nil after torrent deletion", *events[0].TorrentID)
	}
	if events[0].TorrentName != "Deleted Torrent" {
		t.Errorf("TorrentName = %q, want 'Deleted Torrent'", events[0].TorrentName)
	}
}
