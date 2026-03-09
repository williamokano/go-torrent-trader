package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/zeebo/bencode"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/handler"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/williamokano/go-torrent-trader/backend/internal/testutil"
)

// --- mock torrent repo ---

type mockTorrentRepo struct {
	mu       sync.Mutex
	torrents []*model.Torrent
	nextID   int64
}

func newMockTorrentRepo() *mockTorrentRepo {
	return &mockTorrentRepo{nextID: 1}
}

func (m *mockTorrentRepo) GetByID(_ context.Context, id int64) (*model.Torrent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.torrents {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *mockTorrentRepo) GetByInfoHash(_ context.Context, infoHash []byte) (*model.Torrent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.torrents {
		if bytes.Equal(t.InfoHash, infoHash) {
			return t, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *mockTorrentRepo) List(_ context.Context, opts repository.ListTorrentsOptions) ([]model.Torrent, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []model.Torrent
	for _, t := range m.torrents {
		result = append(result, *t)
	}
	total := int64(len(result))
	page := opts.Page
	if page <= 0 {
		page = 1
	}
	perPage := opts.PerPage
	if perPage <= 0 {
		perPage = 25
	}
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

func (m *mockTorrentRepo) Create(_ context.Context, torrent *model.Torrent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	torrent.ID = m.nextID
	m.nextID++
	m.torrents = append(m.torrents, torrent)
	return nil
}

func (m *mockTorrentRepo) Update(_ context.Context, torrent *model.Torrent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, t := range m.torrents {
		if t.ID == torrent.ID {
			m.torrents[i] = torrent
			return nil
		}
	}
	return errors.New("not found")
}

func (m *mockTorrentRepo) Delete(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, t := range m.torrents {
		if t.ID == id {
			m.torrents = append(m.torrents[:i], m.torrents[i+1:]...)
			return nil
		}
	}
	return errors.New("not found")
}

func (m *mockTorrentRepo) IncrementSeeders(_ context.Context, _ int64, _ int) error  { return nil }
func (m *mockTorrentRepo) IncrementLeechers(_ context.Context, _ int64, _ int) error { return nil }
func (m *mockTorrentRepo) IncrementTimesCompleted(_ context.Context, _ int64) error  { return nil }
func (m *mockTorrentRepo) ListByUploader(_ context.Context, _ int64, _ int) ([]model.Torrent, error) {
	return nil, nil
}

// --- mock storage ---

type mockStorage struct {
	mu    sync.Mutex
	files map[string][]byte
}

func newMockStorage() *mockStorage {
	return &mockStorage{files: make(map[string][]byte)}
}

func (m *mockStorage) Put(_ context.Context, key string, reader io.Reader) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.files[key] = data
	m.mu.Unlock()
	return nil
}

func (m *mockStorage) Get(_ context.Context, key string) (io.ReadCloser, error) {
	m.mu.Lock()
	data, ok := m.files[key]
	m.mu.Unlock()
	if !ok {
		return nil, errors.New("not found")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *mockStorage) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	delete(m.files, key)
	m.mu.Unlock()
	return nil
}

func (m *mockStorage) Exists(_ context.Context, key string) (bool, error) {
	m.mu.Lock()
	_, ok := m.files[key]
	m.mu.Unlock()
	return ok, nil
}

func (m *mockStorage) URL(_ context.Context, key string) (string, error) {
	return "/files/" + key, nil
}

// --- mock reseed request repo ---

type mockReseedRequestRepo struct {
	mu       sync.Mutex
	requests []*model.ReseedRequest
	nextID   int64
}

func newMockReseedRequestRepo() *mockReseedRequestRepo {
	return &mockReseedRequestRepo{nextID: 1}
}

func (m *mockReseedRequestRepo) Create(_ context.Context, req *model.ReseedRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	req.ID = m.nextID
	m.nextID++
	m.requests = append(m.requests, req)
	return nil
}

func (m *mockReseedRequestRepo) ExistsByTorrentAndUser(_ context.Context, torrentID, userID int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.requests {
		if r.TorrentID == torrentID && r.RequesterID == userID {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockReseedRequestRepo) CountByTorrent(_ context.Context, torrentID int64) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, r := range m.requests {
		if r.TorrentID == torrentID {
			count++
		}
	}
	return count, nil
}

// --- helpers ---

func buildTorrentFileBytes(name string) []byte {
	info := map[string]interface{}{
		"name":         name,
		"piece length": 262144,
		"pieces":       "xxxxxxxxxxxxxxxxxxxx",
		"length":       1024,
	}
	infoBytes, _ := bencode.EncodeBytes(info)
	meta := map[string]interface{}{
		"announce": "http://example.com/announce",
		"info":     bencode.RawMessage(infoBytes),
	}
	data, _ := bencode.EncodeBytes(meta)
	return data
}

func setupTorrentRouter() (http.Handler, service.SessionStore) {
	userRepo := newMockUserRepo()
	torrentRepo := newMockTorrentRepo()
	store := newMockStorage()
	sessions := testutil.NewMemorySessionStore()
	bus := event.NewInMemoryBus()
	reseedRepo := newMockReseedRequestRepo()
	authSvc := service.NewAuthServiceWithTTL(userRepo, sessions, testutil.NewMemoryPasswordResetStore(), &testutil.NoopSender{}, "http://localhost:8080", service.DefaultAccessTokenTTL, service.DefaultRefreshTokenTTL, &mockGroupRepo{}, bus)
	torrentSvc := service.NewTorrentService(nil, torrentRepo, userRepo, store, service.TorrentServiceConfig{AnnounceURL: "http://localhost/announce"}, bus, reseedRepo)

	router := handler.NewRouter(&handler.Deps{
		AuthService:    authSvc,
		SessionStore:   sessions,
		TorrentService: torrentSvc,
	})
	return router, sessions
}

var testUserCounter int64
var testUserCounterMu sync.Mutex

func nextTestUserID() int64 {
	testUserCounterMu.Lock()
	defer testUserCounterMu.Unlock()
	testUserCounter++
	return testUserCounter
}

// registerAndGetToken registers a user and returns the access token.
func registerAndGetToken(t *testing.T, router http.Handler) string {
	t.Helper()
	n := nextTestUserID()
	body, _ := json.Marshal(map[string]string{
		"username": fmt.Sprintf("tuser%d", n),
		"email":    fmt.Sprintf("tuser%d@test.com", n),
		"password": "password123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	tokens := resp["tokens"].(map[string]interface{})
	return tokens["access_token"].(string)
}

func makeUploadRequest(token string, torrentData []byte, categoryID string) *http.Request {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("torrent_file", "test.torrent")
	_, _ = fw.Write(torrentData)
	_ = w.WriteField("category_id", categoryID)
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

// --- tests ---

func TestHandleUpload_Success(t *testing.T) {
	router, _ := setupTorrentRouter()
	token := registerAndGetToken(t, router)
	torrentData := buildTorrentFileBytes("upload-handler-test")

	req := makeUploadRequest(token, torrentData, "1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	torrent := resp["torrent"].(map[string]interface{})
	if torrent["name"] != "upload-handler-test" {
		t.Errorf("expected name upload-handler-test, got %v", torrent["name"])
	}
}

func TestHandleUpload_Unauthenticated(t *testing.T) {
	router, _ := setupTorrentRouter()
	torrentData := buildTorrentFileBytes("no-auth-test")

	req := makeUploadRequest("invalid-token", torrentData, "1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestHandleUpload_MissingCategoryID(t *testing.T) {
	router, _ := setupTorrentRouter()
	token := registerAndGetToken(t, router)
	torrentData := buildTorrentFileBytes("no-cat-test")

	req := makeUploadRequest(token, torrentData, "0")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleUpload_InvalidTorrent(t *testing.T) {
	router, _ := setupTorrentRouter()
	token := registerAndGetToken(t, router)

	req := makeUploadRequest(token, []byte("not a torrent file"), "1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleUpload_Duplicate(t *testing.T) {
	router, _ := setupTorrentRouter()
	token := registerAndGetToken(t, router)
	torrentData := buildTorrentFileBytes("dup-handler-test")

	// First upload
	req1 := makeUploadRequest(token, torrentData, "1")
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("first upload failed: %d %s", rec1.Code, rec1.Body.String())
	}

	// Second upload (same file)
	req2 := makeUploadRequest(token, torrentData, "1")
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d; body: %s", rec2.Code, rec2.Body.String())
	}
}

func TestHandleList_Success(t *testing.T) {
	router, _ := setupTorrentRouter()
	token := registerAndGetToken(t, router)

	// Upload a torrent first
	torrentData := buildTorrentFileBytes("list-handler-test")
	uploadReq := makeUploadRequest(token, torrentData, "1")
	uploadRec := httptest.NewRecorder()
	router.ServeHTTP(uploadRec, uploadReq)

	// List
	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	torrents := resp["torrents"].([]interface{})
	if len(torrents) != 1 {
		t.Errorf("expected 1 torrent, got %d", len(torrents))
	}
}

func TestHandleList_IncludesUploaderName(t *testing.T) {
	router, _ := setupTorrentRouter()
	token := registerAndGetToken(t, router)

	// Upload a non-anonymous torrent
	torrentData := buildTorrentFileBytes("uploader-name-test")
	uploadReq := makeUploadRequest(token, torrentData, "1")
	uploadRec := httptest.NewRecorder()
	router.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusCreated {
		t.Fatalf("upload failed: %d %s", uploadRec.Code, uploadRec.Body.String())
	}

	// List
	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	torrents := resp["torrents"].([]interface{})
	if len(torrents) == 0 {
		t.Fatal("expected at least 1 torrent")
	}

	first := torrents[0].(map[string]interface{})
	uploaderName, ok := first["uploader_name"]
	if !ok {
		t.Fatal("expected uploader_name field in list response")
	}
	if uploaderName == "" {
		t.Error("expected non-empty uploader_name")
	}
	// Non-anonymous upload should not show "Anonymous"
	if first["anonymous"] == false && uploaderName == "Anonymous" {
		t.Error("non-anonymous torrent should not have uploader_name 'Anonymous'")
	}
}

func TestHandleList_AnonymousUploaderShowsAnonymous(t *testing.T) {
	router, sessions := setupTorrentRouter()

	ownerToken := createSessionWithGroup(sessions, 1500, 5)

	// Upload an anonymous torrent
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("torrent_file", "test.torrent")
	_, _ = fw.Write(buildTorrentFileBytes("anon-uploader-test"))
	_ = w.WriteField("category_id", "1")
	_ = w.WriteField("anonymous", "true")
	_ = w.Close()

	uploadReq := httptest.NewRequest(http.MethodPost, "/api/v1/torrents", &buf)
	uploadReq.Header.Set("Content-Type", w.FormDataContentType())
	uploadReq.Header.Set("Authorization", "Bearer "+ownerToken)
	uploadRec := httptest.NewRecorder()
	router.ServeHTTP(uploadRec, uploadReq)

	if uploadRec.Code != http.StatusCreated {
		t.Fatalf("upload failed: %d %s", uploadRec.Code, uploadRec.Body.String())
	}

	// List
	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents", nil)
	req.Header.Set("Authorization", "Bearer "+ownerToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	torrents := resp["torrents"].([]interface{})

	// Find the anonymous torrent
	found := false
	for _, item := range torrents {
		torrent := item.(map[string]interface{})
		if torrent["anonymous"] == true {
			found = true
			if torrent["uploader_name"] != "Anonymous" {
				t.Errorf("expected uploader_name 'Anonymous' for anonymous torrent, got %v", torrent["uploader_name"])
			}
			break
		}
	}
	if !found {
		t.Error("expected to find an anonymous torrent in the list")
	}
}

func TestHandleList_Unauthenticated(t *testing.T) {
	router, _ := setupTorrentRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestHandleGetByID_Success(t *testing.T) {
	router, _ := setupTorrentRouter()
	token := registerAndGetToken(t, router)

	// Upload
	torrentData := buildTorrentFileBytes("get-handler-test")
	uploadReq := makeUploadRequest(token, torrentData, "1")
	uploadRec := httptest.NewRecorder()
	router.ServeHTTP(uploadRec, uploadReq)

	var uploadResp map[string]interface{}
	_ = json.Unmarshal(uploadRec.Body.Bytes(), &uploadResp)
	torrent := uploadResp["torrent"].(map[string]interface{})
	id := int64(torrent["id"].(float64))

	// Get by ID
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/torrents/%d", id), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleGetByID_NotFound(t *testing.T) {
	router, _ := setupTorrentRouter()
	token := registerAndGetToken(t, router)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents/999", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandleGetByID_InvalidID(t *testing.T) {
	router, _ := setupTorrentRouter()
	token := registerAndGetToken(t, router)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents/abc", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleDownload_NotFound(t *testing.T) {
	router, _ := setupTorrentRouter()
	token := registerAndGetToken(t, router)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents/999/download", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// createSessionWithGroup creates a session directly in the session store with the given groupID.
func createSessionWithGroup(sessions service.SessionStore, userID, groupID int64) string {
	token := fmt.Sprintf("test-token-%d-%d-%d", userID, groupID, nextTestUserID())

	perms := testutil.UserPermissions()
	switch groupID {
	case 1:
		perms = testutil.AdminPermissions()
	case 2:
		perms = testutil.ModeratorPermissions()
	case 6:
		perms = testutil.ValidatingPermissions()
	}

	_ = sessions.Create(&service.Session{
		UserID:           userID,
		GroupID:          groupID,
		Permissions:      perms,
		AccessToken:      token,
		RefreshToken:     "refresh-" + token,
		ExpiresAt:        time.Now().Add(time.Hour),
		RefreshExpiresAt: time.Now().Add(24 * time.Hour),
	})
	return token
}

// --- Edit handler tests ---

func TestHandleEdit_AsOwner(t *testing.T) {
	router, sessions := setupTorrentRouter()

	// Create a regular user (groupID=5) and upload a torrent
	ownerToken := createSessionWithGroup(sessions, 100, 5)
	torrentData := buildTorrentFileBytes("edit-handler-owner")
	uploadReq := makeUploadRequest(ownerToken, torrentData, "1")
	uploadRec := httptest.NewRecorder()
	router.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusCreated {
		t.Fatalf("upload failed: %d %s", uploadRec.Code, uploadRec.Body.String())
	}

	var uploadResp map[string]interface{}
	_ = json.Unmarshal(uploadRec.Body.Bytes(), &uploadResp)
	torrent := uploadResp["torrent"].(map[string]interface{})
	id := int64(torrent["id"].(float64))

	// Edit as owner
	editBody, _ := json.Marshal(map[string]interface{}{
		"name":        "owner-edited",
		"description": "new desc",
	})
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/torrents/%d", id), bytes.NewReader(editBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ownerToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	edited := resp["torrent"].(map[string]interface{})
	if edited["name"] != "owner-edited" {
		t.Errorf("expected name owner-edited, got %v", edited["name"])
	}
}

func TestHandleEdit_AsAdmin(t *testing.T) {
	router, sessions := setupTorrentRouter()

	ownerToken := createSessionWithGroup(sessions, 200, 5)
	torrentData := buildTorrentFileBytes("edit-handler-admin")
	uploadReq := makeUploadRequest(ownerToken, torrentData, "1")
	uploadRec := httptest.NewRecorder()
	router.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusCreated {
		t.Fatalf("upload failed: %d %s", uploadRec.Code, uploadRec.Body.String())
	}

	var uploadResp map[string]interface{}
	_ = json.Unmarshal(uploadRec.Body.Bytes(), &uploadResp)
	torrent := uploadResp["torrent"].(map[string]interface{})
	id := int64(torrent["id"].(float64))

	// Edit as admin (different user, groupID=1)
	adminToken := createSessionWithGroup(sessions, 201, 1)
	editBody, _ := json.Marshal(map[string]interface{}{
		"name":   "admin-edited",
		"banned": true,
		"free":   true,
	})
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/torrents/%d", id), bytes.NewReader(editBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleEdit_Forbidden(t *testing.T) {
	router, sessions := setupTorrentRouter()

	ownerToken := createSessionWithGroup(sessions, 300, 5)
	torrentData := buildTorrentFileBytes("edit-handler-forbidden")
	uploadReq := makeUploadRequest(ownerToken, torrentData, "1")
	uploadRec := httptest.NewRecorder()
	router.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusCreated {
		t.Fatalf("upload failed: %d %s", uploadRec.Code, uploadRec.Body.String())
	}

	var uploadResp map[string]interface{}
	_ = json.Unmarshal(uploadRec.Body.Bytes(), &uploadResp)
	torrent := uploadResp["torrent"].(map[string]interface{})
	id := int64(torrent["id"].(float64))

	// Edit as another non-admin user
	otherToken := createSessionWithGroup(sessions, 301, 5)
	editBody, _ := json.Marshal(map[string]interface{}{
		"name": "hacked",
	})
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/torrents/%d", id), bytes.NewReader(editBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+otherToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleEdit_StaffFieldsForbidden(t *testing.T) {
	router, sessions := setupTorrentRouter()

	ownerToken := createSessionWithGroup(sessions, 400, 5)
	torrentData := buildTorrentFileBytes("edit-handler-staff")
	uploadReq := makeUploadRequest(ownerToken, torrentData, "1")
	uploadRec := httptest.NewRecorder()
	router.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusCreated {
		t.Fatalf("upload failed: %d %s", uploadRec.Code, uploadRec.Body.String())
	}

	var uploadResp map[string]interface{}
	_ = json.Unmarshal(uploadRec.Body.Bytes(), &uploadResp)
	torrent := uploadResp["torrent"].(map[string]interface{})
	id := int64(torrent["id"].(float64))

	// Owner tries to set staff-only field
	editBody, _ := json.Marshal(map[string]interface{}{
		"banned": true,
	})
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/torrents/%d", id), bytes.NewReader(editBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ownerToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleEdit_NotFound(t *testing.T) {
	router, sessions := setupTorrentRouter()
	token := createSessionWithGroup(sessions, 500, 5)

	editBody, _ := json.Marshal(map[string]interface{}{"name": "ghost"})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/torrents/999", bytes.NewReader(editBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// --- Delete handler tests ---

func TestHandleDelete_AsOwner(t *testing.T) {
	router, sessions := setupTorrentRouter()

	ownerToken := createSessionWithGroup(sessions, 600, 5)
	torrentData := buildTorrentFileBytes("delete-handler-owner")
	uploadReq := makeUploadRequest(ownerToken, torrentData, "1")
	uploadRec := httptest.NewRecorder()
	router.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusCreated {
		t.Fatalf("upload failed: %d %s", uploadRec.Code, uploadRec.Body.String())
	}

	var uploadResp map[string]interface{}
	_ = json.Unmarshal(uploadRec.Body.Bytes(), &uploadResp)
	torrent := uploadResp["torrent"].(map[string]interface{})
	id := int64(torrent["id"].(float64))

	deleteBody, _ := json.Marshal(map[string]string{"reason": "bad content"})
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/torrents/%d", id), bytes.NewReader(deleteBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ownerToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Verify torrent is gone
	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/torrents/%d", id), nil)
	getReq.Header.Set("Authorization", "Bearer "+ownerToken)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", getRec.Code)
	}
}

func TestHandleDelete_AsAdmin(t *testing.T) {
	router, sessions := setupTorrentRouter()

	ownerToken := createSessionWithGroup(sessions, 700, 5)
	torrentData := buildTorrentFileBytes("delete-handler-admin")
	uploadReq := makeUploadRequest(ownerToken, torrentData, "1")
	uploadRec := httptest.NewRecorder()
	router.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusCreated {
		t.Fatalf("upload failed: %d %s", uploadRec.Code, uploadRec.Body.String())
	}

	var uploadResp map[string]interface{}
	_ = json.Unmarshal(uploadRec.Body.Bytes(), &uploadResp)
	torrent := uploadResp["torrent"].(map[string]interface{})
	id := int64(torrent["id"].(float64))

	adminToken := createSessionWithGroup(sessions, 701, 1)
	deleteBody, _ := json.Marshal(map[string]string{"reason": "admin cleanup"})
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/torrents/%d", id), bytes.NewReader(deleteBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDelete_Forbidden(t *testing.T) {
	router, sessions := setupTorrentRouter()

	ownerToken := createSessionWithGroup(sessions, 800, 5)
	torrentData := buildTorrentFileBytes("delete-handler-forbidden")
	uploadReq := makeUploadRequest(ownerToken, torrentData, "1")
	uploadRec := httptest.NewRecorder()
	router.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusCreated {
		t.Fatalf("upload failed: %d %s", uploadRec.Code, uploadRec.Body.String())
	}

	var uploadResp map[string]interface{}
	_ = json.Unmarshal(uploadRec.Body.Bytes(), &uploadResp)
	torrent := uploadResp["torrent"].(map[string]interface{})
	id := int64(torrent["id"].(float64))

	otherToken := createSessionWithGroup(sessions, 801, 5)
	deleteBody, _ := json.Marshal(map[string]string{"reason": "trying to delete"})
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/torrents/%d", id), bytes.NewReader(deleteBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+otherToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDelete_MissingReason(t *testing.T) {
	router, sessions := setupTorrentRouter()

	ownerToken := createSessionWithGroup(sessions, 900, 5)
	torrentData := buildTorrentFileBytes("delete-handler-noreason")
	uploadReq := makeUploadRequest(ownerToken, torrentData, "1")
	uploadRec := httptest.NewRecorder()
	router.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusCreated {
		t.Fatalf("upload failed: %d %s", uploadRec.Code, uploadRec.Body.String())
	}

	var uploadResp map[string]interface{}
	_ = json.Unmarshal(uploadRec.Body.Bytes(), &uploadResp)
	torrent := uploadResp["torrent"].(map[string]interface{})
	id := int64(torrent["id"].(float64))

	deleteBody, _ := json.Marshal(map[string]string{"reason": ""})
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/torrents/%d", id), bytes.NewReader(deleteBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ownerToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDelete_NotFound(t *testing.T) {
	router, sessions := setupTorrentRouter()
	token := createSessionWithGroup(sessions, 1000, 5)

	deleteBody, _ := json.Marshal(map[string]string{"reason": "cleanup"})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/torrents/999", bytes.NewReader(deleteBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// --- Reseed handler tests ---

func TestHandleRequestReseed_Success(t *testing.T) {
	router, sessions := setupTorrentRouter()

	ownerToken := createSessionWithGroup(sessions, 1100, 5)
	torrentData := buildTorrentFileBytes("reseed-handler-test")
	uploadReq := makeUploadRequest(ownerToken, torrentData, "1")
	uploadRec := httptest.NewRecorder()
	router.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusCreated {
		t.Fatalf("upload failed: %d %s", uploadRec.Code, uploadRec.Body.String())
	}

	var uploadResp map[string]interface{}
	_ = json.Unmarshal(uploadRec.Body.Bytes(), &uploadResp)
	torrent := uploadResp["torrent"].(map[string]interface{})
	id := int64(torrent["id"].(float64))

	// Request reseed
	requesterToken := createSessionWithGroup(sessions, 1101, 5)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/torrents/%d/reseed", id), nil)
	req.Header.Set("Authorization", "Bearer "+requesterToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleRequestReseed_Duplicate(t *testing.T) {
	router, sessions := setupTorrentRouter()

	ownerToken := createSessionWithGroup(sessions, 1200, 5)
	torrentData := buildTorrentFileBytes("reseed-dup-handler-test")
	uploadReq := makeUploadRequest(ownerToken, torrentData, "1")
	uploadRec := httptest.NewRecorder()
	router.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusCreated {
		t.Fatalf("upload failed: %d %s", uploadRec.Code, uploadRec.Body.String())
	}

	var uploadResp map[string]interface{}
	_ = json.Unmarshal(uploadRec.Body.Bytes(), &uploadResp)
	torrent := uploadResp["torrent"].(map[string]interface{})
	id := int64(torrent["id"].(float64))

	requesterToken := createSessionWithGroup(sessions, 1201, 5)

	// First request
	req1 := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/torrents/%d/reseed", id), nil)
	req1.Header.Set("Authorization", "Bearer "+requesterToken)
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("first reseed failed: %d %s", rec1.Code, rec1.Body.String())
	}

	// Second request (duplicate)
	req2 := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/torrents/%d/reseed", id), nil)
	req2.Header.Set("Authorization", "Bearer "+requesterToken)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d; body: %s", rec2.Code, rec2.Body.String())
	}
}

func TestHandleRequestReseed_TorrentNotFound(t *testing.T) {
	router, sessions := setupTorrentRouter()
	token := createSessionWithGroup(sessions, 1300, 5)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents/999/reseed", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleGetReseedCount_Success(t *testing.T) {
	router, sessions := setupTorrentRouter()

	ownerToken := createSessionWithGroup(sessions, 1400, 5)
	torrentData := buildTorrentFileBytes("reseed-count-handler-test")
	uploadReq := makeUploadRequest(ownerToken, torrentData, "1")
	uploadRec := httptest.NewRecorder()
	router.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusCreated {
		t.Fatalf("upload failed: %d %s", uploadRec.Code, uploadRec.Body.String())
	}

	var uploadResp map[string]interface{}
	_ = json.Unmarshal(uploadRec.Body.Bytes(), &uploadResp)
	torrent := uploadResp["torrent"].(map[string]interface{})
	id := int64(torrent["id"].(float64))

	// Get count (should be 0)
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/torrents/%d/reseed", id), nil)
	req.Header.Set("Authorization", "Bearer "+ownerToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	count := resp["count"].(float64)
	if count != 0 {
		t.Errorf("expected count 0, got %v", count)
	}
}

func TestHandleRequestReseed_Unauthenticated(t *testing.T) {
	router, _ := setupTorrentRouter()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents/1/reseed", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}
