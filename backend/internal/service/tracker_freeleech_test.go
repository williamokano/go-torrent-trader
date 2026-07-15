package service

import (
	"context"
	"testing"
)

// Freeleech accounting: the Free and Silver flags on a torrent discount what
// counts against the leecher's ratio. Free counts no download, Silver counts
// half, and upload is never touched. The flags existed on the model but were
// dead until wired into the announce path.
func TestAnnounce_FreeleechAccounting(t *testing.T) {
	tests := []struct {
		name         string
		free         bool
		silver       bool
		reportedDown int64
		wantCounted  int64
	}{
		{"normal torrent counts full download", false, false, 1000, 1000},
		{"free torrent counts no download", true, false, 1000, 0},
		{"silver torrent counts half download", false, true, 1000, 500},
		{"free wins when both flags are set", true, true, 1000, 0},
		{"silver rounds down on an odd byte count", false, true, 1001, 500},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, userRepo, torrentRepo, peerRepo := setupTracker()

			torrentRepo.mu.Lock()
			torrentRepo.torrents[0].Free = tc.free
			torrentRepo.torrents[0].Silver = tc.silver
			torrentRepo.mu.Unlock()

			// A first-time peer with an empty event takes the reported totals as
			// the delta, giving a known download to account.
			if _, err := svc.Announce(context.Background(), AnnounceRequest{
				Passkey:    testPasskey(),
				InfoHash:   testInfoHash(),
				PeerID:     testPeerID(),
				IP:         "192.168.1.1",
				Port:       6881,
				Uploaded:   500,
				Downloaded: tc.reportedDown,
				Left:       1, // still leeching
				Event:      EventEmpty,
			}); err != nil {
				t.Fatalf("announce: %v", err)
			}

			userRepo.mu.Lock()
			gotDown := userRepo.users[0].Downloaded
			gotUp := userRepo.users[0].Uploaded
			userRepo.mu.Unlock()

			if gotDown != tc.wantCounted {
				t.Errorf("counted download = %d, want %d", gotDown, tc.wantCounted)
			}
			// Upload is never discounted by freeleech.
			if gotUp != 500 {
				t.Errorf("counted upload = %d, want 500 — freeleech must not touch upload", gotUp)
			}

			// The peer row must store the RAW reported download. If freeleech
			// were applied before persisting the peer, the next announce would
			// compute its delta against a discounted baseline and undercount.
			peerRepo.mu.Lock()
			var peerDown int64 = -1
			for _, p := range peerRepo.peers {
				peerDown = p.Downloaded
			}
			peerRepo.mu.Unlock()
			if peerDown != tc.reportedDown {
				t.Errorf("peer stored downloaded = %d, want the raw %d", peerDown, tc.reportedDown)
			}
		})
	}
}

// The discount applies to the delta on each announce, not the lifetime total:
// a leecher's second announce on a silver torrent counts half of only the new
// bytes since the previous one.
func TestAnnounce_FreeleechAppliesToDelta(t *testing.T) {
	svc, userRepo, torrentRepo, _ := setupTracker()

	torrentRepo.mu.Lock()
	torrentRepo.torrents[0].Silver = true
	torrentRepo.mu.Unlock()

	base := AnnounceRequest{
		Passkey:  testPasskey(),
		InfoHash: testInfoHash(),
		PeerID:   testPeerID(),
		IP:       "192.168.1.1",
		Port:     6881,
		Left:     1,
	}

	first := base
	first.Event = EventStarted
	first.Downloaded = 1000
	if _, err := svc.Announce(context.Background(), first); err != nil {
		t.Fatalf("first announce: %v", err)
	}
	// started with an existing-peer-less state records the baseline but counts
	// nothing yet; the delta arrives on the next announce.

	second := base
	second.Event = EventEmpty
	second.Downloaded = 3000 // 2000 new bytes since the baseline
	if _, err := svc.Announce(context.Background(), second); err != nil {
		t.Fatalf("second announce: %v", err)
	}

	userRepo.mu.Lock()
	got := userRepo.users[0].Downloaded
	userRepo.mu.Unlock()

	// Half of the 2000-byte delta.
	if got != 1000 {
		t.Errorf("counted download = %d, want 1000 (half of the 2000-byte delta)", got)
	}
}
