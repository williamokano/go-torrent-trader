package service

import (
	"context"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// What an edit or delete of a torrent may show members.
func TestTorrentPrivacy(t *testing.T) {
	const uploader, staff = 5, 9
	public := model.Torrent{UploaderID: uploader, ModerationStatus: model.ModerationApproved, Visible: true}
	cases := []struct {
		name  string
		mod   func(*model.Torrent)
		actor int64
		want  event.TorrentPrivacy
	}{
		{"public, named uploader", func(*model.Torrent) {}, uploader, event.TorrentPrivacy{}},
		{"anonymous uploader acting", func(t *model.Torrent) { t.Anonymous = true }, uploader, event.TorrentPrivacy{AnonymousActor: true}},
		{"staff acting on an anonymous torrent", func(t *model.Torrent) { t.Anonymous = true }, staff, event.TorrentPrivacy{}},
		{"awaiting moderation", func(t *model.Torrent) { t.ModerationStatus = model.ModerationPending }, uploader, event.TorrentPrivacy{Hidden: true}},
		{"rejected", func(t *model.Torrent) { t.ModerationStatus = model.ModerationRejected }, staff, event.TorrentPrivacy{Hidden: true}},
		{"hidden", func(t *model.Torrent) { t.Visible = false }, staff, event.TorrentPrivacy{Hidden: true}},
		{"banned stays public: naming bans is the log's job", func(t *model.Torrent) { t.Banned = true }, staff, event.TorrentPrivacy{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tor := public
			tc.mod(&tor)
			if got := torrentPrivacy(&tor, tc.actor); got != tc.want {
				t.Errorf("torrentPrivacy = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// An approved torrent unbanned is public again; it is published then, or it
// would never reach the log or the connectors.
func TestEditTorrentUnbanPublishes(t *testing.T) {
	ctx := context.Background()
	h := newPublishedHarness(t)
	h.setModeration(t, false)
	tor, err := h.svc.Upload(ctx, buildTorrentFile("Back.Again"), UploadTorrentRequest{Name: "Back.Again", CategoryID: 1}, 5)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	admin := model.Permissions{IsAdmin: true}
	banned, unbanned := true, false
	if _, err := h.svc.EditTorrent(ctx, tor.ID, 9, admin, EditTorrentRequest{Banned: &banned}); err != nil {
		t.Fatalf("ban: %v", err)
	}
	before := len(h.events)
	if _, err := h.svc.EditTorrent(ctx, tor.ID, 9, admin, EditTorrentRequest{Banned: &unbanned}); err != nil {
		t.Fatalf("unban: %v", err)
	}
	if len(h.events) != before+1 {
		t.Fatalf("unban published %d events, want 1", len(h.events)-before)
	}
}

// An edit that makes a torrent anonymous says so, so the log can hide what
// already named its uploader.
func TestEditTorrentFlagsBecomingAnonymous(t *testing.T) {
	ctx := context.Background()
	h := newPublishedHarness(t)
	h.setModeration(t, false)
	var edits []*event.TorrentEditedEvent
	h.svc.eventBus.Subscribe(event.TorrentEdited, func(_ context.Context, evt event.Event) error {
		edits = append(edits, evt.(*event.TorrentEditedEvent))
		return nil
	})
	tor, err := h.svc.Upload(ctx, buildTorrentFile("Turn.Anon"), UploadTorrentRequest{Name: "Turn.Anon", CategoryID: 1}, 5)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	anon := true
	for i := 0; i < 2; i++ { // the second edit changes nothing about anonymity
		if _, err := h.svc.EditTorrent(ctx, tor.ID, 5, model.Permissions{}, EditTorrentRequest{Anonymous: &anon}); err != nil {
			t.Fatalf("edit: %v", err)
		}
	}
	if len(edits) != 2 || !edits[0].BecameAnonymous || edits[1].BecameAnonymous {
		t.Errorf("BecameAnonymous on the two edits = %v/%v, want true/false", edits[0].BecameAnonymous, edits[1].BecameAnonymous)
	}
}
