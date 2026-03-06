package handler_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/handler"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/zeebo/bencode"
)

// --- Mock repos for announce tests ---

type announceUserRepo struct {
	mu    sync.Mutex
	users []*model.User
}

func (r *announceUserRepo) GetByPasskey(_ context.Context, passkey string) (*model.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, u := range r.users {
		if u.Passkey != nil && *u.Passkey == passkey {
			return u, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (r *announceUserRepo) GetByID(context.Context, int64) (*model.User, error) {
	return nil, sql.ErrNoRows
}
func (r *announceUserRepo) GetByUsername(context.Context, string) (*model.User, error) {
	return nil, sql.ErrNoRows
}
func (r *announceUserRepo) GetByEmail(context.Context, string) (*model.User, error) {
	return nil, sql.ErrNoRows
}
func (r *announceUserRepo) Count(context.Context) (int64, error)              { return 0, nil }
func (r *announceUserRepo) Create(context.Context, *model.User) error         { return nil }
func (r *announceUserRepo) Update(context.Context, *model.User) error         { return nil }
func (r *announceUserRepo) IncrementStats(context.Context, int64, int64, int64) error {
	return nil
}

type announceTorrentRepo struct {
	mu       sync.Mutex
	torrents []*model.Torrent
}

func (r *announceTorrentRepo) GetByInfoHash(_ context.Context, infoHash []byte) (*model.Torrent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.torrents {
		if string(t.InfoHash) == string(infoHash) {
			return t, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (r *announceTorrentRepo) GetByID(context.Context, int64) (*model.Torrent, error) {
	return nil, sql.ErrNoRows
}
func (r *announceTorrentRepo) List(context.Context, repository.ListTorrentsOptions) ([]model.Torrent, int64, error) {
	return nil, 0, nil
}
func (r *announceTorrentRepo) Create(context.Context, *model.Torrent) error         { return nil }
func (r *announceTorrentRepo) Update(context.Context, *model.Torrent) error         { return nil }
func (r *announceTorrentRepo) Delete(context.Context, int64) error                  { return nil }
func (r *announceTorrentRepo) IncrementSeeders(context.Context, int64, int) error   { return nil }
func (r *announceTorrentRepo) IncrementLeechers(context.Context, int64, int) error  { return nil }
func (r *announceTorrentRepo) IncrementTimesCompleted(context.Context, int64) error { return nil }

type announcePeerRepo struct {
	mu    sync.Mutex
	peers []*model.Peer
}

func (r *announcePeerRepo) GetByTorrentAndUser(_ context.Context, torrentID, userID int64) (*model.Peer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.peers {
		if p.TorrentID == torrentID && p.UserID == userID {
			return p, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (r *announcePeerRepo) ListByTorrent(_ context.Context, torrentID int64, limit int) ([]model.Peer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []model.Peer
	for _, p := range r.peers {
		if p.TorrentID == torrentID {
			result = append(result, *p)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (r *announcePeerRepo) Upsert(_ context.Context, peer *model.Peer) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, p := range r.peers {
		if p.TorrentID == peer.TorrentID && p.UserID == peer.UserID && string(p.PeerID) == string(peer.PeerID) {
			r.peers[i] = peer
			return nil
		}
	}
	peer.ID = int64(len(r.peers) + 1)
	r.peers = append(r.peers, peer)
	return nil
}

func (r *announcePeerRepo) Delete(_ context.Context, torrentID, userID int64, peerID []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, p := range r.peers {
		if p.TorrentID == torrentID && p.UserID == userID && string(p.PeerID) == string(peerID) {
			r.peers = append(r.peers[:i], r.peers[i+1:]...)
			return nil
		}
	}
	return sql.ErrNoRows
}

func (r *announcePeerRepo) DeleteStale(context.Context, time.Time) (int64, error) { return 0, nil }

// --- Helpers ---

type announceErrorResponse struct {
	FailureReason string `bencode:"failure reason"`
}

func announcePasskey() string { return "testpasskey123456789" }

func announceInfoHash() []byte {
	h := make([]byte, 20)
	for i := range h {
		h[i] = byte(i + 10)
	}
	return h
}

func announcePeerID() []byte {
	p := make([]byte, 20)
	for i := range p {
		p[i] = byte(i + 50)
	}
	return p
}

func setupAnnounceRouter() http.Handler {
	pk := announcePasskey()
	userRepo := &announceUserRepo{
		users: []*model.User{
			{ID: 1, Enabled: true, Passkey: &pk},
		},
	}
	torrentRepo := &announceTorrentRepo{
		torrents: []*model.Torrent{
			{ID: 1, InfoHash: announceInfoHash(), Seeders: 5, Leechers: 3},
		},
	}
	peerRepo := &announcePeerRepo{}

	trackerSvc := service.NewTrackerService(userRepo, torrentRepo, peerRepo)
	deps := &handler.Deps{
		TrackerService: trackerSvc,
	}
	return handler.NewRouter(deps)
}

func buildAnnounceURL(passkey string, infoHash, peerID []byte, port int, uploaded, downloaded, left int64, event string) string {
	params := url.Values{}
	params.Set("passkey", passkey)
	params.Set("info_hash", string(infoHash))
	params.Set("peer_id", string(peerID))
	params.Set("port", fmt.Sprintf("%d", port))
	params.Set("uploaded", fmt.Sprintf("%d", uploaded))
	params.Set("downloaded", fmt.Sprintf("%d", downloaded))
	params.Set("left", fmt.Sprintf("%d", left))
	if event != "" {
		params.Set("event", event)
	}
	return "/announce?" + params.Encode()
}

// --- Tests ---

func TestAnnounce_ValidRequest(t *testing.T) {
	router := setupAnnounceRouter()

	target := buildAnnounceURL(announcePasskey(), announceInfoHash(), announcePeerID(), 6881, 0, 0, 1000, "started")
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp service.AnnounceResponse
	if err := bencode.DecodeBytes(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Interval != service.DefaultInterval {
		t.Errorf("expected interval %d, got %d", service.DefaultInterval, resp.Interval)
	}
	if resp.MinInterval != service.DefaultMinInterval {
		t.Errorf("expected min interval %d, got %d", service.DefaultMinInterval, resp.MinInterval)
	}
}

func TestAnnounce_MissingPasskey(t *testing.T) {
	router := setupAnnounceRouter()

	params := url.Values{}
	params.Set("info_hash", string(announceInfoHash()))
	params.Set("peer_id", string(announcePeerID()))
	params.Set("port", "6881")
	params.Set("uploaded", "0")
	params.Set("downloaded", "0")
	params.Set("left", "1000")
	params.Set("event", "started")

	req := httptest.NewRequest(http.MethodGet, "/announce?"+params.Encode(), nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (bencoded error), got %d", rec.Code)
	}

	var resp announceErrorResponse
	if err := bencode.DecodeBytes(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode error: %v", err)
	}
	if resp.FailureReason != "missing passkey" {
		t.Errorf("expected 'missing passkey', got %q", resp.FailureReason)
	}
}

func TestAnnounce_InvalidPasskey(t *testing.T) {
	router := setupAnnounceRouter()

	target := buildAnnounceURL("badpasskey0123456789", announceInfoHash(), announcePeerID(), 6881, 0, 0, 1000, "started")
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	var resp announceErrorResponse
	if err := bencode.DecodeBytes(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode error: %v", err)
	}
	if resp.FailureReason != "invalid passkey" {
		t.Errorf("expected 'invalid passkey', got %q", resp.FailureReason)
	}
}

func TestAnnounce_MissingInfoHash(t *testing.T) {
	router := setupAnnounceRouter()

	params := url.Values{}
	params.Set("passkey", announcePasskey())
	params.Set("peer_id", string(announcePeerID()))
	params.Set("port", "6881")
	params.Set("uploaded", "0")
	params.Set("downloaded", "0")
	params.Set("left", "1000")
	params.Set("event", "started")

	req := httptest.NewRequest(http.MethodGet, "/announce?"+params.Encode(), nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	var resp announceErrorResponse
	if err := bencode.DecodeBytes(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode error: %v", err)
	}
	if resp.FailureReason != "missing info_hash" {
		t.Errorf("expected 'missing info_hash', got %q", resp.FailureReason)
	}
}

func TestAnnounce_MissingPort(t *testing.T) {
	router := setupAnnounceRouter()

	params := url.Values{}
	params.Set("passkey", announcePasskey())
	params.Set("info_hash", string(announceInfoHash()))
	params.Set("peer_id", string(announcePeerID()))
	params.Set("uploaded", "0")
	params.Set("downloaded", "0")
	params.Set("left", "1000")
	params.Set("event", "started")

	req := httptest.NewRequest(http.MethodGet, "/announce?"+params.Encode(), nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	var resp announceErrorResponse
	if err := bencode.DecodeBytes(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode error: %v", err)
	}
	if resp.FailureReason != "missing port" {
		t.Errorf("expected 'missing port', got %q", resp.FailureReason)
	}
}

func TestAnnounce_InvalidEvent(t *testing.T) {
	router := setupAnnounceRouter()

	target := buildAnnounceURL(announcePasskey(), announceInfoHash(), announcePeerID(), 6881, 0, 0, 1000, "invalid_event")
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	var resp announceErrorResponse
	if err := bencode.DecodeBytes(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode error: %v", err)
	}
	if resp.FailureReason != "invalid event" {
		t.Errorf("expected 'invalid event', got %q", resp.FailureReason)
	}
}

func TestAnnounce_StoppedEvent(t *testing.T) {
	router := setupAnnounceRouter()

	// First, start.
	target := buildAnnounceURL(announcePasskey(), announceInfoHash(), announcePeerID(), 6881, 0, 0, 1000, "started")
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("start: expected 200, got %d", rec.Code)
	}

	// Then stop.
	target = buildAnnounceURL(announcePasskey(), announceInfoHash(), announcePeerID(), 6881, 100, 200, 800, "stopped")
	req = httptest.NewRequest(http.MethodGet, target, nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("stop: expected 200, got %d", rec.Code)
	}

	var resp service.AnnounceResponse
	if err := bencode.DecodeBytes(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Response should still be valid bencoded.
	if resp.Interval != service.DefaultInterval {
		t.Errorf("expected interval %d, got %d", service.DefaultInterval, resp.Interval)
	}
}

func TestAnnounce_CompletedEvent(t *testing.T) {
	router := setupAnnounceRouter()

	// Start as leecher.
	target := buildAnnounceURL(announcePasskey(), announceInfoHash(), announcePeerID(), 6881, 0, 0, 1000, "started")
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("start: expected 200, got %d", rec.Code)
	}

	// Complete.
	target = buildAnnounceURL(announcePasskey(), announceInfoHash(), announcePeerID(), 6881, 500, 1000, 0, "completed")
	req = httptest.NewRequest(http.MethodGet, target, nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("completed: expected 200, got %d", rec.Code)
	}

	var resp service.AnnounceResponse
	if err := bencode.DecodeBytes(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Interval != service.DefaultInterval {
		t.Errorf("expected interval %d, got %d", service.DefaultInterval, resp.Interval)
	}
}

func TestAnnounce_EmptyEvent(t *testing.T) {
	router := setupAnnounceRouter()

	// Regular announce without event param.
	target := buildAnnounceURL(announcePasskey(), announceInfoHash(), announcePeerID(), 6881, 100, 200, 800, "")
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp service.AnnounceResponse
	if err := bencode.DecodeBytes(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
}
