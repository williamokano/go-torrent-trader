package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/handler"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// --- mock report repo for handler tests ---

type mockReportRepo struct {
	mu      sync.Mutex
	reports []*model.Report
	nextID  int64
}

func newMockReportRepo() *mockReportRepo {
	return &mockReportRepo{nextID: 1}
}

func (m *mockReportRepo) Create(_ context.Context, report *model.Report) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	report.ID = m.nextID
	report.CreatedAt = time.Now()
	m.nextID++
	m.reports = append(m.reports, report)
	return nil
}

func (m *mockReportRepo) GetByID(_ context.Context, id int64) (*model.Report, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.reports {
		if r.ID == id {
			return r, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *mockReportRepo) ExistsByReporterAndTorrent(_ context.Context, reporterID int64, torrentID *int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.reports {
		if r.ReporterID == reporterID {
			if torrentID == nil && r.TorrentID == nil {
				return true, nil
			}
			if torrentID != nil && r.TorrentID != nil && *torrentID == *r.TorrentID {
				return true, nil
			}
		}
	}
	return false, nil
}

func (m *mockReportRepo) List(_ context.Context, opts repository.ListReportsOptions) ([]model.Report, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var filtered []model.Report
	for _, r := range m.reports {
		if opts.Status != nil {
			switch *opts.Status {
			case "resolved":
				if !r.Resolved {
					continue
				}
			case "pending":
				if r.Resolved {
					continue
				}
			}
		}
		filtered = append(filtered, *r)
	}

	total := int64(len(filtered))
	page := opts.Page
	if page <= 0 {
		page = 1
	}
	perPage := opts.PerPage
	if perPage <= 0 {
		perPage = 25
	}
	start := (page - 1) * perPage
	if start >= len(filtered) {
		return nil, total, nil
	}
	end := start + perPage
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[start:end], total, nil
}

func (m *mockReportRepo) Resolve(_ context.Context, id, resolvedByUserID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.reports {
		if r.ID == id {
			r.Resolved = true
			r.ResolvedBy = &resolvedByUserID
			now := time.Now()
			r.ResolvedAt = &now
			return nil
		}
	}
	return errors.New("not found")
}

// --- test helpers ---

func setupReportRouter() (http.Handler, service.SessionStore) {
	userRepo := newMockUserRepo()
	reportRepo := newMockReportRepo()
	sessions := service.NewMemorySessionStore()
	authSvc := service.NewAuthService(userRepo, sessions, service.NewMemoryPasswordResetStore(), &service.NoopSender{}, "http://localhost:8080")
	reportSvc := service.NewReportService(reportRepo)

	router := handler.NewRouter(&handler.Deps{
		AuthService:   authSvc,
		SessionStore:  sessions,
		ReportService: reportSvc,
	})
	return router, sessions
}

// --- tests ---

func TestHandleCreateReport_Success(t *testing.T) {
	router, _ := setupReportRouter()
	token := registerAndGetToken(t, router)

	body, _ := json.Marshal(map[string]interface{}{
		"torrent_id": 1,
		"reason":     "fake content",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/reports", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	report := resp["report"].(map[string]interface{})
	if report["reason"] != "fake content" {
		t.Errorf("expected reason 'fake content', got %v", report["reason"])
	}
}

func TestHandleCreateReport_Unauthenticated(t *testing.T) {
	router, _ := setupReportRouter()

	body, _ := json.Marshal(map[string]interface{}{
		"reason": "test",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/reports", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestHandleCreateReport_EmptyReason(t *testing.T) {
	router, _ := setupReportRouter()
	token := registerAndGetToken(t, router)

	body, _ := json.Marshal(map[string]interface{}{
		"reason": "",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/reports", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCreateReport_Duplicate(t *testing.T) {
	router, _ := setupReportRouter()
	token := registerAndGetToken(t, router)

	body, _ := json.Marshal(map[string]interface{}{
		"torrent_id": 1,
		"reason":     "fake content",
	})

	// First report
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/reports", bytes.NewReader(body))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Authorization", "Bearer "+token)
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("first report failed: %d %s", rec1.Code, rec1.Body.String())
	}

	// Duplicate report
	body2, _ := json.Marshal(map[string]interface{}{
		"torrent_id": 1,
		"reason":     "still fake",
	})
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/reports", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+token)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d; body: %s", rec2.Code, rec2.Body.String())
	}
}

func TestHandleListReports_AdminOnly(t *testing.T) {
	router, sessions := setupReportRouter()

	// Regular user (groupID=5) should get 403
	userToken := createSessionWithGroup(sessions, 400, 5)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-admin, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleListReports_AsAdmin(t *testing.T) {
	router, sessions := setupReportRouter()

	// Create a report first as regular user
	userToken := createSessionWithGroup(sessions, 500, 5)
	body, _ := json.Marshal(map[string]interface{}{
		"torrent_id": 1,
		"reason":     "test report",
	})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/reports", bytes.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+userToken)
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create report failed: %d %s", createRec.Code, createRec.Body.String())
	}

	// List as admin
	adminToken := createSessionWithGroup(sessions, 501, 1)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	reports := resp["reports"].([]interface{})
	if len(reports) != 1 {
		t.Errorf("expected 1 report, got %d", len(reports))
	}
	total := resp["total"].(float64)
	if total != 1 {
		t.Errorf("expected total 1, got %v", total)
	}
}

func TestHandleResolveReport_AsAdmin(t *testing.T) {
	router, sessions := setupReportRouter()

	// Create a report
	userToken := createSessionWithGroup(sessions, 600, 5)
	body, _ := json.Marshal(map[string]interface{}{
		"torrent_id": 1,
		"reason":     "resolve test",
	})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/reports", bytes.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+userToken)
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create report failed: %d %s", createRec.Code, createRec.Body.String())
	}

	var createResp map[string]interface{}
	_ = json.Unmarshal(createRec.Body.Bytes(), &createResp)
	report := createResp["report"].(map[string]interface{})
	id := int(report["id"].(float64))

	// Resolve as admin
	adminToken := createSessionWithGroup(sessions, 601, 1)
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/reports/%d/resolve", id), nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleResolveReport_NonAdmin(t *testing.T) {
	router, sessions := setupReportRouter()

	// Create a report
	userToken := createSessionWithGroup(sessions, 700, 5)
	body, _ := json.Marshal(map[string]interface{}{
		"torrent_id": 1,
		"reason":     "non-admin resolve",
	})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/reports", bytes.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+userToken)
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create report failed: %d %s", createRec.Code, createRec.Body.String())
	}

	// Try to resolve as non-admin
	req := httptest.NewRequest(http.MethodPut, "/api/v1/reports/1/resolve", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleResolveReport_NotFound(t *testing.T) {
	router, sessions := setupReportRouter()

	adminToken := createSessionWithGroup(sessions, 800, 1)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/reports/999/resolve", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCreateReport_NilTorrentID(t *testing.T) {
	router, _ := setupReportRouter()
	token := registerAndGetToken(t, router)

	body, _ := json.Marshal(map[string]interface{}{
		"reason": "general site issue",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/reports", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}
}
