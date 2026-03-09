package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/williamokano/go-torrent-trader/backend/internal/storage"
)

// --- mock PeerRepo for activity tests ---

type mockActivityPeerRepo struct {
	seedingPeers  []repository.PeerWithTorrent
	seedingTotal  int64
	leechingPeers []repository.PeerWithTorrent
	leechingTotal int64
}

func (m *mockActivityPeerRepo) GetByTorrentAndUser(context.Context, int64, int64) (*model.Peer, error) {
	return nil, nil
}
func (m *mockActivityPeerRepo) GetByTorrentUserAndPeerID(context.Context, int64, int64, []byte) (*model.Peer, error) {
	return nil, nil
}
func (m *mockActivityPeerRepo) ListByTorrent(context.Context, int64, int) ([]model.Peer, error) {
	return nil, nil
}
func (m *mockActivityPeerRepo) CountByUser(context.Context, int64) (int, int, error) {
	return 0, 0, nil
}
func (m *mockActivityPeerRepo) Upsert(context.Context, *model.Peer) error { return nil }
func (m *mockActivityPeerRepo) Delete(context.Context, int64, int64, []byte) error {
	return nil
}
func (m *mockActivityPeerRepo) DeleteStale(context.Context, time.Time) (int64, error) {
	return 0, nil
}
func (m *mockActivityPeerRepo) ListByUserSeeding(_ context.Context, _ int64, _, _ int) ([]repository.PeerWithTorrent, int64, error) {
	return m.seedingPeers, m.seedingTotal, nil
}
func (m *mockActivityPeerRepo) ListByUserLeeching(_ context.Context, _ int64, _, _ int) ([]repository.PeerWithTorrent, int64, error) {
	return m.leechingPeers, m.leechingTotal, nil
}

// --- mock TransferHistoryRepo ---

type mockTransferHistoryRepo struct {
	history []repository.TransferHistoryWithTorrent
	total   int64
}

func (m *mockTransferHistoryRepo) Upsert(context.Context, *model.TransferHistory) error {
	return nil
}
func (m *mockTransferHistoryRepo) ListByUser(_ context.Context, _ int64, _, _ int) ([]repository.TransferHistoryWithTorrent, int64, error) {
	return m.history, m.total, nil
}

// --- mock TorrentRepo for HandleUserTorrents tests ---

type mockActivityTorrentRepo struct {
	torrents []model.Torrent
	total    int64
}

func (m *mockActivityTorrentRepo) GetByID(context.Context, int64) (*model.Torrent, error) {
	return nil, nil
}
func (m *mockActivityTorrentRepo) GetByInfoHash(context.Context, []byte) (*model.Torrent, error) {
	return nil, nil
}
func (m *mockActivityTorrentRepo) List(_ context.Context, _ repository.ListTorrentsOptions) ([]model.Torrent, int64, error) {
	return m.torrents, m.total, nil
}
func (m *mockActivityTorrentRepo) ListByUploader(context.Context, int64, int) ([]model.Torrent, error) {
	return nil, nil
}
func (m *mockActivityTorrentRepo) Create(context.Context, *model.Torrent) error  { return nil }
func (m *mockActivityTorrentRepo) Update(context.Context, *model.Torrent) error  { return nil }
func (m *mockActivityTorrentRepo) Delete(context.Context, int64) error           { return nil }
func (m *mockActivityTorrentRepo) IncrementSeeders(context.Context, int64, int) error {
	return nil
}
func (m *mockActivityTorrentRepo) IncrementLeechers(context.Context, int64, int) error {
	return nil
}
func (m *mockActivityTorrentRepo) IncrementTimesCompleted(context.Context, int64) error {
	return nil
}

// --- mock UserRepo (minimal, for TorrentService construction) ---

type mockActivityUserRepo struct{}

func (m *mockActivityUserRepo) GetByID(context.Context, int64) (*model.User, error)        { return nil, nil }
func (m *mockActivityUserRepo) GetByUsername(context.Context, string) (*model.User, error)  { return nil, nil }
func (m *mockActivityUserRepo) GetByEmail(context.Context, string) (*model.User, error)     { return nil, nil }
func (m *mockActivityUserRepo) GetByPasskey(context.Context, string) (*model.User, error)   { return nil, nil }
func (m *mockActivityUserRepo) Count(context.Context) (int64, error)                        { return 0, nil }
func (m *mockActivityUserRepo) Create(context.Context, *model.User) error                   { return nil }
func (m *mockActivityUserRepo) Update(context.Context, *model.User) error                   { return nil }
func (m *mockActivityUserRepo) IncrementStats(context.Context, int64, int64, int64) error   { return nil }
func (m *mockActivityUserRepo) List(context.Context, repository.ListUsersOptions) ([]model.User, int64, error) {
	return nil, 0, nil
}
func (m *mockActivityUserRepo) ListStaff(context.Context) ([]model.User, error) { return nil, nil }
func (m *mockActivityUserRepo) UpdateLastAccess(context.Context, int64) error    { return nil }

// --- mock FileStorage (minimal) ---

type mockActivityStorage struct{}

func (m *mockActivityStorage) Put(context.Context, string, io.Reader) error        { return nil }
func (m *mockActivityStorage) Get(context.Context, string) (io.ReadCloser, error)  { return nil, nil }
func (m *mockActivityStorage) Delete(context.Context, string) error                { return nil }
func (m *mockActivityStorage) Exists(context.Context, string) (bool, error)        { return false, nil }
func (m *mockActivityStorage) URL(context.Context, string) (string, error)         { return "", nil }

// --- mock EventBus (minimal) ---

type mockActivityBus struct{}

func (m *mockActivityBus) Publish(context.Context, event.Event) {}
func (m *mockActivityBus) Subscribe(event.Type, event.Handler)  {}

// --- mock ReseedRequestRepo (minimal) ---

type mockActivityReseedRepo struct{}

func (m *mockActivityReseedRepo) Create(context.Context, *model.ReseedRequest) error         { return nil }
func (m *mockActivityReseedRepo) ExistsByTorrentAndUser(context.Context, int64, int64) (bool, error) {
	return false, nil
}
func (m *mockActivityReseedRepo) CountByTorrent(context.Context, int64) (int, error) { return 0, nil }

// Ensure mock interfaces are satisfied at compile time.
var (
	_ repository.TorrentRepository      = (*mockActivityTorrentRepo)(nil)
	_ repository.UserRepository          = (*mockActivityUserRepo)(nil)
	_ storage.FileStorage                = (*mockActivityStorage)(nil)
	_ event.Bus                          = (*mockActivityBus)(nil)
	_ repository.ReseedRequestRepository = (*mockActivityReseedRepo)(nil)
)

func newTestTorrentService(repo *mockActivityTorrentRepo) *service.TorrentService {
	return service.NewTorrentService(
		nil, // db — not used by List
		repo,
		&mockActivityUserRepo{},
		&mockActivityStorage{},
		service.TorrentServiceConfig{},
		&mockActivityBus{},
		&mockActivityReseedRepo{},
	)
}

func withChiURLParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func withAuth(r *http.Request, userID int64, isStaff bool) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.UserIDKey, userID)
	perms := model.Permissions{IsAdmin: isStaff, IsModerator: isStaff}
	ctx = context.WithValue(ctx, middleware.PermissionsKey, perms)
	return r.WithContext(ctx)
}

func TestHandleUserActivity_Forbidden(t *testing.T) {
	peerRepo := &mockActivityPeerRepo{}
	transferRepo := &mockTransferHistoryRepo{}

	h := &UserActivityHandler{
		peerRepo:     peerRepo,
		transferRepo: transferRepo,
	}

	req := httptest.NewRequest("GET", "/api/v1/users/99/activity?tab=seeding", nil)
	req = withChiURLParam(req, "id", "99")
	req = withAuth(req, 42, false) // viewer is NOT the owner and NOT staff

	rr := httptest.NewRecorder()
	h.HandleUserActivity(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestHandleUserActivity_OwnerCanAccess(t *testing.T) {
	peerRepo := &mockActivityPeerRepo{
		seedingPeers: []repository.PeerWithTorrent{
			{
				Peer:        model.Peer{TorrentID: 1, Uploaded: 1000, Downloaded: 500, Seeder: true, LastAnnounce: time.Now()},
				TorrentName: "Test Torrent",
			},
		},
		seedingTotal: 1,
	}
	transferRepo := &mockTransferHistoryRepo{}

	h := &UserActivityHandler{
		peerRepo:     peerRepo,
		transferRepo: transferRepo,
	}

	req := httptest.NewRequest("GET", "/api/v1/users/42/activity?tab=seeding", nil)
	req = withChiURLParam(req, "id", "42")
	req = withAuth(req, 42, false) // viewer IS the owner

	rr := httptest.NewRecorder()
	h.HandleUserActivity(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	activity, ok := body["activity"].([]interface{})
	if !ok {
		t.Fatal("expected activity array in response")
	}
	if len(activity) != 1 {
		t.Errorf("expected 1 activity item, got %d", len(activity))
	}
}

func TestHandleUserActivity_StaffCanAccess(t *testing.T) {
	peerRepo := &mockActivityPeerRepo{}
	transferRepo := &mockTransferHistoryRepo{
		history: []repository.TransferHistoryWithTorrent{
			{
				TransferHistory: model.TransferHistory{TorrentID: 1, Uploaded: 2000, Downloaded: 1000, Seeder: true, CompletedAt: time.Now(), LastAnnounce: time.Now()},
				TorrentName:     "Completed Torrent",
			},
		},
		total: 1,
	}

	h := &UserActivityHandler{
		peerRepo:     peerRepo,
		transferRepo: transferRepo,
	}

	req := httptest.NewRequest("GET", "/api/v1/users/99/activity?tab=history", nil)
	req = withChiURLParam(req, "id", "99")
	req = withAuth(req, 1, true) // viewer is staff

	rr := httptest.NewRecorder()
	h.HandleUserActivity(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestHandleUserActivity_InvalidTab(t *testing.T) {
	h := &UserActivityHandler{
		peerRepo:     &mockActivityPeerRepo{},
		transferRepo: &mockTransferHistoryRepo{},
	}

	req := httptest.NewRequest("GET", "/api/v1/users/42/activity?tab=invalid", nil)
	req = withChiURLParam(req, "id", "42")
	req = withAuth(req, 42, false)

	rr := httptest.NewRecorder()
	h.HandleUserActivity(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// --- HandleUserTorrents tests ---

func TestHandleUserTorrents_Success(t *testing.T) {
	torrentRepo := &mockActivityTorrentRepo{
		torrents: []model.Torrent{
			{ID: 1, Name: "Public Torrent", Size: 1024, Seeders: 5, Leechers: 2, TimesCompleted: 10, CategoryName: "Movies", UploaderID: 99},
		},
		total: 1,
	}

	h := &UserActivityHandler{
		torrentSvc:   newTestTorrentService(torrentRepo),
		peerRepo:     &mockActivityPeerRepo{},
		transferRepo: &mockTransferHistoryRepo{},
	}

	req := httptest.NewRequest("GET", "/api/v1/users/99/torrents", nil)
	req = withChiURLParam(req, "id", "99")
	// No auth — endpoint is public

	rr := httptest.NewRecorder()
	h.HandleUserTorrents(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	torrents, ok := body["torrents"].([]interface{})
	if !ok {
		t.Fatal("expected torrents array in response")
	}
	if len(torrents) != 1 {
		t.Errorf("expected 1 torrent, got %d", len(torrents))
	}
}

func TestHandleUserTorrents_AnonymousFilteredForNonOwner(t *testing.T) {
	torrentRepo := &mockActivityTorrentRepo{
		torrents: []model.Torrent{
			{ID: 1, Name: "Public Torrent", UploaderID: 99, Anonymous: false},
			{ID: 2, Name: "Anonymous Torrent", UploaderID: 99, Anonymous: true},
		},
		total: 2,
	}

	h := &UserActivityHandler{
		torrentSvc:   newTestTorrentService(torrentRepo),
		peerRepo:     &mockActivityPeerRepo{},
		transferRepo: &mockTransferHistoryRepo{},
	}

	// Viewer is authenticated but not the owner and not staff
	req := httptest.NewRequest("GET", "/api/v1/users/99/torrents", nil)
	req = withChiURLParam(req, "id", "99")
	req = withAuth(req, 42, false)

	rr := httptest.NewRecorder()
	h.HandleUserTorrents(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	torrents := body["torrents"].([]interface{})
	if len(torrents) != 1 {
		t.Errorf("expected 1 torrent (anonymous filtered out), got %d", len(torrents))
	}

	// Total should be adjusted
	total := int(body["total"].(float64))
	if total != 1 {
		t.Errorf("expected total=1 after filtering, got %d", total)
	}
}

func TestHandleUserTorrents_AnonymousVisibleToOwner(t *testing.T) {
	torrentRepo := &mockActivityTorrentRepo{
		torrents: []model.Torrent{
			{ID: 1, Name: "Public Torrent", UploaderID: 99, Anonymous: false},
			{ID: 2, Name: "Anonymous Torrent", UploaderID: 99, Anonymous: true},
		},
		total: 2,
	}

	h := &UserActivityHandler{
		torrentSvc:   newTestTorrentService(torrentRepo),
		peerRepo:     &mockActivityPeerRepo{},
		transferRepo: &mockTransferHistoryRepo{},
	}

	// Viewer IS the profile owner
	req := httptest.NewRequest("GET", "/api/v1/users/99/torrents", nil)
	req = withChiURLParam(req, "id", "99")
	req = withAuth(req, 99, false)

	rr := httptest.NewRecorder()
	h.HandleUserTorrents(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	torrents := body["torrents"].([]interface{})
	if len(torrents) != 2 {
		t.Errorf("expected 2 torrents (owner can see anonymous), got %d", len(torrents))
	}
}

func TestHandleUserTorrents_AnonymousVisibleToStaff(t *testing.T) {
	torrentRepo := &mockActivityTorrentRepo{
		torrents: []model.Torrent{
			{ID: 1, Name: "Public Torrent", UploaderID: 99, Anonymous: false},
			{ID: 2, Name: "Anonymous Torrent", UploaderID: 99, Anonymous: true},
		},
		total: 2,
	}

	h := &UserActivityHandler{
		torrentSvc:   newTestTorrentService(torrentRepo),
		peerRepo:     &mockActivityPeerRepo{},
		transferRepo: &mockTransferHistoryRepo{},
	}

	// Viewer is staff but not the owner
	req := httptest.NewRequest("GET", "/api/v1/users/99/torrents", nil)
	req = withChiURLParam(req, "id", "99")
	req = withAuth(req, 1, true)

	rr := httptest.NewRecorder()
	h.HandleUserTorrents(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	torrents := body["torrents"].([]interface{})
	if len(torrents) != 2 {
		t.Errorf("expected 2 torrents (staff can see anonymous), got %d", len(torrents))
	}
}

func TestHandleUserTorrents_Pagination(t *testing.T) {
	torrentRepo := &mockActivityTorrentRepo{
		torrents: []model.Torrent{
			{ID: 1, Name: "Torrent 1", UploaderID: 99},
		},
		total: 50, // 50 total but only 1 on this page
	}

	h := &UserActivityHandler{
		torrentSvc:   newTestTorrentService(torrentRepo),
		peerRepo:     &mockActivityPeerRepo{},
		transferRepo: &mockTransferHistoryRepo{},
	}

	req := httptest.NewRequest("GET", "/api/v1/users/99/torrents?page=2&per_page=10", nil)
	req = withChiURLParam(req, "id", "99")

	rr := httptest.NewRecorder()
	h.HandleUserTorrents(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	page := int(body["page"].(float64))
	perPage := int(body["per_page"].(float64))
	if page != 2 {
		t.Errorf("expected page=2, got %d", page)
	}
	if perPage != 10 {
		t.Errorf("expected per_page=10, got %d", perPage)
	}
}

func TestHandleUserTorrents_NoAuthAnonymousFiltered(t *testing.T) {
	torrentRepo := &mockActivityTorrentRepo{
		torrents: []model.Torrent{
			{ID: 1, Name: "Public Torrent", UploaderID: 99, Anonymous: false},
			{ID: 2, Name: "Anonymous Torrent", UploaderID: 99, Anonymous: true},
		},
		total: 2,
	}

	h := &UserActivityHandler{
		torrentSvc:   newTestTorrentService(torrentRepo),
		peerRepo:     &mockActivityPeerRepo{},
		transferRepo: &mockTransferHistoryRepo{},
	}

	// No auth context at all — anonymous user
	req := httptest.NewRequest("GET", "/api/v1/users/99/torrents", nil)
	req = withChiURLParam(req, "id", "99")

	rr := httptest.NewRecorder()
	h.HandleUserTorrents(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var body map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	torrents := body["torrents"].([]interface{})
	if len(torrents) != 1 {
		t.Errorf("expected 1 torrent (no auth, anonymous filtered), got %d", len(torrents))
	}
}

func TestSafeRatio_InfiniteReturnsSentinel(t *testing.T) {
	ratio := safeRatio(1000, 0)
	if ratio != -1 {
		t.Errorf("expected -1 sentinel for infinite ratio, got %f", ratio)
	}
}

func TestSafeRatio_Zero(t *testing.T) {
	ratio := safeRatio(0, 0)
	if ratio != 0 {
		t.Errorf("expected 0, got %f", ratio)
	}
}

func TestSafeRatio_Normal(t *testing.T) {
	ratio := safeRatio(1000, 500)
	if ratio != 2.0 {
		t.Errorf("expected 2.0, got %f", ratio)
	}
}
