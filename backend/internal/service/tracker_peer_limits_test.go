package service

import (
	"context"
	"errors"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

func addPeers(repo *trackerMockPeerRepo, userID int64, n int, seed byte) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	for i := 0; i < n; i++ {
		pid := make([]byte, 20)
		pid[0], pid[1] = seed, byte(i)
		repo.peers = append(repo.peers, &model.Peer{TorrentID: 1, UserID: userID, PeerID: pid, IP: "10.0.0.1", Port: 6881})
	}
}

func leecherReq() AnnounceRequest {
	return AnnounceRequest{Passkey: testPasskey(), InfoHash: testInfoHash(), PeerID: testPeerID(),
		IP: "192.168.1.1", Port: 6881, Left: 1000, Event: EventStarted}
}

// One member announcing under many peer_ids could fill a torrent and lock
// everyone else out. A member's clients on one torrent are capped.
func TestAnnounce_MemberPeersPerTorrentCapped(t *testing.T) {
	svc, _, _, peerRepo := setupTrackerWithSettings(map[string]string{SettingTrackerMaxPeersPerUserTorrent: "3"})
	addPeers(peerRepo, 1, 3, 200) // the announcing member (user 1) already has three

	if _, err := svc.Announce(context.Background(), leecherReq()); !errors.Is(err, ErrTooManyOwnPeers) {
		t.Fatalf("fourth client err = %v, want ErrTooManyOwnPeers", err)
	}
}

// Seeders are never turned away by the per-torrent cap: refusing one only
// starves the swarm.
func TestAnnounce_TorrentCapNeverRefusesSeeders(t *testing.T) {
	svc, _, torrentRepo, _ := setupTrackerWithSettings(map[string]string{SettingTrackerMaxPeersPerTorrent: "2"})
	torrentRepo.mu.Lock()
	torrentRepo.torrents[0].Leechers = 2 // the cap counts leechers
	torrentRepo.mu.Unlock()

	seed := leecherReq()
	seed.Left = 0
	if _, err := svc.Announce(context.Background(), seed); err != nil {
		t.Fatalf("a seeder was refused by a full torrent: %v", err)
	}
	leecher := leecherReq()
	leecher.PeerID = append([]byte(nil), leecher.PeerID...)
	leecher.PeerID[0] ^= 0xff // another client
	if _, err := svc.Announce(context.Background(), leecher); !errors.Is(err, ErrTooManyPeers) {
		t.Fatalf("a leecher on a full torrent err = %v, want ErrTooManyPeers", err)
	}
}

// With the shipped defaults (no per-torrent cap, three clients per member), a
// member who has piled peers onto a torrent cannot keep another member out.
func TestAnnounce_DefaultsDoNotLetOneMemberLockATorrent(t *testing.T) {
	svc, _, _, peerRepo := setupTrackerWithSettings(map[string]string{})
	addPeers(peerRepo, 99, 50, 203) // someone else, with fifty

	if _, err := svc.Announce(context.Background(), leecherReq()); err != nil {
		t.Fatalf("another member was locked out: %v", err)
	}
}
