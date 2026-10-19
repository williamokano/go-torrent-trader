package service

import (
	"context"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// An upload awaiting moderation is logged for staff only; approval is what
// puts it in front of members, still credited to "Anonymous" when the
// uploader asked for that.
func TestUploadEventPrivacyFollowsModeration(t *testing.T) {
	ctx := context.Background()
	h := newPublishedHarness(t)
	h.setModeration(t, true)
	var uploads []*event.TorrentUploadedEvent
	h.svc.eventBus.Subscribe(event.TorrentUploaded, func(_ context.Context, evt event.Event) error {
		uploads = append(uploads, evt.(*event.TorrentUploadedEvent))
		return nil
	})

	tor, err := h.svc.Upload(ctx, buildTorrentFile("Quiet.Upload"), UploadTorrentRequest{
		Name: "Quiet.Upload", CategoryID: 1, Anonymous: true,
	}, 5)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if len(uploads) != 1 || !uploads[0].Hidden || !uploads[0].AnonymousActor {
		t.Fatalf("pending upload event = %+v, want hidden and anonymous", uploads)
	}

	if _, err := h.svc.ApproveTorrent(ctx, tor.ID, 9, model.Permissions{IsAdmin: true}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if len(uploads) != 2 {
		t.Fatalf("got %d upload events, want a public one on approval", len(uploads))
	}
	public := uploads[1]
	if public.Hidden || !public.AnonymousActor || public.Actor.ID != 5 {
		t.Errorf("approval event = hidden=%v anonymous=%v actor=%d, want public, anonymous, the uploader",
			public.Hidden, public.AnonymousActor, public.Actor.ID)
	}
}
