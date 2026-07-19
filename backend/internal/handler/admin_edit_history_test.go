package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/handler"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/williamokano/go-torrent-trader/backend/internal/testutil"
)

type memEditHistoryRepo struct {
	entries []model.UserEditHistory
}

// push is how mockUserRepo's UpdateWithHistory/SetStats/SetInvites deliver
// audit entries in this test's fixture — mirroring production, where those
// writes insert into user_edit_history directly rather than through Record.
func (m *memEditHistoryRepo) push(entries []model.UserEditHistory) {
	m.entries = append(m.entries, entries...)
}

func (m *memEditHistoryRepo) Record(_ context.Context, entries []model.UserEditHistory) error {
	m.entries = append(m.entries, entries...)
	return nil
}

func (m *memEditHistoryRepo) ListByUser(_ context.Context, userID int64, limit, offset int) ([]model.UserEditHistory, int64, error) {
	var out []model.UserEditHistory
	for _, e := range m.entries {
		if e.UserID == userID {
			out = append(out, e)
		}
	}
	total := int64(len(out))
	if offset > len(out) {
		offset = len(out)
	}
	out = out[offset:]
	if len(out) > limit {
		out = out[:limit]
	}
	return out, total, nil
}

func setupEditHistoryRouter() (http.Handler, service.SessionStore) {
	userRepo := newMockUserRepo()
	groupRepo := &mockGroupRepo{}
	sessions := testutil.NewMemorySessionStore()
	bus := event.NewInMemoryBus()
	authSvc := service.NewAuthServiceWithTTL(userRepo, sessions, testutil.NewMemoryPasswordResetStore(), &testutil.NoopSender{}, "http://localhost:8080", service.DefaultAccessTokenTTL, service.DefaultRefreshTokenTTL, groupRepo, bus)
	adminSvc := service.NewAdminService(userRepo, groupRepo, bus)
	adminSvc.SetSessionStore(sessions)
	hist := &memEditHistoryRepo{}
	userRepo.historySink = hist
	adminSvc.SetEditHistoryRepo(hist)

	router := handler.NewRouter(&handler.Deps{
		AuthService:  authSvc,
		SessionStore: sessions,
		AdminService: adminSvc,
	})
	return router, sessions
}

func TestHandleListUserEditHistory_RecordsAndReturnsChanges(t *testing.T) {
	router, sessions := setupEditHistoryRouter()

	registerAndGetToken(t, router)
	adminToken := createSessionWithGroup(sessions, 2100, 1)

	// Change uploaded and invites through the admin update endpoint.
	body, _ := json.Marshal(map[string]interface{}{
		"uploaded": 1099511627776,
		"invites":  9,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/1/edit-history", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Entries []map[string]interface{} `json:"entries"`
		Total   int64                    `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 2 || len(resp.Entries) != 2 {
		t.Fatalf("want 2 entries, got total=%d len=%d; body: %s", resp.Total, len(resp.Entries), rec.Body.String())
	}

	byField := map[string]map[string]interface{}{}
	for _, e := range resp.Entries {
		byField[e["field"].(string)] = e
	}
	up, ok := byField["uploaded"]
	if !ok {
		t.Fatalf("no uploaded entry in %s", rec.Body.String())
	}
	if up["old_value"] != "0" || up["new_value"] != "1099511627776" {
		t.Errorf("uploaded old=%v new=%v, want 0 -> 1099511627776", up["old_value"], up["new_value"])
	}
	if up["changed_by"] != float64(2100) {
		t.Errorf("changed_by = %v, want 2100", up["changed_by"])
	}
	if _, ok := byField["invites"]; !ok {
		t.Errorf("no invites entry in %s", rec.Body.String())
	}
}

func TestHandleListUserEditHistory_UserNotFound(t *testing.T) {
	router, sessions := setupEditHistoryRouter()
	adminToken := createSessionWithGroup(sessions, 2101, 1)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/9999/edit-history", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleListUserEditHistory_InvalidID(t *testing.T) {
	router, sessions := setupEditHistoryRouter()
	adminToken := createSessionWithGroup(sessions, 2102, 1)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/abc/edit-history", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleListUserEditHistory_NonAdminForbidden(t *testing.T) {
	router, sessions := setupEditHistoryRouter()
	userToken := createSessionWithGroup(sessions, 2103, 5)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/1/edit-history", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}
