package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/riverqueue/river"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// --- mock PeerRepository ---------------------------------------------------

type mockPeerRepo struct {
	deleteStaleCount int64
	deleteStaleErr   error
	deleteStaleCall  *time.Time // captures the cutoff arg
}

func (m *mockPeerRepo) GetByTorrentAndUser(_ context.Context, _, _ int64) (*model.Peer, error) {
	return nil, nil
}
func (m *mockPeerRepo) GetByTorrentUserAndPeerID(_ context.Context, _, _ int64, _ []byte) (*model.Peer, error) {
	return nil, nil
}
func (m *mockPeerRepo) ListByTorrent(_ context.Context, _ int64, _ int) ([]model.Peer, error) {
	return nil, nil
}
func (m *mockPeerRepo) Upsert(_ context.Context, _ *model.Peer) error { return nil }
func (m *mockPeerRepo) Delete(_ context.Context, _, _ int64, _ []byte) error {
	return nil
}
func (m *mockPeerRepo) CountByUser(_ context.Context, _ int64) (int, int, error) {
	return 0, 0, nil
}
func (m *mockPeerRepo) CountByTorrent(_ context.Context, _ int64) (int, error) {
	return 0, nil
}
func (*mockPeerRepo) CountByTorrentAndUser(context.Context, int64, int64) (int, error) { return 0, nil }

func (m *mockPeerRepo) CountTotalByUser(_ context.Context, _ int64) (int, error) {
	return 0, nil
}
func (*mockPeerRepo) SwapState(context.Context, int64, int64, []byte, model.PeerState) (model.PeerState, bool, error) {
	return model.PeerState{}, false, nil
}

func (m *mockPeerRepo) DeleteStale(_ context.Context, before time.Time) (int64, error) {
	m.deleteStaleCall = &before
	return m.deleteStaleCount, m.deleteStaleErr
}
func (m *mockPeerRepo) ListByUserSeeding(_ context.Context, _ int64, _, _ int) ([]repository.PeerWithTorrent, int64, error) {
	return nil, 0, nil
}
func (m *mockPeerRepo) ListByUserLeeching(_ context.Context, _ int64, _, _ int) ([]repository.PeerWithTorrent, int64, error) {
	return nil, 0, nil
}

var _ repository.PeerRepository = (*mockPeerRepo)(nil)

// --- mock TorrentRepository -------------------------------------------------

type mockTorrentRepo struct{}

func (m *mockTorrentRepo) GetByID(_ context.Context, _ int64) (*model.Torrent, error) {
	return nil, nil
}
func (m *mockTorrentRepo) GetByInfoHash(_ context.Context, _ []byte) (*model.Torrent, error) {
	return nil, nil
}
func (m *mockTorrentRepo) List(_ context.Context, _ repository.ListTorrentsOptions) ([]model.Torrent, int64, error) {
	return nil, 0, nil
}
func (m *mockTorrentRepo) Create(_ context.Context, _ *model.Torrent) error          { return nil }
func (m *mockTorrentRepo) Update(_ context.Context, _ *model.Torrent) error          { return nil }
func (m *mockTorrentRepo) Delete(_ context.Context, _ int64) error                   { return nil }
func (m *mockTorrentRepo) IncrementSeeders(_ context.Context, _ int64, _ int) error  { return nil }
func (m *mockTorrentRepo) IncrementLeechers(_ context.Context, _ int64, _ int) error { return nil }
func (m *mockTorrentRepo) IncrementTimesCompleted(_ context.Context, _ int64) error  { return nil }
func (m *mockTorrentRepo) ListByUploader(_ context.Context, _ int64, _ int) ([]model.Torrent, error) {
	return nil, nil
}

var _ repository.TorrentRepository = (*mockTorrentRepo)(nil)

// --- email handler tests ----------------------------------------------------

type recordingSender struct {
	to, subject, body string
	err               error
}

func (r *recordingSender) Send(_ context.Context, to, subject, body string) error {
	r.to, r.subject, r.body = to, subject, body
	return r.err
}

func emailJob() *river.Job[SendEmailArgs] {
	return &river.Job[SendEmailArgs]{Args: SendEmailArgs{To: "user@example.com", Subject: "Test", Body: "Hello"}}
}

func TestHandleSendEmailSends(t *testing.T) {
	sender := &recordingSender{}
	if err := NewSendEmailHandler(&WorkerDeps{EmailSender: sender})(context.Background(), emailJob()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sender.to != "user@example.com" || sender.subject != "Test" || sender.body != "Hello" {
		t.Fatalf("sent %+v, want the job's message", sender)
	}
}

// A send failure is returned so River retries it (SendEmailArgs allows four
// attempts) instead of the email being dropped.
func TestHandleSendEmailReturnsSendFailure(t *testing.T) {
	sender := &recordingSender{err: errors.New("smtp down")}
	if err := NewSendEmailHandler(&WorkerDeps{EmailSender: sender})(context.Background(), emailJob()); err == nil {
		t.Fatal("expected the send failure to be returned")
	}
}

func TestHandleSendEmailWithoutSenderIsANoOp(t *testing.T) {
	if err := NewSendEmailHandler(&WorkerDeps{})(context.Background(), emailJob()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- cleanup handler tests --------------------------------------------------

func TestCleanupHandler_RemovesStalePeers(t *testing.T) {
	peerRepo := &mockPeerRepo{deleteStaleCount: 5}
	deps := &WorkerDeps{
		PeerRepo:    peerRepo,
		TorrentRepo: &mockTorrentRepo{},
		// DB is nil — the SQL queries will not be called because we'd need
		// a real *sql.DB. We test the DeleteStale path and verify the cutoff.
		// Integration tests cover the full SQL path.
	}

	// We cannot run the full handler without a real DB because it executes raw
	// SQL after deletion. Instead, verify that DeleteStale is called with a
	// reasonable cutoff by testing with removed == 0 (which skips the SQL).
	peerRepo.deleteStaleCount = 0
	handler := NewCleanupHandler(deps)
	task := &river.Job[CleanupPeersArgs]{}

	before := time.Now()
	if err := handler(context.Background(), task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if peerRepo.deleteStaleCall == nil {
		t.Fatal("expected DeleteStale to be called")
	}

	// The cutoff should be approximately now - StalePeerCutoff.
	expectedCutoff := before.Add(-StalePeerCutoff)
	diff := peerRepo.deleteStaleCall.Sub(expectedCutoff)
	if diff < 0 {
		diff = -diff
	}
	if diff > 2*time.Second {
		t.Fatalf("cutoff drift too large: got %v, expected ~%v (diff %v)",
			*peerRepo.deleteStaleCall, expectedCutoff, diff)
	}
}

func TestCleanupHandler_DeleteStaleError(t *testing.T) {
	peerRepo := &mockPeerRepo{
		deleteStaleErr: errors.New("database connection lost"),
	}
	deps := &WorkerDeps{
		PeerRepo:    peerRepo,
		TorrentRepo: &mockTorrentRepo{},
	}

	handler := NewCleanupHandler(deps)
	task := &river.Job[CleanupPeersArgs]{}

	err := handler(context.Background(), task)
	if err == nil {
		t.Fatal("expected error when DeleteStale fails")
	}
	if !errors.Is(err, peerRepo.deleteStaleErr) {
		t.Fatalf("expected wrapped error containing %q, got: %v", peerRepo.deleteStaleErr, err)
	}
}

func TestCleanupHandler_NoPeersRemovedSkipsRecount(t *testing.T) {
	peerRepo := &mockPeerRepo{deleteStaleCount: 0}
	deps := &WorkerDeps{
		PeerRepo:    peerRepo,
		TorrentRepo: &mockTorrentRepo{},
		DB:          nil, // nil DB is safe because handler returns early when removed == 0
	}

	handler := NewCleanupHandler(deps)
	task := &river.Job[CleanupPeersArgs]{}

	if err := handler(context.Background(), task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// One missed announce must not get a peer removed: it would come back with no
// baseline and lose the credit for everything since its last announce.
func TestStalePeerCutoffOutlastsAMissedAnnounce(t *testing.T) {
	if interval := time.Duration(service.DefaultInterval) * time.Second; StalePeerCutoff <= 2*interval {
		t.Fatalf("StalePeerCutoff = %v, want more than two announce intervals (%v)", StalePeerCutoff, 2*interval)
	}
}
