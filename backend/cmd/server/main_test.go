package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/database"
	"github.com/williamokano/go-torrent-trader/backend/internal/handler"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository/postgres"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
	"github.com/williamokano/go-torrent-trader/backend/internal/testutil"
)

func TestHealthzReturns200WithStatusOK(t *testing.T) {
	router := handler.NewRouter(nil)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %s", body["status"])
	}
}

func TestHealthzRejectsNonGET(t *testing.T) {
	router := handler.NewRouter(nil)

	req := httptest.NewRequest(http.MethodPost, "/healthz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Error("expected non-200 status for POST /healthz")
	}
}

func TestIntegrationServerStartsWithDB(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	db, err := database.Connect(dbURL, database.DefaultConnConfig())
	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := database.RunMigrations(db, "../../migrations"); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	userRepo := postgres.NewUserRepo(db)
	sessionStore := testutil.NewMemorySessionStore()
	authService := service.NewAuthService(userRepo, sessionStore, testutil.NewMemoryPasswordResetStore(), &testutil.NoopSender{}, "http://localhost:8080")

	deps := &handler.Deps{
		AuthService:  authService,
		SessionStore: sessionStore,
	}

	router := handler.NewRouter(deps)

	// Verify healthz still works with full deps
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	// Verify auth register endpoint is wired up (should return 400 for empty body, not 404)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", nil)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code == http.StatusNotFound {
		t.Error("expected auth register endpoint to be registered, got 404")
	}
}
