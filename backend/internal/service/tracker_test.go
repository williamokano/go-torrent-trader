package service

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// --- Mock User Repository for tracker tests ---

type trackerMockUserRepo struct {
	mu    sync.Mutex
	users []*model.User
}

func newTrackerMockUserRepo() *trackerMockUserRepo {
	return &trackerMockUserRepo{}
}

func (m *trackerMockUserRepo) addUser(u *model.User) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users = append(m.users, u)
}

func (m *trackerMockUserRepo) GetByID(_ context.Context, id int64) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (m *trackerMockUserRepo) GetByUsername(_ context.Context, username string) (*model.User, error) {
	return nil, sql.ErrNoRows
}

func (m *trackerMockUserRepo) GetByEmail(_ context.Context, _ string) (*model.User, error) {
	return nil, sql.ErrNoRows
}

func (m *trackerMockUserRepo) GetByPasskey(_ context.Context, passkey string) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.Passkey != nil && *u.Passkey == passkey {
			return u, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (m *trackerMockUserRepo) Count(_ context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return int64(len(m.users)), nil
}

func (m *trackerMockUserRepo) Create(_ context.Context, _ *model.User) error { return nil }
func (m *trackerMockUserRepo) Update(_ context.Context, _ *model.User) error { return nil }

func (m *trackerMockUserRepo) IncrementStats(_ context.Context, id int64, uploadDelta, downloadDelta int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			u.Uploaded += uploadDelta
			u.Downloaded += downloadDelta
			return nil
		}
	}
	return sql.ErrNoRows
}

func (m *trackerMockUserRepo) List(_ context.Context, _ repository.ListUsersOptions) ([]model.User, int64, error) {
	return nil, 0, nil
}

// --- Mock Torrent Repository ---

type trackerMockTorrentRepo struct {
	mu       sync.Mutex
	torrents []*model.Torrent
}

func newTrackerMockTorrentRepo() *trackerMockTorrentRepo {
	return &trackerMockTorrentRepo{}
}

func (m *trackerMockTorrentRepo) addTorrent(t *model.Torrent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.torrents = append(m.torrents, t)
}

func (m *trackerMockTorrentRepo) GetByID(_ context.Context, id int64) (*model.Torrent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.torrents {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (m *trackerMockTorrentRepo) GetByInfoHash(_ context.Context, infoHash []byte) (*model.Torrent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.torrents {
		if bytesEqual(t.InfoHash, infoHash) {
			return t, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (m *trackerMockTorrentRepo) List(_ context.Context, _ repository.ListTorrentsOptions) ([]model.Torrent, int64, error) {
	return nil, 0, nil
}

func (m *trackerMockTorrentRepo) Create(_ context.Context, _ *model.Torrent) error { return nil }
func (m *trackerMockTorrentRepo) Update(_ context.Context, _ *model.Torrent) error { return nil }
func (m *trackerMockTorrentRepo) Delete(_ context.Context, _ int64) error          { return nil }

func (m *trackerMockTorrentRepo) IncrementSeeders(_ context.Context, id int64, delta int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.torrents {
		if t.ID == id {
			t.Seeders += delta
			if t.Seeders < 0 {
				t.Seeders = 0
			}
			return nil
		}
	}
	return sql.ErrNoRows
}

func (m *trackerMockTorrentRepo) IncrementLeechers(_ context.Context, id int64, delta int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.torrents {
		if t.ID == id {
			t.Leechers += delta
			if t.Leechers < 0 {
				t.Leechers = 0
			}
			return nil
		}
	}
	return sql.ErrNoRows
}

func (m *trackerMockTorrentRepo) IncrementTimesCompleted(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.torrents {
		if t.ID == id {
			t.TimesCompleted++
			return nil
		}
	}
	return sql.ErrNoRows
}

// --- Mock Peer Repository ---

type trackerMockPeerRepo struct {
	mu    sync.Mutex
	peers []*model.Peer
}

func newTrackerMockPeerRepo() *trackerMockPeerRepo {
	return &trackerMockPeerRepo{}
}

func (m *trackerMockPeerRepo) GetByTorrentAndUser(_ context.Context, torrentID, userID int64) (*model.Peer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.peers {
		if p.TorrentID == torrentID && p.UserID == userID {
			return p, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (m *trackerMockPeerRepo) ListByTorrent(_ context.Context, torrentID int64, limit int) ([]model.Peer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []model.Peer
	for _, p := range m.peers {
		if p.TorrentID == torrentID {
			result = append(result, *p)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (m *trackerMockPeerRepo) Upsert(_ context.Context, peer *model.Peer) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, p := range m.peers {
		if p.TorrentID == peer.TorrentID && p.UserID == peer.UserID && bytesEqual(p.PeerID, peer.PeerID) {
			m.peers[i] = peer
			return nil
		}
	}
	peer.ID = int64(len(m.peers) + 1)
	m.peers = append(m.peers, peer)
	return nil
}

func (m *trackerMockPeerRepo) Delete(_ context.Context, torrentID, userID int64, peerID []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, p := range m.peers {
		if p.TorrentID == torrentID && p.UserID == userID && bytesEqual(p.PeerID, peerID) {
			m.peers = append(m.peers[:i], m.peers[i+1:]...)
			return nil
		}
	}
	return sql.ErrNoRows
}

func (m *trackerMockPeerRepo) DeleteStale(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

// --- Helpers ---

func testPasskey() string { return "abcdef1234567890abcd" }

func testInfoHash() []byte {
	h := make([]byte, 20)
	for i := range h {
		h[i] = byte(i)
	}
	return h
}

func testPeerID() []byte {
	p := make([]byte, 20)
	for i := range p {
		p[i] = byte(i + 100)
	}
	return p
}

func setupTracker() (*TrackerService, *trackerMockUserRepo, *trackerMockTorrentRepo, *trackerMockPeerRepo) {
	userRepo := newTrackerMockUserRepo()
	torrentRepo := newTrackerMockTorrentRepo()
	peerRepo := newTrackerMockPeerRepo()

	pk := testPasskey()
	userRepo.addUser(&model.User{
		ID:      1,
		Enabled: true,
		Passkey: &pk,
	})

	torrentRepo.addTorrent(&model.Torrent{
		ID:       1,
		InfoHash: testInfoHash(),
		Seeders:  0,
		Leechers: 0,
	})

	svc := NewTrackerService(userRepo, torrentRepo, peerRepo)
	return svc, userRepo, torrentRepo, peerRepo
}

// --- Tests ---

func TestAnnounce_StartedAsLeecher(t *testing.T) {
	svc, _, torrentRepo, peerRepo := setupTracker()

	resp, err := svc.Announce(context.Background(), AnnounceRequest{
		Passkey:    testPasskey(),
		InfoHash:   testInfoHash(),
		PeerID:     testPeerID(),
		IP:         "192.168.1.1",
		Port:       6881,
		Uploaded:   0,
		Downloaded: 0,
		Left:       1000,
		Event:      EventStarted,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Interval != DefaultInterval {
		t.Errorf("expected interval %d, got %d", DefaultInterval, resp.Interval)
	}
	if resp.MinInterval != DefaultMinInterval {
		t.Errorf("expected min interval %d, got %d", DefaultMinInterval, resp.MinInterval)
	}

	// Should have created a peer.
	peerRepo.mu.Lock()
	peerCount := len(peerRepo.peers)
	peerRepo.mu.Unlock()
	if peerCount != 1 {
		t.Errorf("expected 1 peer, got %d", peerCount)
	}

	// Should have incremented leechers.
	torrentRepo.mu.Lock()
	leechers := torrentRepo.torrents[0].Leechers
	seeders := torrentRepo.torrents[0].Seeders
	torrentRepo.mu.Unlock()
	if leechers != 1 {
		t.Errorf("expected 1 leecher, got %d", leechers)
	}
	if seeders != 0 {
		t.Errorf("expected 0 seeders, got %d", seeders)
	}
}

func TestAnnounce_StartedAsSeeder(t *testing.T) {
	svc, _, torrentRepo, _ := setupTracker()

	_, err := svc.Announce(context.Background(), AnnounceRequest{
		Passkey:    testPasskey(),
		InfoHash:   testInfoHash(),
		PeerID:     testPeerID(),
		IP:         "192.168.1.1",
		Port:       6881,
		Uploaded:   1000,
		Downloaded: 0,
		Left:       0, // seeder
		Event:      EventStarted,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	torrentRepo.mu.Lock()
	seeders := torrentRepo.torrents[0].Seeders
	leechers := torrentRepo.torrents[0].Leechers
	torrentRepo.mu.Unlock()
	if seeders != 1 {
		t.Errorf("expected 1 seeder, got %d", seeders)
	}
	if leechers != 0 {
		t.Errorf("expected 0 leechers, got %d", leechers)
	}
}

func TestAnnounce_InvalidPasskey(t *testing.T) {
	svc, _, _, _ := setupTracker()

	_, err := svc.Announce(context.Background(), AnnounceRequest{
		Passkey:  "invalidpasskey1234567",
		InfoHash: testInfoHash(),
		PeerID:   testPeerID(),
		IP:       "192.168.1.1",
		Port:     6881,
		Left:     1000,
		Event:    EventStarted,
	})

	if !errors.Is(err, ErrInvalidPasskey) {
		t.Errorf("expected ErrInvalidPasskey, got %v", err)
	}
}

func TestAnnounce_TorrentNotFound(t *testing.T) {
	svc, _, _, _ := setupTracker()

	unknownHash := make([]byte, 20)
	for i := range unknownHash {
		unknownHash[i] = 0xFF
	}

	_, err := svc.Announce(context.Background(), AnnounceRequest{
		Passkey:  testPasskey(),
		InfoHash: unknownHash,
		PeerID:   testPeerID(),
		IP:       "192.168.1.1",
		Port:     6881,
		Left:     1000,
		Event:    EventStarted,
	})

	if !errors.Is(err, ErrTorrentNotFound) {
		t.Errorf("expected ErrTorrentNotFound, got %v", err)
	}
}

func TestAnnounce_BannedTorrent(t *testing.T) {
	svc, _, torrentRepo, _ := setupTracker()

	torrentRepo.mu.Lock()
	torrentRepo.torrents[0].Banned = true
	torrentRepo.mu.Unlock()

	_, err := svc.Announce(context.Background(), AnnounceRequest{
		Passkey:  testPasskey(),
		InfoHash: testInfoHash(),
		PeerID:   testPeerID(),
		IP:       "192.168.1.1",
		Port:     6881,
		Left:     1000,
		Event:    EventStarted,
	})

	if !errors.Is(err, ErrTorrentBanned) {
		t.Errorf("expected ErrTorrentBanned, got %v", err)
	}
}

func TestAnnounce_DisabledUser(t *testing.T) {
	svc, userRepo, _, _ := setupTracker()

	userRepo.mu.Lock()
	userRepo.users[0].Enabled = false
	userRepo.mu.Unlock()

	_, err := svc.Announce(context.Background(), AnnounceRequest{
		Passkey:  testPasskey(),
		InfoHash: testInfoHash(),
		PeerID:   testPeerID(),
		IP:       "192.168.1.1",
		Port:     6881,
		Left:     1000,
		Event:    EventStarted,
	})

	if !errors.Is(err, ErrUserDisabled) {
		t.Errorf("expected ErrUserDisabled, got %v", err)
	}
}

func TestAnnounce_StoppedEvent(t *testing.T) {
	svc, _, torrentRepo, peerRepo := setupTracker()

	// First, start as leecher.
	_, err := svc.Announce(context.Background(), AnnounceRequest{
		Passkey:  testPasskey(),
		InfoHash: testInfoHash(),
		PeerID:   testPeerID(),
		IP:       "192.168.1.1",
		Port:     6881,
		Left:     1000,
		Event:    EventStarted,
	})
	if err != nil {
		t.Fatalf("start announce failed: %v", err)
	}

	// Now stop.
	_, err = svc.Announce(context.Background(), AnnounceRequest{
		Passkey:  testPasskey(),
		InfoHash: testInfoHash(),
		PeerID:   testPeerID(),
		IP:       "192.168.1.1",
		Port:     6881,
		Left:     500,
		Event:    EventStopped,
	})
	if err != nil {
		t.Fatalf("stop announce failed: %v", err)
	}

	// Peer should be removed.
	peerRepo.mu.Lock()
	peerCount := len(peerRepo.peers)
	peerRepo.mu.Unlock()
	if peerCount != 0 {
		t.Errorf("expected 0 peers after stop, got %d", peerCount)
	}

	// Leecher count should be back to 0.
	torrentRepo.mu.Lock()
	leechers := torrentRepo.torrents[0].Leechers
	torrentRepo.mu.Unlock()
	if leechers != 0 {
		t.Errorf("expected 0 leechers after stop, got %d", leechers)
	}
}

func TestAnnounce_CompletedEvent(t *testing.T) {
	svc, _, torrentRepo, _ := setupTracker()

	// Start as leecher.
	_, err := svc.Announce(context.Background(), AnnounceRequest{
		Passkey:  testPasskey(),
		InfoHash: testInfoHash(),
		PeerID:   testPeerID(),
		IP:       "192.168.1.1",
		Port:     6881,
		Left:     1000,
		Event:    EventStarted,
	})
	if err != nil {
		t.Fatalf("start announce failed: %v", err)
	}

	// Complete download (now a seeder).
	_, err = svc.Announce(context.Background(), AnnounceRequest{
		Passkey:    testPasskey(),
		InfoHash:   testInfoHash(),
		PeerID:     testPeerID(),
		IP:         "192.168.1.1",
		Port:       6881,
		Uploaded:   500,
		Downloaded: 1000,
		Left:       0,
		Event:      EventCompleted,
	})
	if err != nil {
		t.Fatalf("completed announce failed: %v", err)
	}

	torrentRepo.mu.Lock()
	tc := torrentRepo.torrents[0].TimesCompleted
	seeders := torrentRepo.torrents[0].Seeders
	leechers := torrentRepo.torrents[0].Leechers
	torrentRepo.mu.Unlock()

	if tc != 1 {
		t.Errorf("expected times_completed=1, got %d", tc)
	}
	if seeders != 1 {
		t.Errorf("expected 1 seeder after completion, got %d", seeders)
	}
	if leechers != 0 {
		t.Errorf("expected 0 leechers after completion, got %d", leechers)
	}
}

func TestAnnounce_UserStatsUpdated(t *testing.T) {
	svc, userRepo, _, _ := setupTracker()

	// Start with some uploaded/downloaded.
	_, err := svc.Announce(context.Background(), AnnounceRequest{
		Passkey:    testPasskey(),
		InfoHash:   testInfoHash(),
		PeerID:     testPeerID(),
		IP:         "192.168.1.1",
		Port:       6881,
		Uploaded:   1000,
		Downloaded: 500,
		Left:       500,
		Event:      EventStarted,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Second announce with more traffic — should produce deltas.
	_, err = svc.Announce(context.Background(), AnnounceRequest{
		Passkey:    testPasskey(),
		InfoHash:   testInfoHash(),
		PeerID:     testPeerID(),
		IP:         "192.168.1.1",
		Port:       6881,
		Uploaded:   3000,
		Downloaded: 1500,
		Left:       0,
		Event:      EventEmpty,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	userRepo.mu.Lock()
	uploaded := userRepo.users[0].Uploaded
	downloaded := userRepo.users[0].Downloaded
	userRepo.mu.Unlock()

	// Delta: 3000 - 1000 = 2000 uploaded, 1500 - 500 = 1000 downloaded
	if uploaded != 2000 {
		t.Errorf("expected uploaded=2000, got %d", uploaded)
	}
	if downloaded != 1000 {
		t.Errorf("expected downloaded=1000, got %d", downloaded)
	}
}

func TestBuildCompactPeerList(t *testing.T) {
	peers := []model.Peer{
		{PeerID: []byte("01234567890123456789"), IP: "192.168.1.1", Port: 6881},
		{PeerID: []byte("98765432109876543210"), IP: "10.0.0.1", Port: 8080},
		{PeerID: []byte("aaaabbbbccccddddeeee"), IP: "172.16.0.1", Port: 51413},
	}

	// Exclude the first peer.
	result := buildCompactPeerList(peers, []byte("01234567890123456789"), 50)

	// Should have 2 peers * 6 bytes = 12 bytes.
	if len(result) != 12 {
		t.Fatalf("expected 12 bytes, got %d", len(result))
	}
}

func TestBuildCompactPeerList_ExcludesIPv6(t *testing.T) {
	peers := []model.Peer{
		{PeerID: []byte("01234567890123456789"), IP: "::1", Port: 6881},
		{PeerID: []byte("98765432109876543210"), IP: "192.168.1.1", Port: 6882},
	}

	result := buildCompactPeerList(peers, nil, 50)

	// Only the IPv4 peer should be included.
	if len(result) != 6 {
		t.Fatalf("expected 6 bytes (1 IPv4 peer), got %d", len(result))
	}
}

func TestBuildCompactPeerList_RespectsMaxPeers(t *testing.T) {
	var peers []model.Peer
	for i := 0; i < 100; i++ {
		pid := make([]byte, 20)
		pid[0] = byte(i)
		peers = append(peers, model.Peer{
			PeerID: pid,
			IP:     "192.168.1.1",
			Port:   6881 + i,
		})
	}

	result := buildCompactPeerList(peers, nil, 50)

	// Should be capped at 50 peers * 6 bytes = 300 bytes.
	if len(result) != 300 {
		t.Fatalf("expected 300 bytes, got %d", len(result))
	}
}
