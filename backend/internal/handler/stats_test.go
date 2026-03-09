package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/williamokano/go-torrent-trader/backend/internal/handler"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

func newTestStatsCache(t *testing.T) (*service.StatsCache, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	cache := service.NewStatsCache(db, rdb, 30*time.Second)
	return cache, mock
}

func TestHandleStatsReturnsJSON(t *testing.T) {
	cache, mock := newTestStatsCache(t)

	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"users", "torrents", "peers", "seeders", "leechers"}).
			AddRow(100, 500, 42, 30, 12))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	rec := httptest.NewRecorder()

	handler.HandleStats(cache)(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	var body struct {
		Stats struct {
			Users    int64 `json:"users"`
			Torrents int64 `json:"torrents"`
			Peers    int64 `json:"peers"`
		} `json:"stats"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	if body.Stats.Users != 100 {
		t.Errorf("expected users=100, got %d", body.Stats.Users)
	}
	if body.Stats.Torrents != 500 {
		t.Errorf("expected torrents=500, got %d", body.Stats.Torrents)
	}
	if body.Stats.Peers != 42 {
		t.Errorf("expected peers=42, got %d", body.Stats.Peers)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

func TestHandleStatsReturnsCachedResult(t *testing.T) {
	cache, mock := newTestStatsCache(t)

	// First call: DB hit
	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"users", "torrents", "peers", "seeders", "leechers"}).
			AddRow(100, 500, 42, 30, 12))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	rec := httptest.NewRecorder()
	handler.HandleStats(cache)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("first call: expected status 200, got %d", rec.Code)
	}

	// Second call: should use cache (no new DB expectation set)
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	rec2 := httptest.NewRecorder()
	handler.HandleStats(cache)(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Errorf("second call: expected status 200, got %d", rec2.Code)
	}

	var body struct {
		Stats struct {
			Users int64 `json:"users"`
		} `json:"stats"`
	}
	if err := json.NewDecoder(rec2.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body.Stats.Users != 100 {
		t.Errorf("expected cached users=100, got %d", body.Stats.Users)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

func TestHandleStatsReturnsErrorOnDBFailure(t *testing.T) {
	cache, mock := newTestStatsCache(t)

	mock.ExpectQuery(`SELECT`).
		WillReturnError(http.ErrAbortHandler) // any error

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	rec := httptest.NewRecorder()

	handler.HandleStats(cache)(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", rec.Code)
	}

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	if body.Error.Code != "internal_error" {
		t.Errorf("expected error code 'internal_error', got '%s'", body.Error.Code)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}
