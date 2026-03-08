package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/handler"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/williamokano/go-torrent-trader/backend/internal/testutil"
)

// mockCategoryRepo is an in-memory category repository for handler tests.
type mockCategoryRepo struct {
	mu            sync.Mutex
	categories    []*model.Category
	nextID        int64
	torrentCounts map[int64]int64
}

func newMockCategoryRepo() *mockCategoryRepo {
	return &mockCategoryRepo{
		nextID:        1,
		torrentCounts: make(map[int64]int64),
	}
}

func (m *mockCategoryRepo) GetByID(_ context.Context, id int64) (*model.Category, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.categories {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *mockCategoryRepo) List(_ context.Context) ([]model.Category, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []model.Category
	for _, c := range m.categories {
		result = append(result, *c)
	}
	return result, nil
}

func (m *mockCategoryRepo) Create(_ context.Context, cat *model.Category) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cat.ID = m.nextID
	m.nextID++
	m.categories = append(m.categories, cat)
	return nil
}

func (m *mockCategoryRepo) Update(_ context.Context, cat *model.Category) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, c := range m.categories {
		if c.ID == cat.ID {
			m.categories[i] = cat
			return nil
		}
	}
	return errors.New("not found")
}

func (m *mockCategoryRepo) Delete(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, c := range m.categories {
		if c.ID == id {
			m.categories = append(m.categories[:i], m.categories[i+1:]...)
			return nil
		}
	}
	return errors.New("not found")
}

func (m *mockCategoryRepo) CountTorrentsByCategory(_ context.Context, categoryID int64) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.torrentCounts[categoryID], nil
}

func setupCategoryAdminRouter() (http.Handler, service.SessionStore, *mockCategoryRepo) {
	userRepo := newMockUserRepo()
	groupRepo := &mockGroupRepo{}
	sessions := testutil.NewMemorySessionStore()
	bus := event.NewInMemoryBus()
	authSvc := service.NewAuthServiceWithTTL(userRepo, sessions, testutil.NewMemoryPasswordResetStore(), &testutil.NoopSender{}, "http://localhost:8080", service.DefaultAccessTokenTTL, service.DefaultRefreshTokenTTL, groupRepo, bus)

	catRepo := newMockCategoryRepo()
	catSvc := service.NewCategoryService(catRepo)

	router := handler.NewRouter(&handler.Deps{
		AuthService:     authSvc,
		SessionStore:    sessions,
		CategoryService: catSvc,
	})
	return router, sessions, catRepo
}

func TestHandleListCategories_AsAdmin(t *testing.T) {
	router, sessions, catRepo := setupCategoryAdminRouter()

	// Seed a category
	catRepo.mu.Lock()
	catRepo.categories = append(catRepo.categories, &model.Category{ID: 1, Name: "Movies", Slug: "movies", SortOrder: 1})
	catRepo.nextID = 2
	catRepo.mu.Unlock()

	adminToken := createSessionWithGroup(sessions, 3000, 1)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/categories", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	cats := resp["categories"].([]interface{})
	if len(cats) != 1 {
		t.Errorf("expected 1 category, got %d", len(cats))
	}
}

func TestHandleListCategories_NonAdmin(t *testing.T) {
	router, sessions, _ := setupCategoryAdminRouter()

	userToken := createSessionWithGroup(sessions, 3001, 5)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/categories", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-admin, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCreateCategory_AsAdmin(t *testing.T) {
	router, sessions, _ := setupCategoryAdminRouter()

	adminToken := createSessionWithGroup(sessions, 3002, 1)
	body, _ := json.Marshal(map[string]interface{}{
		"name":       "Games",
		"sort_order": 4,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/categories", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	cat := resp["category"].(map[string]interface{})
	if cat["name"] != "Games" {
		t.Errorf("expected name Games, got %v", cat["name"])
	}
	if cat["slug"] != "games" {
		t.Errorf("expected slug games, got %v", cat["slug"])
	}
}

func TestHandleCreateCategory_EmptyName(t *testing.T) {
	router, sessions, _ := setupCategoryAdminRouter()

	adminToken := createSessionWithGroup(sessions, 3003, 1)
	body, _ := json.Marshal(map[string]interface{}{
		"name": "",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/categories", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCreateCategory_InvalidJSON(t *testing.T) {
	router, sessions, _ := setupCategoryAdminRouter()

	adminToken := createSessionWithGroup(sessions, 3004, 1)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/categories", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleUpdateCategory_AsAdmin(t *testing.T) {
	router, sessions, catRepo := setupCategoryAdminRouter()

	catRepo.mu.Lock()
	catRepo.categories = append(catRepo.categories, &model.Category{ID: 1, Name: "Movies", Slug: "movies", SortOrder: 1})
	catRepo.nextID = 2
	catRepo.mu.Unlock()

	adminToken := createSessionWithGroup(sessions, 3005, 1)
	body, _ := json.Marshal(map[string]interface{}{
		"name":       "Films",
		"slug":       "films",
		"sort_order": 2,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/categories/1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	cat := resp["category"].(map[string]interface{})
	if cat["name"] != "Films" {
		t.Errorf("expected name Films, got %v", cat["name"])
	}
}

func TestHandleUpdateCategory_NotFound(t *testing.T) {
	router, sessions, _ := setupCategoryAdminRouter()

	adminToken := createSessionWithGroup(sessions, 3006, 1)
	body, _ := json.Marshal(map[string]interface{}{
		"name": "Nope",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/categories/999", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteCategory_AsAdmin(t *testing.T) {
	router, sessions, catRepo := setupCategoryAdminRouter()

	catRepo.mu.Lock()
	catRepo.categories = append(catRepo.categories, &model.Category{ID: 1, Name: "Temp", Slug: "temp", SortOrder: 99})
	catRepo.nextID = 2
	catRepo.mu.Unlock()

	adminToken := createSessionWithGroup(sessions, 3007, 1)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/categories/1", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteCategory_HasTorrents(t *testing.T) {
	router, sessions, catRepo := setupCategoryAdminRouter()

	catRepo.mu.Lock()
	catRepo.categories = append(catRepo.categories, &model.Category{ID: 1, Name: "Movies", Slug: "movies", SortOrder: 1})
	catRepo.nextID = 2
	catRepo.torrentCounts[1] = 5
	catRepo.mu.Unlock()

	adminToken := createSessionWithGroup(sessions, 3008, 1)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/categories/1", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteCategory_NotFound(t *testing.T) {
	router, sessions, _ := setupCategoryAdminRouter()

	adminToken := createSessionWithGroup(sessions, 3009, 1)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/categories/999", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}
