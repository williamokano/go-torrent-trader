package service

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
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

func (m *trackerMockUserRepo) ListStaff(_ context.Context) ([]model.User, error) {
	return nil, nil
}

func (m *trackerMockUserRepo) UpdateLastAccess(_ context.Context, _ int64) error { return nil }

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
func (m *trackerMockTorrentRepo) ListByUploader(_ context.Context, _ int64, _ int) ([]model.Torrent, error) {
	return nil, nil
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

func (m *trackerMockPeerRepo) GetByTorrentUserAndPeerID(_ context.Context, torrentID, userID int64, peerID []byte) (*model.Peer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.peers {
		if p.TorrentID == torrentID && p.UserID == userID && bytesEqual(p.PeerID, peerID) {
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

func (m *trackerMockPeerRepo) CountByUser(_ context.Context, _ int64) (int, int, error) {
	return 0, 0, nil
}
func (m *trackerMockPeerRepo) CountByTorrent(_ context.Context, torrentID int64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, p := range m.peers {
		if p.TorrentID == torrentID {
			count++
		}
	}
	return count, nil
}
func (m *trackerMockPeerRepo) CountTotalByUser(_ context.Context, userID int64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, p := range m.peers {
		if p.UserID == userID {
			count++
		}
	}
	return count, nil
}
func (m *trackerMockPeerRepo) DeleteStale(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}
func (m *trackerMockPeerRepo) ListByUserSeeding(_ context.Context, _ int64, _, _ int) ([]repository.PeerWithTorrent, int64, error) {
	return nil, 0, nil
}
func (m *trackerMockPeerRepo) ListByUserLeeching(_ context.Context, _ int64, _, _ int) ([]repository.PeerWithTorrent, int64, error) {
	return nil, 0, nil
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
		ID:          1,
		Enabled:     true,
		Passkey:     &pk,
		CanDownload: true,
		CanUpload:   true,
		CanChat:     true,
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

func setupTrackerWithSettings(settings map[string]string) (*TrackerService, *trackerMockUserRepo, *trackerMockTorrentRepo, *trackerMockPeerRepo) {
	svc, userRepo, torrentRepo, peerRepo := setupTracker()
	settingsRepo := newMockSiteSettingsRepo()
	for k, v := range settings {
		settingsRepo.settings[k] = &model.SiteSetting{Key: k, Value: v}
	}
	bus := event.NewInMemoryBus()
	siteSettings := NewSiteSettingsService(settingsRepo, bus)
	svc.SetSiteSettings(siteSettings)
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

func TestAnnounce_DownloadSuspended_Leeching(t *testing.T) {
	svc, userRepo, _, _ := setupTracker()

	userRepo.mu.Lock()
	userRepo.users[0].CanDownload = false
	userRepo.mu.Unlock()

	_, err := svc.Announce(context.Background(), AnnounceRequest{
		Passkey:  testPasskey(),
		InfoHash: testInfoHash(),
		PeerID:   testPeerID(),
		IP:       "192.168.1.1",
		Port:     6881,
		Left:     1000, // still leeching
		Event:    EventStarted,
	})

	if !errors.Is(err, ErrDownloadSuspended) {
		t.Errorf("expected ErrDownloadSuspended, got %v", err)
	}
}

func TestAnnounce_DownloadSuspended_SeedingAllowed(t *testing.T) {
	svc, userRepo, _, _ := setupTracker()

	userRepo.mu.Lock()
	userRepo.users[0].CanDownload = false
	userRepo.mu.Unlock()

	// Seeding (Left=0) should still be allowed even with download restriction.
	_, err := svc.Announce(context.Background(), AnnounceRequest{
		Passkey:  testPasskey(),
		InfoHash: testInfoHash(),
		PeerID:   testPeerID(),
		IP:       "192.168.1.1",
		Port:     6881,
		Left:     0, // seeding
		Event:    EventStarted,
	})

	if err != nil {
		t.Errorf("seeding should be allowed even with download restriction, got: %v", err)
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

func TestAnnounce_MaxPeersPerTorrent_RejectsNewPeer(t *testing.T) {
	svc, _, _, peerRepo := setupTrackerWithSettings(map[string]string{
		SettingTrackerMaxPeersPerTorrent: "2",
	})

	// Pre-fill 2 peers on the torrent from other users.
	peerRepo.mu.Lock()
	for i := 0; i < 2; i++ {
		pid := make([]byte, 20)
		pid[0] = byte(i + 200)
		peerRepo.peers = append(peerRepo.peers, &model.Peer{
			ID:        int64(i + 1),
			TorrentID: 1,
			UserID:    int64(i + 10),
			PeerID:    pid,
			IP:        "10.0.0.1",
			Port:      6881,
		})
	}
	peerRepo.mu.Unlock()

	_, err := svc.Announce(context.Background(), AnnounceRequest{
		Passkey:  testPasskey(),
		InfoHash: testInfoHash(),
		PeerID:   testPeerID(),
		IP:       "192.168.1.1",
		Port:     6881,
		Left:     1000,
		Event:    EventStarted,
	})

	if !errors.Is(err, ErrTooManyPeers) {
		t.Errorf("expected ErrTooManyPeers, got %v", err)
	}
}

func TestAnnounce_MaxPeersPerTorrent_AllowsExistingPeer(t *testing.T) {
	svc, _, _, peerRepo := setupTrackerWithSettings(map[string]string{
		SettingTrackerMaxPeersPerTorrent: "1",
	})

	// Pre-fill this exact peer so it's an update, not a new peer.
	peerRepo.mu.Lock()
	peerRepo.peers = append(peerRepo.peers, &model.Peer{
		ID:        1,
		TorrentID: 1,
		UserID:    1,
		PeerID:    testPeerID(),
		IP:        "192.168.1.1",
		Port:      6881,
		Seeder:    false,
	})
	peerRepo.mu.Unlock()

	// Regular announce (update) should succeed even though limit is 1 and 1 peer exists.
	_, err := svc.Announce(context.Background(), AnnounceRequest{
		Passkey:    testPasskey(),
		InfoHash:   testInfoHash(),
		PeerID:     testPeerID(),
		IP:         "192.168.1.1",
		Port:       6881,
		Uploaded:   100,
		Downloaded: 50,
		Left:       500,
		Event:      EventEmpty,
	})

	if err != nil {
		t.Errorf("expected existing peer update to succeed, got %v", err)
	}
}

func TestAnnounce_MaxPeersPerUser_RejectsNewPeer(t *testing.T) {
	svc, _, torrentRepo, peerRepo := setupTrackerWithSettings(map[string]string{
		SettingTrackerMaxPeersPerUser: "2",
	})

	// Add a second torrent for the user's other peers.
	otherInfoHash := make([]byte, 20)
	for i := range otherInfoHash {
		otherInfoHash[i] = byte(i + 50)
	}
	torrentRepo.addTorrent(&model.Torrent{
		ID:       2,
		InfoHash: otherInfoHash,
	})

	// Pre-fill 2 peers for user 1 on different torrents.
	peerRepo.mu.Lock()
	for i := 0; i < 2; i++ {
		pid := make([]byte, 20)
		pid[0] = byte(i + 200)
		peerRepo.peers = append(peerRepo.peers, &model.Peer{
			ID:        int64(i + 1),
			TorrentID: int64(i + 2), // different torrents
			UserID:    1,
			PeerID:    pid,
			IP:        "10.0.0.1",
			Port:      6881,
		})
	}
	peerRepo.mu.Unlock()

	_, err := svc.Announce(context.Background(), AnnounceRequest{
		Passkey:  testPasskey(),
		InfoHash: testInfoHash(),
		PeerID:   testPeerID(),
		IP:       "192.168.1.1",
		Port:     6881,
		Left:     1000,
		Event:    EventStarted,
	})

	if !errors.Is(err, ErrTooManyConns) {
		t.Errorf("expected ErrTooManyConns, got %v", err)
	}
}

func TestAnnounce_ConnectionLimits_StoppedEventBypassesLimits(t *testing.T) {
	svc, _, _, peerRepo := setupTrackerWithSettings(map[string]string{
		SettingTrackerMaxPeersPerTorrent: "1",
		SettingTrackerMaxPeersPerUser:    "1",
	})

	// Pre-fill this peer so it can be stopped.
	peerRepo.mu.Lock()
	peerRepo.peers = append(peerRepo.peers, &model.Peer{
		ID:        1,
		TorrentID: 1,
		UserID:    1,
		PeerID:    testPeerID(),
		IP:        "192.168.1.1",
		Port:      6881,
		Seeder:    false,
	})
	peerRepo.mu.Unlock()

	// Stopped event should never be blocked by connection limits.
	_, err := svc.Announce(context.Background(), AnnounceRequest{
		Passkey:  testPasskey(),
		InfoHash: testInfoHash(),
		PeerID:   testPeerID(),
		IP:       "192.168.1.1",
		Port:     6881,
		Left:     500,
		Event:    EventStopped,
	})

	if err != nil {
		t.Errorf("stopped event should not be blocked by limits, got %v", err)
	}
}

func TestAnnounce_ConnectionLimits_ZeroDisablesLimit(t *testing.T) {
	svc, _, _, _ := setupTrackerWithSettings(map[string]string{
		SettingTrackerMaxPeersPerTorrent: "0",
		SettingTrackerMaxPeersPerUser:    "0",
	})

	// Should succeed with limits disabled (set to 0).
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
		t.Errorf("expected success with limits disabled, got %v", err)
	}
}

func TestAnnounce_ConnectionLimits_NoSettingsUsesDefaults(t *testing.T) {
	// Without site settings service, limits are not enforced.
	svc, _, _, _ := setupTracker()

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
		t.Errorf("expected success without site settings, got %v", err)
	}
}

// --- Mock Group Repository ---

type trackerMockGroupRepo struct {
	mu     sync.Mutex
	groups []*model.Group
}

func newTrackerMockGroupRepo() *trackerMockGroupRepo {
	return &trackerMockGroupRepo{}
}

func (m *trackerMockGroupRepo) addGroup(g *model.Group) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.groups = append(m.groups, g)
}

func (m *trackerMockGroupRepo) GetByID(_ context.Context, id int64) (*model.Group, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, g := range m.groups {
		if g.ID == id {
			return g, nil
		}
	}
	return nil, errors.New("group not found")
}

func (m *trackerMockGroupRepo) List(_ context.Context) ([]model.Group, error) {
	return nil, nil
}

// --- Wait Time Test Helpers ---

func setupTrackerWithWaitTime(settings map[string]string, group *model.Group) (*TrackerService, *trackerMockUserRepo, *trackerMockTorrentRepo, *trackerMockPeerRepo) {
	svc, userRepo, torrentRepo, peerRepo := setupTracker()
	settingsRepo := newMockSiteSettingsRepo()
	for k, v := range settings {
		settingsRepo.settings[k] = &model.SiteSetting{Key: k, Value: v}
	}
	bus := event.NewInMemoryBus()
	siteSettings := NewSiteSettingsService(settingsRepo, bus)
	svc.SetSiteSettings(siteSettings)

	if group != nil {
		groupRepo := newTrackerMockGroupRepo()
		groupRepo.addGroup(group)
		svc.SetGroupRepo(groupRepo)
		// Set the user's group ID to match
		userRepo.mu.Lock()
		userRepo.users[0].GroupID = group.ID
		userRepo.mu.Unlock()
	}

	return svc, userRepo, torrentRepo, peerRepo
}

// --- Wait Time Tests ---

func TestAnnounce_WaitTime_BlocksLowRatioOnNewTorrent(t *testing.T) {
	svc, userRepo, torrentRepo, _ := setupTrackerWithWaitTime(map[string]string{
		SettingWaitTimeEnabled:     "true",
		SettingWaitTimeBypassRatio: "0.95",
	}, nil)

	// Set user to have a low ratio: uploaded 100, downloaded 1000 → ratio = 0.1
	userRepo.mu.Lock()
	userRepo.users[0].Uploaded = 100
	userRepo.users[0].Downloaded = 1000
	userRepo.mu.Unlock()

	// Set torrent as recently uploaded (just now).
	torrentRepo.mu.Lock()
	torrentRepo.torrents[0].CreatedAt = time.Now()
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

	var waitErr *WaitTimeError
	if !errors.As(err, &waitErr) {
		t.Fatalf("expected WaitTimeError, got %v", err)
	}
	if waitErr.RemainingSeconds <= 0 {
		t.Errorf("expected positive remaining seconds, got %d", waitErr.RemainingSeconds)
	}
}

func TestAnnounce_WaitTime_AllowsHighRatioUser(t *testing.T) {
	svc, userRepo, torrentRepo, _ := setupTrackerWithWaitTime(map[string]string{
		SettingWaitTimeEnabled:     "true",
		SettingWaitTimeBypassRatio: "0.95",
	}, nil)

	// High ratio: uploaded 10000, downloaded 1000 → ratio = 10.0
	userRepo.mu.Lock()
	userRepo.users[0].Uploaded = 10000
	userRepo.users[0].Downloaded = 1000
	userRepo.mu.Unlock()

	torrentRepo.mu.Lock()
	torrentRepo.torrents[0].CreatedAt = time.Now()
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

	if err != nil {
		t.Errorf("expected high-ratio user to bypass wait time, got %v", err)
	}
}

func TestAnnounce_WaitTime_AllowsZeroDownloadUser(t *testing.T) {
	svc, userRepo, torrentRepo, _ := setupTrackerWithWaitTime(map[string]string{
		SettingWaitTimeEnabled: "true",
	}, nil)

	// Zero downloaded → infinite ratio → exempt.
	userRepo.mu.Lock()
	userRepo.users[0].Uploaded = 0
	userRepo.users[0].Downloaded = 0
	userRepo.mu.Unlock()

	torrentRepo.mu.Lock()
	torrentRepo.torrents[0].CreatedAt = time.Now()
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

	if err != nil {
		t.Errorf("expected zero-download user to be exempt, got %v", err)
	}
}

func TestAnnounce_WaitTime_AllowsImmuneGroup(t *testing.T) {
	immuneGroup := &model.Group{ID: 5, Name: "VIP", IsImmune: true}
	svc, userRepo, torrentRepo, _ := setupTrackerWithWaitTime(map[string]string{
		SettingWaitTimeEnabled: "true",
	}, immuneGroup)

	// Low ratio but immune group.
	userRepo.mu.Lock()
	userRepo.users[0].Uploaded = 100
	userRepo.users[0].Downloaded = 1000
	userRepo.mu.Unlock()

	torrentRepo.mu.Lock()
	torrentRepo.torrents[0].CreatedAt = time.Now()
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

	if err != nil {
		t.Errorf("expected immune user to bypass wait time, got %v", err)
	}
}

func TestAnnounce_WaitTime_AllowsAdminGroup(t *testing.T) {
	adminGroup := &model.Group{ID: 1, Name: "Admin", IsAdmin: true}
	svc, userRepo, torrentRepo, _ := setupTrackerWithWaitTime(map[string]string{
		SettingWaitTimeEnabled: "true",
	}, adminGroup)

	userRepo.mu.Lock()
	userRepo.users[0].Uploaded = 100
	userRepo.users[0].Downloaded = 1000
	userRepo.mu.Unlock()

	torrentRepo.mu.Lock()
	torrentRepo.torrents[0].CreatedAt = time.Now()
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

	if err != nil {
		t.Errorf("expected admin to bypass wait time, got %v", err)
	}
}

func TestAnnounce_WaitTime_AllowsOldTorrent(t *testing.T) {
	svc, userRepo, torrentRepo, _ := setupTrackerWithWaitTime(map[string]string{
		SettingWaitTimeEnabled: "true",
	}, nil)

	// Low ratio user.
	userRepo.mu.Lock()
	userRepo.users[0].Uploaded = 100
	userRepo.users[0].Downloaded = 1000
	userRepo.mu.Unlock()

	// Torrent uploaded 3 days ago (older than any default tier wait).
	torrentRepo.mu.Lock()
	torrentRepo.torrents[0].CreatedAt = time.Now().Add(-72 * time.Hour)
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

	if err != nil {
		t.Errorf("expected old torrent to pass wait time, got %v", err)
	}
}

func TestAnnounce_WaitTime_AllowsSeeders(t *testing.T) {
	svc, userRepo, torrentRepo, _ := setupTrackerWithWaitTime(map[string]string{
		SettingWaitTimeEnabled: "true",
	}, nil)

	// Low ratio but seeding (left=0).
	userRepo.mu.Lock()
	userRepo.users[0].Uploaded = 100
	userRepo.users[0].Downloaded = 1000
	userRepo.mu.Unlock()

	torrentRepo.mu.Lock()
	torrentRepo.torrents[0].CreatedAt = time.Now()
	torrentRepo.mu.Unlock()

	_, err := svc.Announce(context.Background(), AnnounceRequest{
		Passkey:  testPasskey(),
		InfoHash: testInfoHash(),
		PeerID:   testPeerID(),
		IP:       "192.168.1.1",
		Port:     6881,
		Left:     0, // seeder
		Event:    EventStarted,
	})

	if err != nil {
		t.Errorf("expected seeder to bypass wait time, got %v", err)
	}
}

func TestAnnounce_WaitTime_AllowsExistingPeer(t *testing.T) {
	svc, userRepo, torrentRepo, peerRepo := setupTrackerWithWaitTime(map[string]string{
		SettingWaitTimeEnabled: "true",
	}, nil)

	// Low ratio.
	userRepo.mu.Lock()
	userRepo.users[0].Uploaded = 100
	userRepo.users[0].Downloaded = 1000
	userRepo.mu.Unlock()

	torrentRepo.mu.Lock()
	torrentRepo.torrents[0].CreatedAt = time.Now()
	torrentRepo.mu.Unlock()

	// Pre-fill this peer so it's an existing peer update (re-announce).
	peerRepo.mu.Lock()
	peerRepo.peers = append(peerRepo.peers, &model.Peer{
		ID:        1,
		TorrentID: 1,
		UserID:    1,
		PeerID:    testPeerID(),
		IP:        "192.168.1.1",
		Port:      6881,
		Seeder:    false,
	})
	peerRepo.mu.Unlock()

	_, err := svc.Announce(context.Background(), AnnounceRequest{
		Passkey:    testPasskey(),
		InfoHash:   testInfoHash(),
		PeerID:     testPeerID(),
		IP:         "192.168.1.1",
		Port:       6881,
		Uploaded:   50,
		Downloaded: 100,
		Left:       500,
		Event:      EventEmpty,
	})

	if err != nil {
		t.Errorf("expected existing peer to bypass wait time, got %v", err)
	}
}

func TestAnnounce_WaitTime_DisabledByDefault(t *testing.T) {
	// Wait time not enabled → no blocking.
	svc, userRepo, torrentRepo, _ := setupTrackerWithWaitTime(map[string]string{}, nil)

	userRepo.mu.Lock()
	userRepo.users[0].Uploaded = 100
	userRepo.users[0].Downloaded = 1000
	userRepo.mu.Unlock()

	torrentRepo.mu.Lock()
	torrentRepo.torrents[0].CreatedAt = time.Now()
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

	if err != nil {
		t.Errorf("expected wait time disabled to allow download, got %v", err)
	}
}

func TestAnnounce_WaitTime_CustomTiers(t *testing.T) {
	// Custom tiers: ratio < 0.3 → 24h wait.
	svc, userRepo, torrentRepo, _ := setupTrackerWithWaitTime(map[string]string{
		SettingWaitTimeEnabled:     "true",
		SettingWaitTimeBypassRatio: "0.5",
		SettingWaitTimeTiers:       `[{"ratio":0.3,"hours":24}]`,
	}, nil)

	// ratio = 0.1 (below 0.3 tier)
	userRepo.mu.Lock()
	userRepo.users[0].Uploaded = 100
	userRepo.users[0].Downloaded = 1000
	userRepo.mu.Unlock()

	// Torrent uploaded 12h ago (less than 24h wait).
	torrentRepo.mu.Lock()
	torrentRepo.torrents[0].CreatedAt = time.Now().Add(-12 * time.Hour)
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

	var waitErr *WaitTimeError
	if !errors.As(err, &waitErr) {
		t.Fatalf("expected WaitTimeError with custom tiers, got %v", err)
	}
	// Should be approximately 12h remaining (in seconds).
	if waitErr.RemainingSeconds < 11*3600 || waitErr.RemainingSeconds > 13*3600 {
		t.Errorf("expected ~12h remaining, got %d seconds", waitErr.RemainingSeconds)
	}
}

func TestAnnounce_WaitTime_RatioAboveTierButBelowBypass(t *testing.T) {
	// Tiers: <0.5 → 48h, <0.8 → 12h. Bypass at 0.95.
	// User ratio = 0.6 (above 0.5 but below 0.8) → 12h wait.
	svc, userRepo, torrentRepo, _ := setupTrackerWithWaitTime(map[string]string{
		SettingWaitTimeEnabled:     "true",
		SettingWaitTimeBypassRatio: "0.95",
		SettingWaitTimeTiers:       `[{"ratio":0.5,"hours":48},{"ratio":0.8,"hours":12}]`,
	}, nil)

	userRepo.mu.Lock()
	userRepo.users[0].Uploaded = 600
	userRepo.users[0].Downloaded = 1000
	userRepo.mu.Unlock()

	// Torrent uploaded 6h ago (less than 12h).
	torrentRepo.mu.Lock()
	torrentRepo.torrents[0].CreatedAt = time.Now().Add(-6 * time.Hour)
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

	var waitErr *WaitTimeError
	if !errors.As(err, &waitErr) {
		t.Fatalf("expected WaitTimeError for mid-tier ratio, got %v", err)
	}
}

func TestAnnounce_WaitTime_AllowsUploader(t *testing.T) {
	svc, userRepo, torrentRepo, _ := setupTrackerWithWaitTime(map[string]string{
		SettingWaitTimeEnabled: "true",
	}, nil)

	// Low ratio user who is also the torrent uploader.
	userRepo.mu.Lock()
	userRepo.users[0].Uploaded = 100
	userRepo.users[0].Downloaded = 1000
	userRepo.mu.Unlock()

	torrentRepo.mu.Lock()
	torrentRepo.torrents[0].CreatedAt = time.Now()
	torrentRepo.torrents[0].UploaderID = 1 // same as user ID
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

	if err != nil {
		t.Errorf("expected uploader to bypass wait time, got %v", err)
	}
}

func TestAnnounce_WaitTime_AllowsModeratorGroup(t *testing.T) {
	modGroup := &model.Group{ID: 3, Name: "Moderator", IsModerator: true}
	svc, userRepo, torrentRepo, _ := setupTrackerWithWaitTime(map[string]string{
		SettingWaitTimeEnabled: "true",
	}, modGroup)

	userRepo.mu.Lock()
	userRepo.users[0].Uploaded = 100
	userRepo.users[0].Downloaded = 1000
	userRepo.mu.Unlock()

	torrentRepo.mu.Lock()
	torrentRepo.torrents[0].CreatedAt = time.Now()
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

	if err != nil {
		t.Errorf("expected moderator to bypass wait time, got %v", err)
	}
}

func TestAnnounce_WaitTime_TierBoundaryExactRatio(t *testing.T) {
	// User ratio exactly 0.5 should NOT match the <0.5 tier (48h) but SHOULD match <0.65 tier (24h).
	svc, userRepo, torrentRepo, _ := setupTrackerWithWaitTime(map[string]string{
		SettingWaitTimeEnabled:     "true",
		SettingWaitTimeBypassRatio: "0.95",
	}, nil)

	// ratio = exactly 0.5
	userRepo.mu.Lock()
	userRepo.users[0].Uploaded = 500
	userRepo.users[0].Downloaded = 1000
	userRepo.mu.Unlock()

	// Torrent uploaded 30h ago — older than 24h but less than 48h.
	torrentRepo.mu.Lock()
	torrentRepo.torrents[0].CreatedAt = time.Now().Add(-30 * time.Hour)
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

	// ratio=0.5, so NOT < 0.5 tier. Falls to <0.65 tier (24h). Torrent is 30h old > 24h → allowed.
	if err != nil {
		t.Errorf("expected ratio exactly at tier boundary to use next tier, got %v", err)
	}
}

func TestAnnounce_WaitTime_MalformedTiersFallsBackToDefaults(t *testing.T) {
	svc, userRepo, torrentRepo, _ := setupTrackerWithWaitTime(map[string]string{
		SettingWaitTimeEnabled: "true",
		SettingWaitTimeTiers:   `not valid json`,
	}, nil)

	// Low ratio.
	userRepo.mu.Lock()
	userRepo.users[0].Uploaded = 100
	userRepo.users[0].Downloaded = 1000
	userRepo.mu.Unlock()

	// New torrent.
	torrentRepo.mu.Lock()
	torrentRepo.torrents[0].CreatedAt = time.Now()
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

	// Should fall back to default tiers and block (ratio 0.1 < 0.5 tier → 48h wait).
	var waitErr *WaitTimeError
	if !errors.As(err, &waitErr) {
		t.Fatalf("expected WaitTimeError with malformed tiers (fallback to defaults), got %v", err)
	}
}
