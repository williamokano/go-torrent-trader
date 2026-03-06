package handler_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/handler"
)

func TestJSONSetsContentTypeAndStatus(t *testing.T) {
	rec := httptest.NewRecorder()

	handler.JSON(rec, 201, map[string]string{"key": "value"})

	if rec.Code != 201 {
		t.Errorf("expected status 201, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}

	if body["key"] != "value" {
		t.Errorf("expected key=value, got key=%s", body["key"])
	}
}

func TestErrorResponseStructure(t *testing.T) {
	rec := httptest.NewRecorder()

	handler.ErrorResponse(rec, 400, "bad_request", "invalid input")

	if rec.Code != 400 {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	var body map[string]map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}

	errObj, ok := body["error"]
	if !ok {
		t.Fatal("expected 'error' key in response")
	}

	if errObj["code"] != "bad_request" {
		t.Errorf("expected code=bad_request, got %s", errObj["code"])
	}

	if errObj["message"] != "invalid input" {
		t.Errorf("expected message='invalid input', got %s", errObj["message"])
	}
}
