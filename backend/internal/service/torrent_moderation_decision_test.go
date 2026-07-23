package service

import (
	"context"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

func TestModerationDecision_PublishesEvent(t *testing.T) {
	ctx := context.Background()
	repo := newMemTorrentRepo()
	bus := event.NewInMemoryBus()
	svc := NewTorrentService(nil, repo, newMemUserRepo(), newMemStorage(),
		TorrentServiceConfig{}, bus, nil)
	svc.SetModerationRepo(&fakeModerationRepo{repo: repo})

	var captured *event.TorrentModeratedEvent
	bus.Subscribe(event.TorrentModerated, func(_ context.Context, evt event.Event) error {
		captured = evt.(*event.TorrentModeratedEvent)
		return nil
	})

	t.Run("approve", func(t *testing.T) {
		captured = nil
		tor := &model.Torrent{UploaderID: 5, Name: "Thing", ModerationStatus: model.ModerationPending}
		_ = repo.Create(ctx, tor)
		if _, err := svc.ApproveTorrent(ctx, tor.ID, 9, model.Permissions{IsAdmin: true}); err != nil {
			t.Fatalf("approve: %v", err)
		}
		if captured == nil {
			t.Fatal("no TorrentModeratedEvent published on approve")
		}
		if captured.Decision != model.ModerationApproved {
			t.Errorf("decision = %q, want approved", captured.Decision)
		}
		if captured.UploaderID != 5 || captured.Actor.ID != 9 {
			t.Errorf("uploader=%d actor=%d, want 5 / 9", captured.UploaderID, captured.Actor.ID)
		}
	})

	t.Run("reject", func(t *testing.T) {
		captured = nil
		tor := &model.Torrent{UploaderID: 5, Name: "Thing2", ModerationStatus: model.ModerationPending}
		_ = repo.Create(ctx, tor)
		if _, err := svc.RejectTorrent(ctx, tor.ID, 9, model.Permissions{IsModerator: true}); err != nil {
			t.Fatalf("reject: %v", err)
		}
		if captured == nil || captured.Decision != model.ModerationRejected {
			t.Fatalf("reject event = %+v, want decision rejected", captured)
		}
	})
}
