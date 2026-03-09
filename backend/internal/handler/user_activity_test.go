package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
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

// --- torrentLister adapter to satisfy *service.TorrentService indirectly ---
// Since we can't easily construct *service.TorrentService in tests, we test
// via the router with deps set up. Instead, let's test the handler methods
// via direct HTTP requests with Chi context.

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
