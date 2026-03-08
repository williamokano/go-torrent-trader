package handler_test

import (
	"context"
	"encoding/xml"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/handler"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/williamokano/go-torrent-trader/backend/internal/testutil"
)

// passkeyAwareMockUserRepo extends mockUserRepo to support GetByPasskey lookups.
type passkeyAwareMockUserRepo struct {
	*mockUserRepo
}

func (m *passkeyAwareMockUserRepo) GetByPasskey(_ context.Context, passkey string) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.Passkey != nil && *u.Passkey == passkey {
			return u, nil
		}
	}
	return nil, errors.New("not found")
}

func newPasskeyAwareMockUserRepo() *passkeyAwareMockUserRepo {
	return &passkeyAwareMockUserRepo{mockUserRepo: newMockUserRepo()}
}

func setupRSSRouter() (http.Handler, service.SessionStore, *passkeyAwareMockUserRepo) {
	userRepo := newPasskeyAwareMockUserRepo()
	torrentRepo := newMockTorrentRepo()
	store := newMockStorage()
	sessions := testutil.NewMemorySessionStore()
	bus := event.NewInMemoryBus()
	authSvc := service.NewAuthServiceWithTTL(userRepo, sessions, testutil.NewMemoryPasswordResetStore(), &testutil.NoopSender{}, "http://localhost:8080", service.DefaultAccessTokenTTL, service.DefaultRefreshTokenTTL, &mockGroupRepo{}, bus)
	torrentSvc := service.NewTorrentService(torrentRepo, userRepo, store, service.TorrentServiceConfig{AnnounceURL: "http://localhost/announce"}, bus)

	router := handler.NewRouter(&handler.Deps{
		AuthService:    authSvc,
		SessionStore:   sessions,
		TorrentService: torrentSvc,
		UserRepo:       userRepo,
		RSSConfig: &handler.RSSConfig{
			SiteName: "TestTracker",
			BaseURL:  "http://localhost:5173",
			ApiURL:   "http://localhost:8080",
		},
	})
	return router, sessions, userRepo
}

func getPasskeyFromRepo(t *testing.T, repo *passkeyAwareMockUserRepo) string {
	t.Helper()
	repo.mu.Lock()
	defer repo.mu.Unlock()
	for _, u := range repo.users {
		if u.Passkey != nil && *u.Passkey != "" {
			return *u.Passkey
		}
	}
	t.Fatal("no user with passkey found in repo")
	return ""
}

func TestHandleRSS_MissingPasskey(t *testing.T) {
	router, _, _ := setupRSSRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/rss", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleRSS_InvalidPasskey(t *testing.T) {
	router, _, _ := setupRSSRouter()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/rss?passkey=invalid-key", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleRSS_ValidPasskey_ReturnsXML(t *testing.T) {
	router, _, userRepo := setupRSSRouter()

	// Register a user to create one with a passkey
	token := registerAndGetToken(t, router)
	passkey := getPasskeyFromRepo(t, userRepo)

	// Upload a torrent
	torrentData := buildTorrentFileBytes("rss-test-torrent")
	uploadReq := makeUploadRequest(token, torrentData, "1")
	uploadRec := httptest.NewRecorder()
	router.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code != http.StatusCreated {
		t.Fatalf("upload failed: %d %s", uploadRec.Code, uploadRec.Body.String())
	}

	// Fetch RSS feed
	req := httptest.NewRequest(http.MethodGet, "/api/v1/rss?passkey="+passkey, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/rss+xml; charset=utf-8" {
		t.Errorf("expected Content-Type application/rss+xml; charset=utf-8, got %s", contentType)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "<?xml") {
		t.Error("response should start with XML declaration")
	}
	if !strings.Contains(body, `<rss version="2.0">`) {
		t.Error("response should contain RSS 2.0 element")
	}
	if !strings.Contains(body, "<title>TestTracker</title>") {
		t.Error("response should contain site name in channel title")
	}
	if !strings.Contains(body, "rss-test-torrent") {
		t.Error("response should contain torrent name")
	}
	if !strings.Contains(body, "application/x-bittorrent") {
		t.Error("response should contain enclosure type")
	}
	if !strings.Contains(body, "passkey="+passkey) {
		t.Error("response should contain passkey in download URL")
	}
	if !strings.Contains(body, "http://localhost:5173/torrent/") {
		t.Error("response should contain frontend URL for torrent link")
	}

	// Verify it's valid XML
	var feed struct {
		XMLName xml.Name `xml:"rss"`
		Channel struct {
			Title string `xml:"title"`
			Items []struct {
				Title string `xml:"title"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	if err := xml.Unmarshal([]byte(body), &feed); err != nil {
		t.Fatalf("response is not valid XML: %v", err)
	}
	if len(feed.Channel.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(feed.Channel.Items))
	}
}

func TestHandleRSS_EmptyFeed(t *testing.T) {
	router, _, userRepo := setupRSSRouter()

	// Register a user but don't upload any torrents
	_ = registerAndGetToken(t, router)
	passkey := getPasskeyFromRepo(t, userRepo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/rss?passkey="+passkey, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Should still be valid RSS XML with no items
	var feed struct {
		XMLName xml.Name `xml:"rss"`
		Channel struct {
			Items []struct{} `xml:"item"`
		} `xml:"channel"`
	}
	if err := xml.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
		t.Fatalf("response is not valid XML: %v", err)
	}
	if len(feed.Channel.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(feed.Channel.Items))
	}
}

func TestHandleRSS_CategoryFilter(t *testing.T) {
	router, _, userRepo := setupRSSRouter()

	token := registerAndGetToken(t, router)
	passkey := getPasskeyFromRepo(t, userRepo)

	// Upload two torrents
	torrentData1 := buildTorrentFileBytes("cat-filter-1")
	uploadReq1 := makeUploadRequest(token, torrentData1, "1")
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, uploadReq1)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("upload 1 failed: %d %s", rec1.Code, rec1.Body.String())
	}

	torrentData2 := buildTorrentFileBytes("cat-filter-2")
	uploadReq2 := makeUploadRequest(token, torrentData2, "2")
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, uploadReq2)
	if rec2.Code != http.StatusCreated {
		t.Fatalf("upload 2 failed: %d %s", rec2.Code, rec2.Body.String())
	}

	// Fetch with cat filter — the mock doesn't filter but we verify the request succeeds
	req := httptest.NewRequest(http.MethodGet, "/api/v1/rss?passkey="+passkey+"&cat=1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Verify valid XML
	var feed struct {
		XMLName xml.Name `xml:"rss"`
	}
	if err := xml.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
		t.Fatalf("response is not valid XML: %v", err)
	}
}

func TestHandleRSS_LimitParam(t *testing.T) {
	router, _, userRepo := setupRSSRouter()

	_ = registerAndGetToken(t, router)
	passkey := getPasskeyFromRepo(t, userRepo)

	// Verify limit=1 request works
	req := httptest.NewRequest(http.MethodGet, "/api/v1/rss?passkey="+passkey+"&limit=1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// Ensure unused imports are referenced.
var _ repository.UserRepository = (*passkeyAwareMockUserRepo)(nil)
