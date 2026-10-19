package service

import (
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
