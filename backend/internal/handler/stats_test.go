package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/williamokano/go-torrent-trader/backend/internal/handler"
)

func TestHandleStatsReturnsJSON(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck // sqlmock close is safe to ignore

	mock.ExpectQuery(`SELECT`).
		WillReturnRows(sqlmock.NewRows([]string{"users", "torrents", "peers"}).
			AddRow(100, 500, 42))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	rec := httptest.NewRecorder()

	handler.HandleStats(db)(rec, req)

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

func TestHandleStatsReturnsErrorOnDBFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck // sqlmock close is safe to ignore

	mock.ExpectQuery(`SELECT`).
		WillReturnError(http.ErrAbortHandler) // any error

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	rec := httptest.NewRecorder()

	handler.HandleStats(db)(rec, req)

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
