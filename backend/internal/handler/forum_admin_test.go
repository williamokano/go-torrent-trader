package handler_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/handler"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/williamokano/go-torrent-trader/backend/internal/testutil"
)

// mockForumCategoryRepo implements repository.ForumCategoryRepository for handler tests.
type mockForumCategoryRepo struct {
	mu          sync.Mutex
	categories  []model.ForumCategory
	nextID      int64
	forumCounts map[int64]int64
}

func newMockForumCategoryRepo() *mockForumCategoryRepo {
	return &mockForumCategoryRepo{nextID: 1, forumCounts: make(map[int64]int64)}
}

func (m *mockForumCategoryRepo) GetByID(_ context.Context, id int64) (*model.ForumCategory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.categories {
		if m.categories[i].ID == id {
			return &m.categories[i], nil
		}
	}
	return nil, sql.ErrNoRows
}

func (m *mockForumCategoryRepo) List(_ context.Context) ([]model.ForumCategory, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]model.ForumCategory, len(m.categories))
	copy(result, m.categories)
	return result, nil
}

func (m *mockForumCategoryRepo) Create(_ context.Context, cat *model.ForumCategory) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cat.ID = m.nextID
	m.nextID++
	cat.CreatedAt = time.Now()
	m.categories = append(m.categories, *cat)
	return nil
}

func (m *mockForumCategoryRepo) Update(_ context.Context, cat *model.ForumCategory) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.categories {
		if m.categories[i].ID == cat.ID {
			m.categories[i] = *cat
			return nil
		}
	}
	return sql.ErrNoRows
}

func (m *mockForumCategoryRepo) Delete(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.categories {
		if m.categories[i].ID == id {
			m.categories = append(m.categories[:i], m.categories[i+1:]...)
			return nil
		}
	}
	return sql.ErrNoRows
}

func (m *mockForumCategoryRepo) CountForumsByCategory(_ context.Context, categoryID int64) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.forumCounts[categoryID], nil
}

// mockForumAdminForumRepo implements repository.ForumRepository for admin handler tests.
type mockForumAdminForumRepo struct {
	mu          sync.Mutex
	forums      []model.Forum
	nextID      int64
	topicCounts map[int64]int64
}

func newMockForumAdminForumRepo() *mockForumAdminForumRepo {
	return &mockForumAdminForumRepo{nextID: 1, topicCounts: make(map[int64]int64)}
}

func (m *mockForumAdminForumRepo) GetByID(_ context.Context, id int64) (*model.Forum, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.forums {
		if m.forums[i].ID == id {
			return &m.forums[i], nil
		}
	}
	return nil, sql.ErrNoRows
}

func (m *mockForumAdminForumRepo) ListByCategory(_ context.Context, categoryID int64) ([]model.Forum, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []model.Forum
	for _, f := range m.forums {
		if f.CategoryID == categoryID {
			result = append(result, f)
		}
	}
	return result, nil
}

func (m *mockForumAdminForumRepo) List(_ context.Context) ([]model.Forum, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]model.Forum, len(m.forums))
	copy(result, m.forums)
	return result, nil
}

func (m *mockForumAdminForumRepo) Create(_ context.Context, forum *model.Forum) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	forum.ID = m.nextID
	m.nextID++
	forum.CreatedAt = time.Now()
	m.forums = append(m.forums, *forum)
	return nil
}

func (m *mockForumAdminForumRepo) Update(_ context.Context, forum *model.Forum) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.forums {
		if m.forums[i].ID == forum.ID {
			m.forums[i] = *forum
			return nil
		}
	}
	return sql.ErrNoRows
}

func (m *mockForumAdminForumRepo) Delete(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.forums {
		if m.forums[i].ID == id {
			m.forums = append(m.forums[:i], m.forums[i+1:]...)
			return nil
		}
	}
	return sql.ErrNoRows
}

func (m *mockForumAdminForumRepo) CountTopicsByForum(_ context.Context, forumID int64) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.topicCounts[forumID], nil
}

func (m *mockForumAdminForumRepo) IncrementTopicCount(_ context.Context, _ int64, _ int) error { return nil }
func (m *mockForumAdminForumRepo) IncrementPostCount(_ context.Context, _ int64, _ int) error  { return nil }
func (m *mockForumAdminForumRepo) UpdateLastPost(_ context.Context, _ int64, _ int64) error     { return nil }
func (m *mockForumAdminForumRepo) RecalculateLastPost(_ context.Context, _ int64) error         { return nil }
func (m *mockForumAdminForumRepo) RecalculateCounts(_ context.Context, _ int64) error           { return nil }

// mockForumAdminTopicRepo satisfies ForumTopicRepository with no-op methods.
type mockForumAdminTopicRepo struct{}

func (m *mockForumAdminTopicRepo) GetByID(_ context.Context, _ int64) (*model.ForumTopic, error)                   { return nil, sql.ErrNoRows }
func (m *mockForumAdminTopicRepo) ListByForum(_ context.Context, _ int64, _, _ int) ([]model.ForumTopic, int64, error) { return nil, 0, nil }
func (m *mockForumAdminTopicRepo) Create(_ context.Context, _ *model.ForumTopic) error                             { return nil }
func (m *mockForumAdminTopicRepo) IncrementViewCount(_ context.Context, _ int64) error                             { return nil }
func (m *mockForumAdminTopicRepo) IncrementPostCount(_ context.Context, _ int64, _ int) error                      { return nil }
func (m *mockForumAdminTopicRepo) UpdateLastPost(_ context.Context, _ int64, _ int64, _ time.Time) error           { return nil }
func (m *mockForumAdminTopicRepo) RecalculateLastPost(_ context.Context, _ int64) error                            { return nil }
func (m *mockForumAdminTopicRepo) SetLocked(_ context.Context, _ int64, _ bool) error                              { return nil }
func (m *mockForumAdminTopicRepo) SetPinned(_ context.Context, _ int64, _ bool) error                              { return nil }
func (m *mockForumAdminTopicRepo) UpdateTitle(_ context.Context, _ int64, _ string) error                          { return nil }
func (m *mockForumAdminTopicRepo) UpdateForumID(_ context.Context, _ int64, _ int64) error                         { return nil }
func (m *mockForumAdminTopicRepo) Delete(_ context.Context, _ int64) error                                         { return nil }

// mockForumAdminPostRepo satisfies ForumPostRepository with no-op methods.
type mockForumAdminPostRepo struct{}

func (m *mockForumAdminPostRepo) GetByID(_ context.Context, _ int64) (*model.ForumPost, error) { return nil, sql.ErrNoRows }
func (m *mockForumAdminPostRepo) ListByTopic(_ context.Context, _ int64, _, _ int) ([]model.ForumPost, int64, error) { return nil, 0, nil }
func (m *mockForumAdminPostRepo) Create(_ context.Context, _ *model.ForumPost) error { return nil }
func (m *mockForumAdminPostRepo) Update(_ context.Context, _ *model.ForumPost) error { return nil }
func (m *mockForumAdminPostRepo) Delete(_ context.Context, _ int64) error { return nil }
func (m *mockForumAdminPostRepo) CountByUser(_ context.Context, _ int64) (int, error) { return 0, nil }
func (m *mockForumAdminPostRepo) Search(_ context.Context, _ string, _ *int64, _ int, _, _ int) ([]model.ForumSearchResult, int64, error) { return nil, 0, nil }
func (m *mockForumAdminPostRepo) GetFirstPostIDByTopic(_ context.Context, _ int64) (int64, error) { return 0, nil }
func (m *mockForumAdminPostRepo) SoftDelete(_ context.Context, _ int64, _ int64) error { return nil }
func (m *mockForumAdminPostRepo) Restore(_ context.Context, _ int64) error { return nil }
func (m *mockForumAdminPostRepo) CreateEdit(_ context.Context, _ *model.ForumPostEdit) error { return nil }
func (m *mockForumAdminPostRepo) ListEdits(_ context.Context, _ int64) ([]model.ForumPostEdit, error) { return nil, nil }

func setupForumAdminRouter() (http.Handler, service.SessionStore, *mockForumCategoryRepo, *mockForumAdminForumRepo) {
	userRepo := newMockUserRepo()
	groupRepo := &mockGroupRepo{}
	sessions := testutil.NewMemorySessionStore()
	bus := event.NewInMemoryBus()
	authSvc := service.NewAuthServiceWithTTL(userRepo, sessions, testutil.NewMemoryPasswordResetStore(), &testutil.NoopSender{}, "http://localhost:8080", service.DefaultAccessTokenTTL, service.DefaultRefreshTokenTTL, groupRepo, bus)

	catRepo := newMockForumCategoryRepo()
	forumRepo := newMockForumAdminForumRepo()
	forumSvc := service.NewForumService(nil, catRepo, forumRepo, &mockForumAdminTopicRepo{}, &mockForumAdminPostRepo{}, userRepo, nil, bus)

	router := handler.NewRouter(&handler.Deps{
		AuthService:  authSvc,
		SessionStore: sessions,
		ForumService: forumSvc,
	})
	return router, sessions, catRepo, forumRepo
}

func TestHandleListForumCategories_AsAdmin(t *testing.T) {
	router, sessions, catRepo, _ := setupForumAdminRouter()

	catRepo.mu.Lock()
	catRepo.categories = append(catRepo.categories, model.ForumCategory{ID: 1, Name: "General", SortOrder: 1, CreatedAt: time.Now()})
	catRepo.nextID = 2
	catRepo.mu.Unlock()

	adminToken := createSessionWithGroup(sessions, 4000, 1)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/forum-categories", nil)
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

func TestHandleListForumCategories_NonAdmin(t *testing.T) {
	router, sessions, _, _ := setupForumAdminRouter()

	userToken := createSessionWithGroup(sessions, 4001, 5)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/forum-categories", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestHandleCreateForumCategory_AsAdmin(t *testing.T) {
	router, sessions, _, _ := setupForumAdminRouter()

	adminToken := createSessionWithGroup(sessions, 4002, 1)
	body, _ := json.Marshal(map[string]interface{}{
		"name":       "Community",
		"sort_order": 2,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/forum-categories", bytes.NewReader(body))
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
	if cat["name"] != "Community" {
		t.Errorf("expected name Community, got %v", cat["name"])
	}
}

func TestHandleCreateForumCategory_EmptyName(t *testing.T) {
	router, sessions, _, _ := setupForumAdminRouter()

	adminToken := createSessionWithGroup(sessions, 4003, 1)
	body, _ := json.Marshal(map[string]interface{}{"name": ""})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/forum-categories", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleUpdateForumCategory_AsAdmin(t *testing.T) {
	router, sessions, catRepo, _ := setupForumAdminRouter()

	catRepo.mu.Lock()
	catRepo.categories = append(catRepo.categories, model.ForumCategory{ID: 1, Name: "Old", SortOrder: 0, CreatedAt: time.Now()})
	catRepo.nextID = 2
	catRepo.mu.Unlock()

	adminToken := createSessionWithGroup(sessions, 4004, 1)
	body, _ := json.Marshal(map[string]interface{}{
		"name":       "Renamed",
		"sort_order": 10,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/forum-categories/1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleUpdateForumCategory_NotFound(t *testing.T) {
	router, sessions, _, _ := setupForumAdminRouter()

	adminToken := createSessionWithGroup(sessions, 4005, 1)
	body, _ := json.Marshal(map[string]interface{}{"name": "Nope"})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/forum-categories/999", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteForumCategory_AsAdmin(t *testing.T) {
	router, sessions, catRepo, _ := setupForumAdminRouter()

	catRepo.mu.Lock()
	catRepo.categories = append(catRepo.categories, model.ForumCategory{ID: 1, Name: "Temp", SortOrder: 0, CreatedAt: time.Now()})
	catRepo.nextID = 2
	catRepo.mu.Unlock()

	adminToken := createSessionWithGroup(sessions, 4006, 1)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/forum-categories/1", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteForumCategory_HasForums(t *testing.T) {
	router, sessions, catRepo, _ := setupForumAdminRouter()

	catRepo.mu.Lock()
	catRepo.categories = append(catRepo.categories, model.ForumCategory{ID: 1, Name: "WithForums", SortOrder: 0, CreatedAt: time.Now()})
	catRepo.nextID = 2
	catRepo.forumCounts[1] = 3
	catRepo.mu.Unlock()

	adminToken := createSessionWithGroup(sessions, 4007, 1)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/forum-categories/1", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleListForums_AsAdmin(t *testing.T) {
	router, sessions, _, forumRepo := setupForumAdminRouter()

	forumRepo.mu.Lock()
	forumRepo.forums = append(forumRepo.forums, model.Forum{ID: 1, Name: "General", CategoryID: 1, CreatedAt: time.Now()})
	forumRepo.nextID = 2
	forumRepo.mu.Unlock()

	adminToken := createSessionWithGroup(sessions, 4008, 1)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/forums", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	forums := resp["forums"].([]interface{})
	if len(forums) != 1 {
		t.Errorf("expected 1 forum, got %d", len(forums))
	}
}

func TestHandleCreateForum_AsAdmin(t *testing.T) {
	router, sessions, catRepo, _ := setupForumAdminRouter()

	catRepo.mu.Lock()
	catRepo.categories = append(catRepo.categories, model.ForumCategory{ID: 1, Name: "General", SortOrder: 0, CreatedAt: time.Now()})
	catRepo.nextID = 2
	catRepo.mu.Unlock()

	adminToken := createSessionWithGroup(sessions, 4009, 1)
	body, _ := json.Marshal(map[string]interface{}{
		"name":            "Announcements",
		"description":     "Site news",
		"category_id":     1,
		"sort_order":      1,
		"min_group_level": 0,
		"min_post_level":  5,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/forums", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	forum := resp["forum"].(map[string]interface{})
	if forum["name"] != "Announcements" {
		t.Errorf("expected name Announcements, got %v", forum["name"])
	}
}

func TestHandleUpdateForum_AsAdmin(t *testing.T) {
	router, sessions, catRepo, forumRepo := setupForumAdminRouter()

	catRepo.mu.Lock()
	catRepo.categories = append(catRepo.categories, model.ForumCategory{ID: 1, Name: "General", SortOrder: 0, CreatedAt: time.Now()})
	catRepo.nextID = 2
	catRepo.mu.Unlock()

	forumRepo.mu.Lock()
	forumRepo.forums = append(forumRepo.forums, model.Forum{ID: 1, Name: "Old", CategoryID: 1, CreatedAt: time.Now()})
	forumRepo.nextID = 2
	forumRepo.mu.Unlock()

	adminToken := createSessionWithGroup(sessions, 4010, 1)
	body, _ := json.Marshal(map[string]interface{}{
		"name":        "Renamed",
		"description": "New desc",
		"category_id": 1,
		"sort_order":  2,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/forums/1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteForum_AsAdmin(t *testing.T) {
	router, sessions, _, forumRepo := setupForumAdminRouter()

	forumRepo.mu.Lock()
	forumRepo.forums = append(forumRepo.forums, model.Forum{ID: 1, Name: "Temp", CategoryID: 1, CreatedAt: time.Now()})
	forumRepo.nextID = 2
	forumRepo.mu.Unlock()

	adminToken := createSessionWithGroup(sessions, 4011, 1)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/forums/1", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteForum_HasTopics(t *testing.T) {
	router, sessions, _, forumRepo := setupForumAdminRouter()

	forumRepo.mu.Lock()
	forumRepo.forums = append(forumRepo.forums, model.Forum{ID: 1, Name: "WithTopics", CategoryID: 1, CreatedAt: time.Now()})
	forumRepo.nextID = 2
	forumRepo.topicCounts[1] = 5
	forumRepo.mu.Unlock()

	adminToken := createSessionWithGroup(sessions, 4012, 1)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/forums/1", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d; body: %s", rec.Code, rec.Body.String())
	}
}
