package service

import (
	"context"
	"sync"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// --- Mock announce event repository ---

type trackerMockAnnounceEventRepo struct {
	mu     sync.Mutex
	events []model.AnnounceEvent
}

func (m *trackerMockAnnounceEventRepo) Create(_ context.Context, e *model.AnnounceEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, *e) // copy: caller reuses the struct across announces
	return nil
}

func (m *trackerMockAnnounceEventRepo) ListByUser(_ context.Context, _ int64, _, _ int) ([]repository.AnnounceEventWithTorrent, int64, error) {
	return nil, 0, nil
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
	if e.IP != "1.2.3.4" || e.Port != 6881 {
		t.Errorf("endpoint = %s:%d, want 1.2.3.4:6881", e.IP, e.Port)
	}
	if e.TorrentID == nil {
		t.Error("TorrentID = nil, want the announced torrent")
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
