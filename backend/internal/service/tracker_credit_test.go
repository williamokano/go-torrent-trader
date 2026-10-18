package service

import (
	"context"
	"errors"
	"testing"
)

// A peer_id the tracker has never seen carries no baseline to measure its
// counters against, so its first announce credits nothing — whatever the
// event. Crediting the reported totals let a member claim any upload by
// announcing under a fresh peer_id, and a "stopped" announce did so without
// leaving a peer row behind.
func TestAnnounce_UnknownPeerIsNotCredited(t *testing.T) {
	events := map[string]AnnounceEvent{
		"stopped": EventStopped, "regular": EventEmpty, "completed": EventCompleted, "started": EventStarted,
	}
	for name, ev := range events {
		t.Run(name, func(t *testing.T) {
			svc, userRepo, _, _ := setupTracker()

			for i := 0; i < 3; i++ {
				pid := testPeerID()
				pid[0] = byte(i) // a fresh peer_id every time
				if _, err := svc.Announce(context.Background(), AnnounceRequest{
					Passkey:    testPasskey(),
					InfoHash:   testInfoHash(),
					PeerID:     pid,
					IP:         "192.168.1.1",
					Port:       6881,
					Uploaded:   1 << 50,
					Downloaded: 1 << 40,
					Event:      ev,
				}); err != nil {
					t.Fatalf("announce %d: %v", i, err)
				}
			}

			userRepo.mu.Lock()
			up, down := userRepo.users[0].Uploaded, userRepo.users[0].Downloaded
			userRepo.mu.Unlock()
			if up != 0 || down != 0 {
				t.Errorf("unknown peers credited uploaded=%d downloaded=%d, want 0/0", up, down)
			}
		})
	}
}

// The counters reported on a peer's first announce become its baseline, so
// the next announce is credited only the growth since then.
func TestAnnounce_FirstAnnounceSetsBaseline(t *testing.T) {
	svc, userRepo, _, _ := setupTracker()

	for _, up := range []int64{5000, 7000} {
		if _, err := svc.Announce(context.Background(), AnnounceRequest{
			Passkey:  testPasskey(),
			InfoHash: testInfoHash(),
			PeerID:   testPeerID(),
			IP:       "192.168.1.1",
			Port:     6881,
			Uploaded: up,
			Event:    EventEmpty,
		}); err != nil {
			t.Fatalf("announce: %v", err)
		}
	}

	userRepo.mu.Lock()
	got := userRepo.users[0].Uploaded
	userRepo.mu.Unlock()
	if got != 2000 {
		t.Errorf("uploaded = %d, want 2000 (growth after the baseline only)", got)
	}
}

func TestAnnounce_RejectsNegativeCounters(t *testing.T) {
	cases := map[string]AnnounceRequest{
		"uploaded":   {Uploaded: -1},
		"downloaded": {Downloaded: -1 << 50},
		"left":       {Left: -1},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			svc, userRepo, _, peerRepo := setupTracker()
			req.Passkey, req.InfoHash, req.PeerID = testPasskey(), testInfoHash(), testPeerID()
			req.IP, req.Port, req.Event = "192.168.1.1", 6881, EventEmpty

			if _, err := svc.Announce(context.Background(), req); !errors.Is(err, ErrInvalidCounters) {
				t.Fatalf("err = %v, want ErrInvalidCounters", err)
			}
			userRepo.mu.Lock()
			up, down := userRepo.users[0].Uploaded, userRepo.users[0].Downloaded
			userRepo.mu.Unlock()
			if up != 0 || down != 0 {
				t.Errorf("stats changed to %d/%d on a rejected announce", up, down)
			}
			peerRepo.mu.Lock()
			n := len(peerRepo.peers)
			peerRepo.mu.Unlock()
			if n != 0 {
				t.Errorf("rejected announce stored %d peers", n)
			}
		})
	}
}

// "Could not look the peer up" must not be read as "new peer": that path
// skips cheat detection and the connection limits' existing-peer exemption,
// and would reset the delta baseline.
func TestAnnounce_PeerLookupErrorFailsAnnounce(t *testing.T) {
	svc, userRepo, _, peerRepo := setupTracker()
	peerRepo.lookupErr = errors.New("connection reset")

	_, err := svc.Announce(context.Background(), AnnounceRequest{
		Passkey:  testPasskey(),
		InfoHash: testInfoHash(),
		PeerID:   testPeerID(),
		IP:       "192.168.1.1",
		Port:     6881,
		Uploaded: 1 << 40,
		Event:    EventEmpty,
	})
	if err == nil {
		t.Fatal("announce succeeded despite a failed peer lookup")
	}
	userRepo.mu.Lock()
	up := userRepo.users[0].Uploaded
	userRepo.mu.Unlock()
	if up != 0 {
		t.Errorf("uploaded = %d after a failed lookup, want 0", up)
	}
}
