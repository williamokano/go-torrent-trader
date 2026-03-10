package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/hibiken/asynq"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
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
func (m *mockPeerRepo) CountTotalByUser(_ context.Context, _ int64) (int, error) {
	return 0, nil
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
func (m *mockTorrentRepo) Create(_ context.Context, _ *model.Torrent) error              { return nil }
func (m *mockTorrentRepo) Update(_ context.Context, _ *model.Torrent) error              { return nil }
func (m *mockTorrentRepo) Delete(_ context.Context, _ int64) error                       { return nil }
func (m *mockTorrentRepo) IncrementSeeders(_ context.Context, _ int64, _ int) error      { return nil }
func (m *mockTorrentRepo) IncrementLeechers(_ context.Context, _ int64, _ int) error      { return nil }
func (m *mockTorrentRepo) IncrementTimesCompleted(_ context.Context, _ int64) error { return nil }
func (m *mockTorrentRepo) ListByUploader(_ context.Context, _ int64, _ int) ([]model.Torrent, error) {
	return nil, nil
}

var _ repository.TorrentRepository = (*mockTorrentRepo)(nil)

// --- email handler tests ----------------------------------------------------

func TestHandleSendEmailValid(t *testing.T) {
	payload, err := json.Marshal(EmailPayload{
		To:      "user@example.com",
		Subject: "Test",
		Body:    "Hello",
	})
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	deps := &WorkerDeps{}
	handler := NewSendEmailHandler(deps)
	task := asynq.NewTask(TaskSendEmail, payload)
	if err := handler(context.Background(), task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleSendEmailInvalidPayload(t *testing.T) {
	deps := &WorkerDeps{}
	handler := NewSendEmailHandler(deps)
	task := asynq.NewTask(TaskSendEmail, []byte("invalid json"))
	err := handler(context.Background(), task)
	if err == nil {
		t.Fatal("expected error for invalid payload")
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
	task := asynq.NewTask(TaskCleanupPeers, nil)

	before := time.Now()
	if err := handler(context.Background(), task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if peerRepo.deleteStaleCall == nil {
		t.Fatal("expected DeleteStale to be called")
	}

	// The cutoff should be approximately now - 30 minutes.
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
	task := asynq.NewTask(TaskCleanupPeers, nil)

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
	task := asynq.NewTask(TaskCleanupPeers, nil)

	if err := handler(context.Background(), task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- recalc stats handler test ----------------------------------------------

func TestRecalcStatsHandler_NilCache(t *testing.T) {
	deps := &WorkerDeps{
		PeerRepo:    &mockPeerRepo{},
		TorrentRepo: &mockTorrentRepo{},
		StatsCache:  nil,
	}

	handler := NewRecalcStatsHandler(deps)
	task := asynq.NewTask(TaskRecalcStats, nil)

	if err := handler(context.Background(), task); err != nil {
		t.Fatalf("unexpected error with nil cache: %v", err)
	}
}
