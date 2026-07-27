package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// --- Mock announce event repository ---

type trackerMockAnnounceEventRepo struct {
	mu     sync.Mutex
	events []model.AnnounceEvent
	// createErr makes Create fail the way the real one can. A double that always
	// succeeds cannot exercise the best-effort path, which is the whole guarantee
	// the announce path relies on.
	createErr error
	calls     int
}

func (m *trackerMockAnnounceEventRepo) Create(_ context.Context, e *model.AnnounceEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.createErr != nil {
		return m.createErr
	}
	m.events = append(m.events, *e) // copy: caller reuses the struct across announces
	return nil
}

func (m *trackerMockAnnounceEventRepo) ListByUser(_ context.Context, _ int64, _, _ int) ([]repository.AnnounceEventWithTorrent, int64, error) {
	return nil, 0, nil
}

// The read and housekeeping halves of the interface are unreachable from the
// tracker, which only ever appends. They fail loudly rather than returning a
// plausible zero value, so a future call from the announce path shows up as a test
// failure instead of as a silently empty result.
func (m *trackerMockAnnounceEventRepo) DeleteOlderThan(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, errors.New("DeleteOlderThan: not reachable from the announce path")
}

func (m *trackerMockAnnounceEventRepo) snapshot() []model.AnnounceEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]model.AnnounceEvent(nil), m.events...)
}

// TestAnnounce_RecordsEventByDefault proves capture is on when no site settings
// are wired (the capture-on default) and the announcing peer's fields land in
// the log.
func TestAnnounce_RecordsEventByDefault(t *testing.T) {
	svc, _, _, _ := setupTracker()
	log := &trackerMockAnnounceEventRepo{}
	svc.SetAnnounceEventRepo(log)

	_, err := svc.Announce(context.Background(), AnnounceRequest{
		Passkey:  testPasskey(),
		InfoHash: testInfoHash(),
		PeerID:   testPeerID(),
		IP:       "1.2.3.4",
		Port:     6881,
		Left:     1000,
		Event:    EventStarted,
	})
	if err != nil {
		t.Fatalf("Announce: %v", err)
	}

	events := log.snapshot()
	if len(events) != 1 {
		t.Fatalf("recorded %d events, want 1", len(events))
	}
	e := events[0]
	if e.Event != "started" {
		t.Errorf("Event = %q, want started", e.Event)
	}
	if e.Seeder {
		t.Error("Seeder = true, want false (left > 0)")
	}
	if e.Port != 6881 {
		t.Errorf("Port = %d, want 6881", e.Port)
	}
	if e.TorrentID == nil {
		t.Error("TorrentID = nil, want the announced torrent")
	}
}

// TestAnnounce_SurvivesAFailingLogWrite proves the guarantee recordAnnounceEvent
// documents — "a logging failure is logged but never breaks the announce" — by
// making the write fail rather than by reading the code.
//
// It matters more than a doc-check normally would. The announce is the one request
// on the site that must not fail: a member whose client gets an error stops being
// counted in the swarm. Nothing enforced this, because the only double for the
// repository always returned nil, so every existing test exercised the success
// path and the guarantee held by inspection alone.
//
// It is also the premise the partitioning proposal (#221) rests on. That issue
// argues a missing monthly partition would "reject announces", which is what makes
// its failure mode worse than the current DELETE falling behind. On today's code
// that is not so — the insert error is swallowed here and the announce still
// answers, so a missing partition would silently drop log rows instead. Whichever
// way #221 goes, the property is worth pinning: if a later change ever routes this
// write through the announce's own transaction, the tracker starts failing on a
// logging error and this test is what says so.
func TestAnnounce_SurvivesAFailingLogWrite(t *testing.T) {
	svc, _, _, _ := setupTracker()
	log := &trackerMockAnnounceEventRepo{
		createErr: errors.New("insert failed: no partition of relation \"announce_events\" found for row"),
	}
	svc.SetAnnounceEventRepo(log)

	resp, err := svc.Announce(context.Background(), AnnounceRequest{
		Passkey:  testPasskey(),
		InfoHash: testInfoHash(),
		PeerID:   testPeerID(),
		IP:       "1.2.3.4",
		Port:     6881,
		Left:     1000,
		Event:    EventStarted,
	})
	if err != nil {
		t.Fatalf("Announce returned %v; a failing announce-log write must not break the announce", err)
	}
	if resp == nil {
		t.Fatal("Announce returned no response; the peer would get nothing back")
	}
	// The peer still has to be told when to come back, or a client that treats a
	// malformed response as fatal drops out of the swarm.
	if resp.Interval <= 0 {
		t.Errorf("Interval = %d, want the usual announce interval", resp.Interval)
	}

	// And the write must actually have been attempted — otherwise this passes for
	// the wrong reason, by the log being skipped rather than by its failure being
	// tolerated.
	if log.calls != 1 {
		t.Errorf("Create called %d times, want 1", log.calls)
	}
}

// TestAnnounce_LogDisabledRecordsNothing proves the enable flag gates capture.
func TestAnnounce_LogDisabledRecordsNothing(t *testing.T) {
	svc, _, _, _ := setupTrackerWithSettings(map[string]string{
		SettingAnnounceLogEnabled: "false",
	})
	log := &trackerMockAnnounceEventRepo{}
	svc.SetAnnounceEventRepo(log)

	_, err := svc.Announce(context.Background(), AnnounceRequest{
		Passkey:  testPasskey(),
		InfoHash: testInfoHash(),
		PeerID:   testPeerID(),
		IP:       "1.2.3.4",
		Port:     6881,
		Left:     1000,
		Event:    EventStarted,
	})
	if err != nil {
		t.Fatalf("Announce: %v", err)
	}

	if got := len(log.snapshot()); got != 0 {
		t.Fatalf("recorded %d events, want 0 when disabled", got)
	}
}

// TestAnnounce_LogCapturesDeltas proves that a keep-alive announce records the
// per-announce deltas (the data destroyed by the overwriting projections) and
// maps the empty BitTorrent event to "announce".
func TestAnnounce_LogCapturesDeltas(t *testing.T) {
	svc, _, _, _ := setupTracker()
	log := &trackerMockAnnounceEventRepo{}
	svc.SetAnnounceEventRepo(log)
	ctx := context.Background()

	// First announce establishes the peer's baseline (delta 0).
	if _, err := svc.Announce(ctx, AnnounceRequest{
		Passkey: testPasskey(), InfoHash: testInfoHash(), PeerID: testPeerID(),
		IP: "1.2.3.4", Port: 6881, Left: 1000, Event: EventStarted,
	}); err != nil {
		t.Fatalf("Announce(started): %v", err)
	}

	// Second announce reports progress: 100 up / 500 down since the baseline.
	if _, err := svc.Announce(ctx, AnnounceRequest{
		Passkey: testPasskey(), InfoHash: testInfoHash(), PeerID: testPeerID(),
		IP: "1.2.3.4", Port: 6881, Uploaded: 100, Downloaded: 500, Left: 500,
		Event: EventEmpty,
	}); err != nil {
		t.Fatalf("Announce(update): %v", err)
	}

	events := log.snapshot()
	if len(events) != 2 {
		t.Fatalf("recorded %d events, want 2", len(events))
	}
	e := events[1]
	if e.Event != "announce" {
		t.Errorf("Event = %q, want announce (empty event mapped)", e.Event)
	}
	if e.UploadedDelta != 100 || e.DownloadedDelta != 500 {
		t.Errorf("deltas = %d/%d, want 100/500", e.UploadedDelta, e.DownloadedDelta)
	}
	// Torrent is not freeleech, so the counted delta equals the raw delta.
	if e.CountedDownloadedDelta != 500 {
		t.Errorf("CountedDownloadedDelta = %d, want 500", e.CountedDownloadedDelta)
	}
	if e.Uploaded != 100 || e.Downloaded != 500 {
		t.Errorf("totals = %d/%d, want 100/500", e.Uploaded, e.Downloaded)
	}
}
