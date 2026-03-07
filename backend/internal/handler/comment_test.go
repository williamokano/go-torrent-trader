package handler_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/handler"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/williamokano/go-torrent-trader/backend/internal/testutil"
)

// --- mock comment repo ---

type mockCommentRepo struct {
	mu       sync.Mutex
	comments []*model.Comment
	nextID   int64
}

func newMockCommentRepo() *mockCommentRepo {
	return &mockCommentRepo{nextID: 1}
}

func (m *mockCommentRepo) Create(_ context.Context, c *model.Comment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c.ID = m.nextID
	m.nextID++
	m.comments = append(m.comments, c)
	return nil
}

func (m *mockCommentRepo) GetByID(_ context.Context, id int64) (*model.Comment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.comments {
		if c.ID == id {
			copy := *c
			if copy.Username == "" {
				copy.Username = "testuser"
			}
			return &copy, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (m *mockCommentRepo) ListByTorrent(_ context.Context, torrentID int64, page, perPage int) ([]model.Comment, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []model.Comment
	for _, c := range m.comments {
		if c.TorrentID == torrentID {
			copy := *c
			if copy.Username == "" {
				copy.Username = "testuser"
			}
			result = append(result, copy)
		}
	}
	total := int64(len(result))
	start := (page - 1) * perPage
	if start >= len(result) {
		return nil, total, nil
	}
	end := start + perPage
	if end > len(result) {
		end = len(result)
	}
	return result[start:end], total, nil
}

func (m *mockCommentRepo) Update(_ context.Context, c *model.Comment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, existing := range m.comments {
		if existing.ID == c.ID {
			m.comments[i].Body = c.Body
			return nil
		}
	}
	return sql.ErrNoRows
}

func (m *mockCommentRepo) Delete(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, c := range m.comments {
		if c.ID == id {
			m.comments = append(m.comments[:i], m.comments[i+1:]...)
			return nil
		}
	}
	return sql.ErrNoRows
}

// --- mock rating repo ---

type mockRatingRepo struct {
	mu      sync.Mutex
	ratings []*model.Rating
	nextID  int64
}

func newMockRatingRepo() *mockRatingRepo {
	return &mockRatingRepo{nextID: 1}
}

func (m *mockRatingRepo) Upsert(_ context.Context, r *model.Rating) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.ratings {
		if existing.TorrentID == r.TorrentID && existing.UserID == r.UserID {
			existing.Rating = r.Rating
			r.ID = existing.ID
			return nil
		}
	}
	r.ID = m.nextID
	m.nextID++
	copy := *r
	m.ratings = append(m.ratings, &copy)
	return nil
}

func (m *mockRatingRepo) GetByTorrentAndUser(_ context.Context, torrentID, userID int64) (*model.Rating, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.ratings {
		if r.TorrentID == torrentID && r.UserID == userID {
			copy := *r
			return &copy, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (m *mockRatingRepo) GetStatsByTorrent(_ context.Context, torrentID int64) (float64, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var sum, count int
	for _, r := range m.ratings {
		if r.TorrentID == torrentID {
			sum += r.Rating
			count++
		}
	}
	if count == 0 {
		return 0, 0, nil
	}
	return float64(sum) / float64(count), count, nil
}

// --- mock torrent repo (for comment tests) ---

type mockTorrentRepoForCommentHandler struct {
	mu       sync.Mutex
	torrents []*model.Torrent
	nextID   int64
}

func newMockTorrentRepoForCommentHandler() *mockTorrentRepoForCommentHandler {
	return &mockTorrentRepoForCommentHandler{
		torrents: []*model.Torrent{{ID: 1, Name: "test-torrent"}},
		nextID:   2,
	}
}

func (m *mockTorrentRepoForCommentHandler) GetByID(_ context.Context, id int64) (*model.Torrent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.torrents {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *mockTorrentRepoForCommentHandler) GetByInfoHash(context.Context, []byte) (*model.Torrent, error) {
	return nil, errors.New("not found")
}
func (m *mockTorrentRepoForCommentHandler) List(context.Context, repository.ListTorrentsOptions) ([]model.Torrent, int64, error) {
	return nil, 0, nil
}
func (m *mockTorrentRepoForCommentHandler) Create(context.Context, *model.Torrent) error  { return nil }
func (m *mockTorrentRepoForCommentHandler) Update(context.Context, *model.Torrent) error  { return nil }
func (m *mockTorrentRepoForCommentHandler) Delete(context.Context, int64) error            { return nil }
func (m *mockTorrentRepoForCommentHandler) IncrementSeeders(context.Context, int64, int) error {
	return nil
}
func (m *mockTorrentRepoForCommentHandler) IncrementLeechers(context.Context, int64, int) error {
	return nil
}
func (m *mockTorrentRepoForCommentHandler) IncrementTimesCompleted(context.Context, int64) error {
	return nil
}

// --- helpers ---

func setupCommentRouter() (http.Handler, service.SessionStore) {
	userRepo := newMockUserRepo()
	torrentRepo := newMockTorrentRepoForCommentHandler()
	sessions := testutil.NewMemorySessionStore()
	authSvc := service.NewAuthServiceWithTTL(userRepo, sessions, testutil.NewMemoryPasswordResetStore(), &testutil.NoopSender{}, "http://localhost:8080", service.DefaultAccessTokenTTL, service.DefaultRefreshTokenTTL, &mockGroupRepo{})
	commentSvc := service.NewCommentService(newMockCommentRepo(), newMockRatingRepo(), torrentRepo)
	// TorrentService is needed because comment/rating routes are nested under /torrents
	torrentSvc := service.NewTorrentService(newMockTorrentRepo(), userRepo, newMockStorage(), service.TorrentServiceConfig{AnnounceURL: "http://localhost/announce"})

	router := handler.NewRouter(&handler.Deps{
		AuthService:    authSvc,
		SessionStore:   sessions,
		TorrentService: torrentSvc,
		CommentService: commentSvc,
	})
	return router, sessions
}

// --- comment handler tests ---

func TestHandleCreateComment_Success(t *testing.T) {
	router, sessions := setupCommentRouter()
	token := createSessionWithGroup(sessions, 1100, 5)

	body, _ := json.Marshal(map[string]string{"body": "Nice upload!"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents/1/comments", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	comment := resp["comment"].(map[string]interface{})
	if comment["body"] != "Nice upload!" {
		t.Errorf("expected body 'Nice upload!', got %v", comment["body"])
	}
}

func TestHandleCreateComment_Unauthenticated(t *testing.T) {
	router, _ := setupCommentRouter()

	body, _ := json.Marshal(map[string]string{"body": "Hello"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents/1/comments", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestHandleCreateComment_EmptyBody(t *testing.T) {
	router, sessions := setupCommentRouter()
	token := createSessionWithGroup(sessions, 1101, 5)

	body, _ := json.Marshal(map[string]string{"body": ""})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents/1/comments", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCreateComment_TorrentNotFound(t *testing.T) {
	router, sessions := setupCommentRouter()
	token := createSessionWithGroup(sessions, 1102, 5)

	body, _ := json.Marshal(map[string]string{"body": "Hello"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents/999/comments", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleListComments_Success(t *testing.T) {
	router, sessions := setupCommentRouter()
	token := createSessionWithGroup(sessions, 1103, 5)

	// Create a comment first
	body, _ := json.Marshal(map[string]string{"body": "Test comment"})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/torrents/1/comments", bytes.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+token)
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)

	// List
	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents/1/comments", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	comments := resp["comments"].([]interface{})
	if len(comments) != 1 {
		t.Errorf("expected 1 comment, got %d", len(comments))
	}
	total := int(resp["total"].(float64))
	if total != 1 {
		t.Errorf("expected total 1, got %d", total)
	}
}

func TestHandleEditComment_AsAuthor(t *testing.T) {
	router, sessions := setupCommentRouter()
	token := createSessionWithGroup(sessions, 1104, 5)

	// Create
	body, _ := json.Marshal(map[string]string{"body": "Original"})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/torrents/1/comments", bytes.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+token)
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)

	var createResp map[string]interface{}
	_ = json.Unmarshal(createRec.Body.Bytes(), &createResp)
	comment := createResp["comment"].(map[string]interface{})
	commentID := int64(comment["id"].(float64))

	// Edit
	editBody, _ := json.Marshal(map[string]string{"body": "Edited"})
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/comments/%d", commentID), bytes.NewReader(editBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	edited := resp["comment"].(map[string]interface{})
	if edited["body"] != "Edited" {
		t.Errorf("expected body 'Edited', got %v", edited["body"])
	}
}

func TestHandleEditComment_Forbidden(t *testing.T) {
	router, sessions := setupCommentRouter()
	authorToken := createSessionWithGroup(sessions, 1105, 5)
	otherToken := createSessionWithGroup(sessions, 1106, 5)

	// Create as author
	body, _ := json.Marshal(map[string]string{"body": "Original"})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/torrents/1/comments", bytes.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+authorToken)
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)

	var createResp map[string]interface{}
	_ = json.Unmarshal(createRec.Body.Bytes(), &createResp)
	comment := createResp["comment"].(map[string]interface{})
	commentID := int64(comment["id"].(float64))

	// Edit as other user
	editBody, _ := json.Marshal(map[string]string{"body": "Hacked"})
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/comments/%d", commentID), bytes.NewReader(editBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+otherToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleEditComment_AsAdmin(t *testing.T) {
	router, sessions := setupCommentRouter()
	authorToken := createSessionWithGroup(sessions, 1107, 5)
	adminToken := createSessionWithGroup(sessions, 1108, 1)

	// Create as author
	body, _ := json.Marshal(map[string]string{"body": "Original"})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/torrents/1/comments", bytes.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+authorToken)
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)

	var createResp map[string]interface{}
	_ = json.Unmarshal(createRec.Body.Bytes(), &createResp)
	comment := createResp["comment"].(map[string]interface{})
	commentID := int64(comment["id"].(float64))

	// Edit as admin
	editBody, _ := json.Marshal(map[string]string{"body": "Admin edit"})
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/comments/%d", commentID), bytes.NewReader(editBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteComment_AsAdmin(t *testing.T) {
	router, sessions := setupCommentRouter()
	authorToken := createSessionWithGroup(sessions, 1109, 5)
	adminToken := createSessionWithGroup(sessions, 1110, 1)

	// Create as author
	body, _ := json.Marshal(map[string]string{"body": "To delete"})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/torrents/1/comments", bytes.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+authorToken)
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)

	var createResp map[string]interface{}
	_ = json.Unmarshal(createRec.Body.Bytes(), &createResp)
	comment := createResp["comment"].(map[string]interface{})
	commentID := int64(comment["id"].(float64))

	// Delete as admin
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/comments/%d", commentID), nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteComment_NonAdmin(t *testing.T) {
	router, sessions := setupCommentRouter()
	authorToken := createSessionWithGroup(sessions, 1111, 5)

	// Create
	body, _ := json.Marshal(map[string]string{"body": "To delete"})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/torrents/1/comments", bytes.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+authorToken)
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)

	var createResp map[string]interface{}
	_ = json.Unmarshal(createRec.Body.Bytes(), &createResp)
	comment := createResp["comment"].(map[string]interface{})
	commentID := int64(comment["id"].(float64))

	// Try delete as non-admin (even the author)
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/comments/%d", commentID), nil)
	req.Header.Set("Authorization", "Bearer "+authorToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// --- rating handler tests ---

func TestHandleRateTorrent_Success(t *testing.T) {
	router, sessions := setupCommentRouter()
	token := createSessionWithGroup(sessions, 1120, 5)

	body, _ := json.Marshal(map[string]int{"rating": 4})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents/1/rating", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	avg := resp["average"].(float64)
	if avg != 4.0 {
		t.Errorf("expected average 4.0, got %f", avg)
	}
	count := int(resp["count"].(float64))
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}
}

func TestHandleRateTorrent_InvalidRating(t *testing.T) {
	router, sessions := setupCommentRouter()
	token := createSessionWithGroup(sessions, 1121, 5)

	body, _ := json.Marshal(map[string]int{"rating": 6})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents/1/rating", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleRateTorrent_TorrentNotFound(t *testing.T) {
	router, sessions := setupCommentRouter()
	token := createSessionWithGroup(sessions, 1122, 5)

	body, _ := json.Marshal(map[string]int{"rating": 3})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents/999/rating", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleGetRating_Success(t *testing.T) {
	router, sessions := setupCommentRouter()
	token := createSessionWithGroup(sessions, 1123, 5)

	// Rate first
	body, _ := json.Marshal(map[string]int{"rating": 5})
	rateReq := httptest.NewRequest(http.MethodPost, "/api/v1/torrents/1/rating", bytes.NewReader(body))
	rateReq.Header.Set("Content-Type", "application/json")
	rateReq.Header.Set("Authorization", "Bearer "+token)
	rateRec := httptest.NewRecorder()
	router.ServeHTTP(rateRec, rateReq)

	// Get rating
	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents/1/rating", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["average"].(float64) != 5.0 {
		t.Errorf("expected average 5.0, got %v", resp["average"])
	}
	if resp["user_rating"].(float64) != 5.0 {
		t.Errorf("expected user_rating 5, got %v", resp["user_rating"])
	}
}

func TestHandleRateTorrent_Unauthenticated(t *testing.T) {
	router, _ := setupCommentRouter()

	body, _ := json.Marshal(map[string]int{"rating": 3})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents/1/rating", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}
