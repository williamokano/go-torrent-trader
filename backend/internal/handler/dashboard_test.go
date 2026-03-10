package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/handler"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/williamokano/go-torrent-trader/backend/internal/testutil"
)

// mockDashboardRepo implements repository.DashboardRepository for testing.
type mockDashboardRepo struct {
	stats *repository.DashboardStats
	err   error
}

func (m *mockDashboardRepo) GetStats(_ context.Context) (*repository.DashboardStats, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.stats, nil
}

// mockActivityLogRepo implements repository.ActivityLogRepository for testing.
type mockActivityLogRepo struct {
	logs []model.ActivityLog
	err  error
}

func (m *mockActivityLogRepo) Create(_ context.Context, _ *model.ActivityLog) error {
	return nil
}

func (m *mockActivityLogRepo) List(_ context.Context, _ repository.ListActivityLogsOptions) ([]model.ActivityLog, int64, error) {
	if m.err != nil {
		return nil, 0, m.err
	}
	return m.logs, int64(len(m.logs)), nil
}

func setupDashboardRouter(dashRepo repository.DashboardRepository, logRepo repository.ActivityLogRepository) (http.Handler, service.SessionStore) {
	userRepo := newMockUserRepo()
	groupRepo := &mockGroupRepo{}
	sessions := testutil.NewMemorySessionStore()
	bus := event.NewInMemoryBus()
	authSvc := service.NewAuthServiceWithTTL(userRepo, sessions, testutil.NewMemoryPasswordResetStore(), &testutil.NoopSender{}, "http://localhost:8080", service.DefaultAccessTokenTTL, service.DefaultRefreshTokenTTL, groupRepo, bus)
	activityLogSvc := service.NewActivityLogService(logRepo)

	router := handler.NewRouter(&handler.Deps{
		AuthService:        authSvc,
		SessionStore:       sessions,
		DashboardRepo:      dashRepo,
		ActivityLogService: activityLogSvc,
	})
	return router, sessions
}

func TestHandleDashboard_HappyPath(t *testing.T) {
	dashRepo := &mockDashboardRepo{
		stats: &repository.DashboardStats{
			UsersTotal:     100,
			UsersToday:     5,
			UsersWeek:      20,
			TorrentsTotal:  500,
			TorrentsToday:  10,
			PeersTotal:     300,
			PeersSeeders:   200,
			PeersLeechers:  100,
			PendingReports: 3,
			ActiveWarnings: 2,
			ActiveMutes:    1,
		},
	}
	logRepo := &mockActivityLogRepo{
		logs: []model.ActivityLog{
			{ID: 1, EventType: "user_login", Message: "User logged in"},
		},
	}

	router, sessions := setupDashboardRouter(dashRepo, logRepo)
	adminToken := createSessionWithGroup(sessions, 9001, 1)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp handler.DashboardStats
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Users.Total != 100 {
		t.Errorf("expected users total 100, got %d", resp.Users.Total)
	}
	if resp.Torrents.Total != 500 {
		t.Errorf("expected torrents total 500, got %d", resp.Torrents.Total)
	}
	if resp.Peers.Seeders != 200 {
		t.Errorf("expected seeders 200, got %d", resp.Peers.Seeders)
	}
	if resp.PendingReports != 3 {
		t.Errorf("expected pending reports 3, got %d", resp.PendingReports)
	}
	if len(resp.RecentActivity) != 1 {
		t.Errorf("expected 1 activity entry, got %d", len(resp.RecentActivity))
	}
}

func TestHandleDashboard_StatsError(t *testing.T) {
	dashRepo := &mockDashboardRepo{
		err: errors.New("db connection failed"),
	}
	logRepo := &mockActivityLogRepo{}

	router, sessions := setupDashboardRouter(dashRepo, logRepo)
	adminToken := createSessionWithGroup(sessions, 9002, 1)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDashboard_NonAdmin(t *testing.T) {
	dashRepo := &mockDashboardRepo{
		stats: &repository.DashboardStats{},
	}
	logRepo := &mockActivityLogRepo{}

	router, sessions := setupDashboardRouter(dashRepo, logRepo)
	userToken := createSessionWithGroup(sessions, 9003, 5) // group 5 = regular user

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-admin, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDashboard_MetadataNotDoubleEncoded(t *testing.T) {
	dashRepo := &mockDashboardRepo{
		stats: &repository.DashboardStats{},
	}
	meta := `{"torrent_id":42,"name":"test"}`
	logRepo := &mockActivityLogRepo{
		logs: []model.ActivityLog{
			{ID: 1, EventType: "torrent_upload", Message: "Uploaded torrent", Metadata: &meta},
		},
	}

	router, sessions := setupDashboardRouter(dashRepo, logRepo)
	adminToken := createSessionWithGroup(sessions, 9004, 1)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Parse the full response to check that metadata is a nested object, not a string
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	var activities []map[string]json.RawMessage
	if err := json.Unmarshal(raw["recent_activity"], &activities); err != nil {
		t.Fatalf("failed to unmarshal recent_activity: %v", err)
	}

	if len(activities) == 0 {
		t.Fatal("expected at least 1 activity entry")
	}

	metaRaw := activities[0]["metadata"]
	// If metadata was double-encoded, it would start with a quote character (string)
	// It should start with { (object)
	if len(metaRaw) == 0 || metaRaw[0] != '{' {
		t.Errorf("expected metadata to be a JSON object, got: %s", string(metaRaw))
	}

	var metaObj map[string]interface{}
	if err := json.Unmarshal(metaRaw, &metaObj); err != nil {
		t.Fatalf("metadata is not valid JSON object: %v; raw: %s", err, string(metaRaw))
	}
	if metaObj["torrent_id"] != float64(42) {
		t.Errorf("expected torrent_id 42, got %v", metaObj["torrent_id"])
	}
}
