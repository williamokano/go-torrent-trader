package handler_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/handler"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/williamokano/go-torrent-trader/backend/internal/testutil"
)

// --- Admin schema round-trip -------------------------------------------------

func TestCreateCategoryWithSchema_RoundTrip(t *testing.T) {
	router, sessions, _ := setupCategoryAdminRouter()
	adminToken := createSessionWithGroup(sessions, 4100, 1)

	body, _ := json.Marshal(map[string]interface{}{
		"name": "Movies",
		"metadata_schema": []map[string]interface{}{
			{"key": "year", "label": "Year", "type": "number", "min": 1900, "max": 2100, "integer": true},
			{"key": "codec", "label": "Codec", "type": "select", "options": []string{"x264", "x265"}},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/categories", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	cat := resp["category"].(map[string]interface{})
	schema, ok := cat["metadata_schema"].([]interface{})
	if !ok || len(schema) != 2 {
		t.Fatalf("metadata_schema = %v, want 2 fields", cat["metadata_schema"])
	}
}

func TestCreateCategoryWithInvalidSchema(t *testing.T) {
	router, sessions, _ := setupCategoryAdminRouter()
	adminToken := createSessionWithGroup(sessions, 4101, 1)

	body, _ := json.Marshal(map[string]interface{}{
		"name": "Movies",
		// select without options is invalid
		"metadata_schema": []map[string]interface{}{
			{"key": "codec", "label": "Codec", "type": "select"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/categories", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// --- Resolve endpoint (inheritance) -----------------------------------------

func TestResolveMetadataSchemaEndpoint_Inheritance(t *testing.T) {
	router, _, catRepo := setupCategoryAdminRouter()

	parentID := int64(1)
	catRepo.mu.Lock()
	catRepo.categories = append(catRepo.categories,
		&model.Category{ID: 1, Name: "Video", MetadataSchema: json.RawMessage(`[{"key":"codec","label":"Codec","type":"select","options":["x264","x265"]}]`)},
		&model.Category{ID: 2, Name: "Movies", ParentID: &parentID, MetadataSchema: json.RawMessage(`[{"key":"year","label":"Year","type":"number"}]`)},
	)
	catRepo.nextID = 3
	catRepo.mu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/categories/2/metadata-schema", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	fields := resp["fields"].([]interface{})
	if len(fields) != 2 {
		t.Fatalf("expected 2 effective fields, got %d: %v", len(fields), fields)
	}
	if fields[0].(map[string]interface{})["key"] != "codec" || fields[1].(map[string]interface{})["key"] != "year" {
		t.Fatalf("effective fields out of order: %v", fields)
	}
}

func TestResolveMetadataSchemaEndpoint_NotFound(t *testing.T) {
	router, _, _ := setupCategoryAdminRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/categories/999/metadata-schema", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// --- Upload with metadata ----------------------------------------------------

func setupTorrentMetadataRouter() (http.Handler, *mockCategoryRepo) {
	userRepo := newMockUserRepo()
	torrentRepo := newMockTorrentRepo()
	store := newMockStorage()
	sessions := testutil.NewMemorySessionStore()
	bus := event.NewInMemoryBus()
	reseedRepo := newMockReseedRequestRepo()
	authSvc := service.NewAuthServiceWithTTL(userRepo, sessions, testutil.NewMemoryPasswordResetStore(), &testutil.NoopSender{}, "http://localhost:8080", service.DefaultAccessTokenTTL, service.DefaultRefreshTokenTTL, &mockGroupRepo{}, bus)
	torrentSvc := service.NewTorrentService(nil, torrentRepo, userRepo, store, service.TorrentServiceConfig{AnnounceURL: "http://localhost/announce"}, bus, reseedRepo)
	catRepo := newMockCategoryRepo()
	torrentSvc.SetCategoryRepo(catRepo)
	catSvc := service.NewCategoryService(catRepo)

	router := handler.NewRouter(&handler.Deps{
		AuthService:     authSvc,
		SessionStore:    sessions,
		TorrentService:  torrentSvc,
		UserRepo:        userRepo,
		CategoryService: catSvc,
		CategoryRepo:    catRepo,
	})
	return router, catRepo
}

func makeUploadRequestWithMetadata(token string, data []byte, categoryID, metadataJSON string) *http.Request {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("torrent_file", "test.torrent")
	_, _ = fw.Write(data)
	_ = w.WriteField("category_id", categoryID)
	if metadataJSON != "" {
		_ = w.WriteField("metadata", metadataJSON)
	}
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func seedMoviesCategory(catRepo *mockCategoryRepo) {
	catRepo.mu.Lock()
	catRepo.categories = append(catRepo.categories, &model.Category{
		ID: 1, Name: "Movies", Slug: "movies",
		MetadataSchema: json.RawMessage(`[{"key":"year","label":"Year","type":"number","min":1900,"max":2100,"integer":true}]`),
	})
	catRepo.nextID = 2
	catRepo.mu.Unlock()
}

func TestUploadWithValidMetadata(t *testing.T) {
	router, catRepo := setupTorrentMetadataRouter()
	seedMoviesCategory(catRepo)
	token := registerAndGetToken(t, router)

	req := makeUploadRequestWithMetadata(token, buildTorrentFileBytes("meta-upload-ok"), "1", `{"year":2024}`)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	meta := resp["torrent"].(map[string]interface{})["metadata"].(map[string]interface{})
	if meta["year"].(float64) != 2024 {
		t.Fatalf("metadata year = %v, want 2024", meta["year"])
	}
}

func TestUploadWithInvalidMetadataJSON(t *testing.T) {
	router, catRepo := setupTorrentMetadataRouter()
	seedMoviesCategory(catRepo)
	token := registerAndGetToken(t, router)

	req := makeUploadRequestWithMetadata(token, buildTorrentFileBytes("meta-upload-badjson"), "1", `{not json`)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestUploadWithInvalidMetadataValues(t *testing.T) {
	router, catRepo := setupTorrentMetadataRouter()
	seedMoviesCategory(catRepo)
	token := registerAndGetToken(t, router)

	// year out of range -> 422 validation error
	req := makeUploadRequestWithMetadata(token, buildTorrentFileBytes("meta-upload-range"), "1", `{"year":3000}`)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestUploadWithUnknownMetadataField(t *testing.T) {
	router, catRepo := setupTorrentMetadataRouter()
	seedMoviesCategory(catRepo)
	token := registerAndGetToken(t, router)

	req := makeUploadRequestWithMetadata(token, buildTorrentFileBytes("meta-upload-unknown"), "1", `{"bogus":1}`)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d; body: %s", rec.Code, rec.Body.String())
	}
}
