package service

import (
	"context"
	"errors"
	"testing"
)

// creditReq is an announce for the test user and torrent; callers set only
// the fields under test.
func creditReq(peerID []byte, ev AnnounceEvent, uploaded, downloaded int64) AnnounceRequest {
	return AnnounceRequest{
		Passkey:    testPasskey(),
		InfoHash:   testInfoHash(),
		PeerID:     peerID,
		IP:         "192.168.1.1",
		Port:       6881,
		Uploaded:   uploaded,
		Downloaded: downloaded,
		Event:      ev,
	}
}

// userTotals reads the test user's credited stats.
func userTotals(r *trackerMockUserRepo) (up, down int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.users[0].Uploaded, r.users[0].Downloaded
}

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
				if _, err := svc.Announce(context.Background(), creditReq(pid, ev, 1<<50, 1<<40)); err != nil {
					t.Fatalf("announce %d: %v", i, err)
				}
			}

			if up, down := userTotals(userRepo); up != 0 || down != 0 {
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
		if _, err := svc.Announce(context.Background(), creditReq(testPeerID(), EventEmpty, up, 0)); err != nil {
			t.Fatalf("announce: %v", err)
		}
	}

	if up, _ := userTotals(userRepo); up != 2000 {
		t.Errorf("uploaded = %d, want 2000 (growth after the baseline only)", up)
	}
}

func TestAnnounce_RejectsNegativeCounters(t *testing.T) {
	cases := map[string]func(*AnnounceRequest){
		"uploaded":   func(r *AnnounceRequest) { r.Uploaded = -1 },
		"downloaded": func(r *AnnounceRequest) { r.Downloaded = -1 << 50 },
		"left":       func(r *AnnounceRequest) { r.Left = -1 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			svc, userRepo, _, peerRepo := setupTracker()
			req := creditReq(testPeerID(), EventEmpty, 0, 0)
			mutate(&req)

			if _, err := svc.Announce(context.Background(), req); !errors.Is(err, ErrInvalidCounters) {
				t.Fatalf("err = %v, want ErrInvalidCounters", err)
			}
			if up, down := userTotals(userRepo); up != 0 || down != 0 {
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
// would put an existing peer through the new-peer limits and wait time,
// count it again in the swarm totals, and reset its delta baseline.
func TestAnnounce_PeerLookupErrorFailsAnnounce(t *testing.T) {
	svc, userRepo, _, peerRepo := setupTracker()
	peerRepo.lookupErr = errors.New("connection reset")

	if _, err := svc.Announce(context.Background(), creditReq(testPeerID(), EventEmpty, 1<<40, 0)); err == nil {
		t.Fatal("announce succeeded despite a failed peer lookup")
	}
	if up, _ := userTotals(userRepo); up != 0 {
		t.Errorf("uploaded = %d after a failed lookup, want 0", up)
	}
}

// Counters restart at zero when the client does. A lower total must credit
// nothing rather than a negative delta, and becomes the new baseline.
func TestAnnounce_CounterResetClampsAtZero(t *testing.T) {
	svc, userRepo, _, _ := setupTracker()

	for _, up := range []int64{5000, 100, 300} {
		if _, err := svc.Announce(context.Background(), creditReq(testPeerID(), EventEmpty, up, up)); err != nil {
			t.Fatalf("announce: %v", err)
		}
	}

	if up, down := userTotals(userRepo); up != 200 || down != 200 {
		t.Errorf("credited %d/%d, want 200/200 (only the growth after the reset)", up, down)
	}
}

// A known peer's stopped announce carries its final totals; that last
// stretch of the session is credited like any other announce.
func TestAnnounce_StoppedKnownPeerIsCredited(t *testing.T) {
	svc, userRepo, _, _ := setupTracker()

	if _, err := svc.Announce(context.Background(), creditReq(testPeerID(), EventStarted, 0, 0)); err != nil {
		t.Fatalf("started: %v", err)
	}
	if _, err := svc.Announce(context.Background(), creditReq(testPeerID(), EventStopped, 4000, 1000)); err != nil {
		t.Fatalf("stopped: %v", err)
	}

	if up, down := userTotals(userRepo); up != 4000 || down != 1000 {
		t.Errorf("credited %d/%d, want 4000/1000", up, down)
	}
}

// The swap moves a peer's baseline before the peer row is refreshed. If that
// refresh fails, the traffic the swap measured is still credited — a retry
// would measure nothing and the bytes would be lost.
func TestAnnounce_CreditsSwappedTrafficWhenPeerRefreshFails(t *testing.T) {
	svc, userRepo, _, peerRepo := setupTracker()
	if _, err := svc.Announce(context.Background(), creditReq(testPeerID(), EventStarted, 0, 0)); err != nil {
		t.Fatalf("started: %v", err)
	}
	peerRepo.mu.Lock()
	peerRepo.upsertErr = errors.New("connection reset")
	peerRepo.mu.Unlock()

	if _, err := svc.Announce(context.Background(), creditReq(testPeerID(), EventEmpty, 3000, 0)); err != nil {
		t.Fatalf("announce failed after a successful swap: %v", err)
	}
	if up, _ := userTotals(userRepo); up != 3000 {
		t.Errorf("uploaded = %d, want the swapped 3000 credited", up)
	}
}

// A peer removed between the lookup and the swap has no baseline left: it
// is stored again as a new peer and credited nothing.
func TestAnnounce_PeerReapedBeforeSwapIsNew(t *testing.T) {
	svc, userRepo, torrentRepo, peerRepo := setupTracker()
	if _, err := svc.Announce(context.Background(), creditReq(testPeerID(), EventStarted, 0, 0)); err != nil {
		t.Fatalf("started: %v", err)
	}
	peerRepo.mu.Lock()
	peerRepo.reapBeforeSwap = true
	peerRepo.mu.Unlock()

	if _, err := svc.Announce(context.Background(), creditReq(testPeerID(), EventEmpty, 9000, 0)); err != nil {
		t.Fatalf("announce: %v", err)
	}
	if up, _ := userTotals(userRepo); up != 0 {
		t.Errorf("uploaded = %d, want 0 for a peer with no baseline", up)
	}
	peerRepo.mu.Lock()
	n := len(peerRepo.peers)
	peerRepo.mu.Unlock()
	if n != 1 {
		t.Errorf("peers stored = %d, want the reaped peer stored again", n)
	}
	torrentRepo.mu.Lock()
	defer torrentRepo.mu.Unlock()
	if got := torrentRepo.torrents[0].Seeders + torrentRepo.torrents[0].Leechers; got != 2 {
		t.Errorf("swarm count = %d, want it counted again as a new peer (the reap's decrement is the cleanup job's)", got)
	}
}
