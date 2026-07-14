package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// The service silently substitutes page 1 / 25 per page when the caller omits
// them. If the handler echoes back the raw query values instead of the resolved
// ones, the response claims `per_page: 0` for a body that actually holds 25
// rows — and any client computing total/per_page divides by zero.

func TestActivityLogEchoesResolvedPaginationWhenOmitted(t *testing.T) {
	logs := &stubActivityLogRepo{total: 100}
	h := NewActivityLogHandler(service.NewActivityLogService(logs))

	// No page / per_page in the query string.
	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/activity-logs", nil), 1)
	w := httptest.NewRecorder()
	h.HandleList(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := decodeBody(t, w)

	if got := body["page"]; got != float64(1) {
		t.Errorf("page = %v, want 1 — the response must report the page actually served", got)
	}
	if got := body["per_page"]; got != float64(25) {
		t.Errorf("per_page = %v, want 25 — reporting 0 makes total/per_page a division by zero", got)
	}

	// And the value echoed must be the value the service was asked for.
	if logs.lastOpts.Page != 1 || logs.lastOpts.PerPage != 25 {
		t.Errorf("service queried page=%d per_page=%d, want 1/25 — the echo must match the query",
			logs.lastOpts.Page, logs.lastOpts.PerPage)
	}
}

func TestWarningListEchoesResolvedPaginationWhenOmitted(t *testing.T) {
	warnings := newStubWarningRepo()
	h := newWarningHandler(warnings, newStubUserRepo())

	req := staffAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/warnings", nil), 1)
	w := httptest.NewRecorder()
	h.HandleListWarnings(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}
	body := decodeBody(t, w)

	if got := body["page"]; got != float64(1) {
		t.Errorf("page = %v, want 1", got)
	}
	if got := body["per_page"]; got != float64(25) {
		t.Errorf("per_page = %v, want 25", got)
	}
	if warnings.lastOpts.Page != 1 || warnings.lastOpts.PerPage != 25 {
		t.Errorf("service queried page=%d per_page=%d, want 1/25",
			warnings.lastOpts.Page, warnings.lastOpts.PerPage)
	}
}

// An explicit page/per_page must still be honoured, not overwritten by defaults.
func TestActivityLogHonoursExplicitPagination(t *testing.T) {
	logs := &stubActivityLogRepo{total: 100}
	h := NewActivityLogHandler(service.NewActivityLogService(logs))

	req := adminAuthed(httptest.NewRequest(http.MethodGet,
		"/api/v1/activity-logs?page=3&per_page=50", nil), 1)
	w := httptest.NewRecorder()
	h.HandleList(w, req)

	body := decodeBody(t, w)
	if body["page"] != float64(3) || body["per_page"] != float64(50) {
		t.Errorf("page=%v per_page=%v, want 3/50 — explicit values must survive", body["page"], body["per_page"])
	}
}
