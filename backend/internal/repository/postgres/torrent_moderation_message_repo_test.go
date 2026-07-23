package postgres

import (
	"context"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

func TestTorrentModerationMessages_CRUDAndCount(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	repo := NewTorrentModerationMessageRepo(db)
	uploader := newUser(t, db)
	tor := newPendingTorrent(t, db, uploader.ID)

	if n, err := repo.CountByTorrent(ctx, tor.ID); err != nil || n != 0 {
		t.Fatalf("initial count = %d, err %v; want 0", n, err)
	}

	m1 := &model.TorrentModerationMessage{TorrentID: tor.ID, AuthorID: uploader.ID, Body: "first"}
	if err := repo.Create(ctx, m1); err != nil {
		t.Fatalf("Create m1: %v", err)
	}
	if m1.ID == 0 || m1.CreatedAt.IsZero() {
		t.Errorf("Create did not populate id/created_at: %+v", m1)
	}
	m2 := &model.TorrentModerationMessage{TorrentID: tor.ID, AuthorID: uploader.ID, Body: "second"}
	if err := repo.Create(ctx, m2); err != nil {
		t.Fatalf("Create m2: %v", err)
	}

	msgs, err := repo.ListByTorrent(ctx, tor.ID)
	if err != nil {
		t.Fatalf("ListByTorrent: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("len = %d, want 2", len(msgs))
	}
	if msgs[0].Body != "first" || msgs[1].Body != "second" {
		t.Errorf("order = [%q %q], want [first second]", msgs[0].Body, msgs[1].Body)
	}
	if msgs[0].AuthorUsername != uploader.Username {
		t.Errorf("author username = %q, want %q", msgs[0].AuthorUsername, uploader.Username)
	}

	if n, err := repo.CountByTorrent(ctx, tor.ID); err != nil || n != 2 {
		t.Fatalf("count = %d, err %v; want 2", n, err)
	}
}

func TestListModerationQueue_PopulatesMessageCount(t *testing.T) {
	db := requireDB(t)
	resetTestData(t, db)
	ctx := context.Background()

	torrentRepo := NewTorrentRepo(db)
	msgRepo := NewTorrentModerationMessageRepo(db)
	uploader := newUser(t, db)

	withMsgs := newPendingTorrent(t, db, uploader.ID)
	newPendingTorrent(t, db, uploader.ID) // no messages
	for i := 0; i < 3; i++ {
		if err := msgRepo.Create(ctx, &model.TorrentModerationMessage{
			TorrentID: withMsgs.ID, AuthorID: uploader.ID, Body: "note",
		}); err != nil {
			t.Fatalf("seed message: %v", err)
		}
	}

	got, _, err := torrentRepo.ListModerationQueue(ctx, repository.ModerationQueueOptions{
		Assigned: repository.ModAssignedAll,
	})
	if err != nil {
		t.Fatalf("ListModerationQueue: %v", err)
	}

	counts := map[int64]int{}
	for _, tr := range got {
		counts[tr.ID] = tr.MessageCount
	}
	if counts[withMsgs.ID] != 3 {
		t.Errorf("message_count for withMsgs = %d, want 3", counts[withMsgs.ID])
	}
}
