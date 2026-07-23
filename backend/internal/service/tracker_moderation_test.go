package service

import (
	"context"
	"errors"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

func announcePending(svc *TrackerService) error {
	_, err := svc.Announce(context.Background(), AnnounceRequest{
		Passkey:  testPasskey(),
		InfoHash: testInfoHash(),
		PeerID:   testPeerID(),
		IP:       "192.168.1.1",
		Port:     6881,
		Left:     0, // seeder — sidesteps wait-time paths
		Event:    EventStarted,
	})
	return err
}

func TestAnnounce_PendingTorrent(t *testing.T) {
	t.Run("rejects a non-uploader non-staff peer", func(t *testing.T) {
		svc, _, torrentRepo, _ := setupTracker()
		torrentRepo.mu.Lock()
		torrentRepo.torrents[0].ModerationStatus = model.ModerationPending
		torrentRepo.torrents[0].UploaderID = 2 // uploaded by someone else
		torrentRepo.mu.Unlock()

		if err := announcePending(svc); !errors.Is(err, ErrTorrentNotApproved) {
			t.Errorf("err = %v, want ErrTorrentNotApproved", err)
		}
	})

	t.Run("allows the uploader to seed their own pending torrent", func(t *testing.T) {
		svc, _, torrentRepo, _ := setupTracker()
		torrentRepo.mu.Lock()
		torrentRepo.torrents[0].ModerationStatus = model.ModerationPending
		torrentRepo.torrents[0].UploaderID = 1 // the announcing user
		torrentRepo.mu.Unlock()

		if err := announcePending(svc); err != nil {
			t.Errorf("uploader announce: %v", err)
		}
	})

	t.Run("allows staff", func(t *testing.T) {
		svc, userRepo, torrentRepo, _ := setupTracker()
		groupRepo := newTrackerMockGroupRepo()
		groupRepo.addGroup(&model.Group{ID: 5, IsModerator: true})
		svc.SetGroupRepo(groupRepo)
		userRepo.mu.Lock()
		userRepo.users[0].GroupID = 5
		userRepo.mu.Unlock()
		torrentRepo.mu.Lock()
		torrentRepo.torrents[0].ModerationStatus = model.ModerationPending
		torrentRepo.torrents[0].UploaderID = 2 // not the moderator
		torrentRepo.mu.Unlock()

		if err := announcePending(svc); err != nil {
			t.Errorf("staff announce: %v", err)
		}
	})
}
