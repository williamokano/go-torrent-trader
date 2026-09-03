package service

import (
	"context"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// setupTrackerWithHnR builds a tracker with HnR enabled and a fakeHnRRepo
// wired in, mirroring setupTrackerWithSettings.
func setupTrackerWithHnR(extraSettings map[string]string) (*TrackerService, *trackerMockTorrentRepo, *fakeHnRRepo) {
	settings := map[string]string{SettingHnREnabled: "true"}
	for k, v := range extraSettings {
		settings[k] = v
	}
	svc, _, torrentRepo, _ := setupTrackerWithSettings(settings)
	hnrRepo := newFakeHnRRepo()
	svc.SetHnRRepo(hnrRepo)
	return svc, torrentRepo, hnrRepo
}

func completeDownload(t *testing.T, svc *TrackerService, uploaded, downloaded int64) {
	t.Helper()
	if _, err := svc.Announce(context.Background(), AnnounceRequest{
		Passkey: testPasskey(), InfoHash: testInfoHash(), PeerID: testPeerID(),
		IP: "192.168.1.1", Port: 6881, Left: 1000, Event: EventStarted,
	}); err != nil {
		t.Fatalf("start announce: %v", err)
	}
	if _, err := svc.Announce(context.Background(), AnnounceRequest{
		Passkey: testPasskey(), InfoHash: testInfoHash(), PeerID: testPeerID(),
		IP: "192.168.1.1", Port: 6881, Uploaded: uploaded, Downloaded: downloaded,
		Left: 0, Event: EventCompleted,
	}); err != nil {
		t.Fatalf("completed announce: %v", err)
	}
}

func TestAnnounce_HnR_CompletedEventCreatesRecord(t *testing.T) {
	svc, _, hnrRepo := setupTrackerWithHnR(nil)
	completeDownload(t, svc, 500, 1000)

	records, err := hnrRepo.ListForUser(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 hnr record after completion, got %d", len(records))
	}
	if records[0].TorrentID != 1 || records[0].State != model.HnRStateActive {
		t.Errorf("unexpected record: %+v", records[0])
	}
}

func TestAnnounce_HnR_RepeatedCompletedEventDoesNotDuplicate(t *testing.T) {
	svc, _, hnrRepo := setupTrackerWithHnR(nil)
	completeDownload(t, svc, 500, 1000)

	// A second completed event for the same peer (some clients resend it).
	if _, err := svc.Announce(context.Background(), AnnounceRequest{
		Passkey: testPasskey(), InfoHash: testInfoHash(), PeerID: testPeerID(),
		IP: "192.168.1.1", Port: 6881, Uploaded: 600, Downloaded: 1000,
		Left: 0, Event: EventCompleted,
	}); err != nil {
		t.Fatalf("second completed announce: %v", err)
	}

	records, err := hnrRepo.ListForUser(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected exactly 1 hnr record, got %d", len(records))
	}
}

func TestAnnounce_HnR_LeecherToSeederTransitionCreatesRecord(t *testing.T) {
	// A client that never sends a "completed" event: it starts as a leecher,
	// then eventually announces with left=0 directly. The transition itself
	// must establish the obligation as a fallback.
	svc, _, hnrRepo := setupTrackerWithHnR(nil)

	if _, err := svc.Announce(context.Background(), AnnounceRequest{
		Passkey: testPasskey(), InfoHash: testInfoHash(), PeerID: testPeerID(),
		IP: "192.168.1.1", Port: 6881, Left: 1000, Event: EventStarted,
	}); err != nil {
		t.Fatalf("start announce: %v", err)
	}
	// A regular (non-completed) announce that now reports left=0.
	if _, err := svc.Announce(context.Background(), AnnounceRequest{
		Passkey: testPasskey(), InfoHash: testInfoHash(), PeerID: testPeerID(),
		IP: "192.168.1.1", Port: 6881, Uploaded: 100, Downloaded: 1000, Left: 0,
	}); err != nil {
		t.Fatalf("transition announce: %v", err)
	}

	records, err := hnrRepo.ListForUser(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected the leecher->seeder transition to create a record, got %d records", len(records))
	}
}

func TestAnnounce_HnR_SeedingAnnouncesAccumulate(t *testing.T) {
	svc, _, hnrRepo := setupTrackerWithHnR(nil)
	completeDownload(t, svc, 0, 1000)

	// A follow-up keep-alive seeding announce with more uploaded bytes.
	if _, err := svc.Announce(context.Background(), AnnounceRequest{
		Passkey: testPasskey(), InfoHash: testInfoHash(), PeerID: testPeerID(),
		IP: "192.168.1.1", Port: 6881, Uploaded: 700, Downloaded: 1000, Left: 0,
	}); err != nil {
		t.Fatalf("keep-alive announce: %v", err)
	}

	records, err := hnrRepo.ListForUser(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	// The delta between 700 and the 0 already reported at completion is 700.
	if records[0].Uploaded != 700 {
		t.Errorf("expected accumulated upload delta of 700, got %d", records[0].Uploaded)
	}
	if hnrRepo.accumulateCalls < 2 {
		t.Errorf("expected Accumulate to be called for both seeding announces, got %d calls", hnrRepo.accumulateCalls)
	}
}

func TestAnnounce_HnR_StoppedEventDoesNotAccumulate(t *testing.T) {
	svc, _, hnrRepo := setupTrackerWithHnR(nil)
	completeDownload(t, svc, 0, 1000)
	callsAfterComplete := hnrRepo.accumulateCalls

	if _, err := svc.Announce(context.Background(), AnnounceRequest{
		Passkey: testPasskey(), InfoHash: testInfoHash(), PeerID: testPeerID(),
		IP: "192.168.1.1", Port: 6881, Uploaded: 999, Downloaded: 1000, Left: 0,
		Event: EventStopped,
	}); err != nil {
		t.Fatalf("stopped announce: %v", err)
	}

	if hnrRepo.accumulateCalls != callsAfterComplete {
		t.Errorf("a stopped announce must never accumulate seed time: calls went from %d to %d",
			callsAfterComplete, hnrRepo.accumulateCalls)
	}
}

func TestAnnounce_HnR_DisabledFeatureTouchesNothing(t *testing.T) {
	svc, _, torrentRepo, _ := setupTrackerWithSettings(map[string]string{SettingHnREnabled: "false"})
	hnrRepo := newFakeHnRRepo()
	svc.SetHnRRepo(hnrRepo)
	_ = torrentRepo

	completeDownload(t, svc, 500, 1000)

	if hnrRepo.createCalls != 0 {
		t.Errorf("expected 0 CreateIfNotExists calls with hnr_enabled=false, got %d", hnrRepo.createCalls)
	}
	if hnrRepo.accumulateCalls != 0 {
		t.Errorf("expected 0 Accumulate calls with hnr_enabled=false, got %d", hnrRepo.accumulateCalls)
	}
}

func TestAnnounce_HnR_UnconfiguredRepoIsSkippedSafely(t *testing.T) {
	// s.hnr is nil (SetHnRRepo never called) even with the setting on — this
	// is the "feature not wired at all" case, and must not panic or error.
	svc, _, _, _ := setupTrackerWithSettings(map[string]string{SettingHnREnabled: "true"})
	completeDownload(t, svc, 500, 1000)
}

func TestAnnounce_HnR_SkipsExemptTorrent(t *testing.T) {
	// The exemption check (torrents.hnr_exempt) is enforced inside
	// CreateIfNotExists' own SQL in production, not passed in by
	// TrackerService — so the fake's exemption is configured on the repo
	// fixture directly, exactly mirroring where the real check lives.
	svc, _, hnrRepo := setupTrackerWithHnR(nil)
	hnrRepo.setTorrent(1, 0, true)

	completeDownload(t, svc, 500, 1000)

	records, err := hnrRepo.ListForUser(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected no hnr record for an hnr_exempt torrent, got %d", len(records))
	}
}

func TestAnnounce_HnR_SkipsExemptDonor(t *testing.T) {
	// Unlike torrents.hnr_exempt (enforced in CreateIfNotExists' own SQL),
	// hnr_exempt_donors is a plain site setting checked by TrackerService
	// itself against the announcing user's Donor flag — so this is set up by
	// hand rather than through setupTrackerWithHnR, to get at the user repo.
	svc, userRepo, _, _ := setupTrackerWithSettings(map[string]string{
		SettingHnREnabled:      "true",
		SettingHnRExemptDonors: "true",
	})
	hnrRepo := newFakeHnRRepo()
	svc.SetHnRRepo(hnrRepo)
	u, err := userRepo.GetByID(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	u.Donor = true

	completeDownload(t, svc, 500, 1000)

	records, err := hnrRepo.ListForUser(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected no hnr record for a donor when hnr_exempt_donors is on, got %d", len(records))
	}
}

func TestAnnounce_HnR_TracksDonorWhenExemptionOff(t *testing.T) {
	// hnr_exempt_donors defaults to false — a donor is tracked exactly like
	// anyone else unless an operator opts in.
	svc, userRepo, _, _ := setupTrackerWithSettings(map[string]string{
		SettingHnREnabled: "true",
	})
	hnrRepo := newFakeHnRRepo()
	svc.SetHnRRepo(hnrRepo)
	u, err := userRepo.GetByID(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	u.Donor = true

	completeDownload(t, svc, 500, 1000)

	records, err := hnrRepo.ListForUser(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected a donor to be tracked when hnr_exempt_donors is off, got %d records", len(records))
	}
}
