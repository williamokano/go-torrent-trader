package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

type fakeModerationMessageRepo struct {
	msgs   []model.TorrentModerationMessage
	nextID int64
}

func (f *fakeModerationMessageRepo) Create(_ context.Context, m *model.TorrentModerationMessage) error {
	f.nextID++
	m.ID = f.nextID
	m.CreatedAt = time.Now()
	f.msgs = append(f.msgs, *m)
	return nil
}

func (f *fakeModerationMessageRepo) ListByTorrent(_ context.Context, torrentID int64) ([]model.TorrentModerationMessage, error) {
	var out []model.TorrentModerationMessage
	for _, m := range f.msgs {
		if m.TorrentID == torrentID {
			out = append(out, m)
		}
	}
	return out, nil
}

func (f *fakeModerationMessageRepo) CountByTorrent(_ context.Context, torrentID int64) (int, error) {
	n := 0
	for _, m := range f.msgs {
		if m.TorrentID == torrentID {
			n++
		}
	}
	return n, nil
}

func newSvcWithMessages() (*TorrentService, *memTorrentRepo, *memUserRepo, *fakeModerationMessageRepo, event.Bus) {
	repo := newMemTorrentRepo()
	userRepo := newMemUserRepo()
	bus := event.NewInMemoryBus()
	svc := NewTorrentService(nil, repo, userRepo, newMemStorage(), TorrentServiceConfig{}, bus, nil)
	msgRepo := &fakeModerationMessageRepo{}
	svc.SetModerationMessageRepo(msgRepo)
	return svc, repo, userRepo, msgRepo, bus
}

func TestModerationMessages_Authorization(t *testing.T) {
	ctx := context.Background()
	svc, repo, _, _, _ := newSvcWithMessages()
	tor := &model.Torrent{UploaderID: 5, ModerationStatus: model.ModerationPending}
	_ = repo.Create(ctx, tor)

	// Uploader may post + read.
	if _, err := svc.PostModerationMessage(ctx, tor.ID, 5, model.Permissions{}, "please fix the nfo"); err != nil {
		t.Fatalf("uploader post: %v", err)
	}
	if msgs, err := svc.ListModerationMessages(ctx, tor.ID, 5, model.Permissions{}); err != nil || len(msgs) != 1 {
		t.Fatalf("uploader list = %d msgs, err %v; want 1", len(msgs), err)
	}

	// Staff may post.
	if _, err := svc.PostModerationMessage(ctx, tor.ID, 9, model.Permissions{IsModerator: true}, "on it"); err != nil {
		t.Fatalf("staff post: %v", err)
	}

	// A random member may neither read nor post.
	if _, err := svc.ListModerationMessages(ctx, tor.ID, 99, model.Permissions{}); !errors.Is(err, ErrForbidden) {
		t.Errorf("stranger list err = %v, want ErrForbidden", err)
	}
	if _, err := svc.PostModerationMessage(ctx, tor.ID, 99, model.Permissions{}, "hi"); !errors.Is(err, ErrForbidden) {
		t.Errorf("stranger post err = %v, want ErrForbidden", err)
	}
}

func TestModerationMessages_EmptyBodyAndUnavailable(t *testing.T) {
	ctx := context.Background()
	svc, repo, _, _, _ := newSvcWithMessages()
	tor := &model.Torrent{UploaderID: 5, ModerationStatus: model.ModerationPending}
	_ = repo.Create(ctx, tor)

	if _, err := svc.PostModerationMessage(ctx, tor.ID, 5, model.Permissions{}, "   "); !errors.Is(err, ErrEmptyMessage) {
		t.Errorf("blank body err = %v, want ErrEmptyMessage", err)
	}

	// No message repo wired.
	bare := NewTorrentService(nil, repo, newMemUserRepo(), newMemStorage(), TorrentServiceConfig{}, event.NewInMemoryBus(), nil)
	if _, err := bare.ListModerationMessages(ctx, tor.ID, 5, model.Permissions{IsAdmin: true}); !errors.Is(err, ErrModerationUnavailable) {
		t.Errorf("unavailable list err = %v, want ErrModerationUnavailable", err)
	}
}

func TestModerationMessages_PublishesEventToUploaderAndModerator(t *testing.T) {
	ctx := context.Background()
	svc, repo, _, _, bus := newSvcWithMessages()

	var captured *event.TorrentModerationMessagePostedEvent
	bus.Subscribe(event.TorrentModerationMsg, func(_ context.Context, evt event.Event) error {
		captured = evt.(*event.TorrentModerationMessagePostedEvent)
		return nil
	})

	modID := int64(8)
	tor := &model.Torrent{
		UploaderID:          5,
		Name:                "Thing",
		ModerationStatus:    model.ModerationPending,
		AssignedModeratorID: &modID,
	}
	_ = repo.Create(ctx, tor)

	if _, err := svc.PostModerationMessage(ctx, tor.ID, 8, model.Permissions{IsModerator: true}, "needs work"); err != nil {
		t.Fatalf("post: %v", err)
	}

	if captured == nil {
		t.Fatal("no event published")
	}
	if captured.UploaderID != 5 {
		t.Errorf("event UploaderID = %d, want 5", captured.UploaderID)
	}
	if captured.AssignedModeratorID == nil || *captured.AssignedModeratorID != 8 {
		t.Errorf("event AssignedModeratorID = %v, want 8", captured.AssignedModeratorID)
	}
	if captured.Actor.ID != 8 {
		t.Errorf("event actor = %d, want 8", captured.Actor.ID)
	}
}
