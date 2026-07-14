package handler

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// Error and validation paths for the ban handler that the router-level tests in
// ban_test.go do not reach.

func newBanHandler(bans *stubBanRepo) *BanHandler {
	return NewBanHandler(service.NewBanService(bans, event.NewInMemoryBus()))
}

func TestListEmailBansReturnsEmptyArrayNotNull(t *testing.T) {
	h := newBanHandler(&stubBanRepo{})

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/bans/emails", nil), 1)
	w := httptest.NewRecorder()
	h.HandleListEmailBans(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if items, ok := decodeBody(t, w)["email_bans"].([]interface{}); !ok || len(items) != 0 {
		t.Errorf("email_bans = %v, want an empty array", items)
	}
}

func TestListEmailBansSurfacesStoreFailure(t *testing.T) {
	h := newBanHandler(&stubBanRepo{listErr: errStub})

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/bans/emails", nil), 1)
	w := httptest.NewRecorder()
	h.HandleListEmailBans(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestCreateEmailBanRequiresAuth(t *testing.T) {
	h := newBanHandler(&stubBanRepo{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/bans/emails", strings.NewReader(`{"pattern":"*@bad.test"}`))
	w := httptest.NewRecorder()
	h.HandleCreateEmailBan(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestCreateEmailBanValidatesInput(t *testing.T) {
	for _, body := range []string{`{`, `{"pattern":""}`} {
		bans := &stubBanRepo{}
		h := newBanHandler(bans)

		req := adminAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/bans/emails", strings.NewReader(body)), 1)
		w := httptest.NewRecorder()
		h.HandleCreateEmailBan(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, w.Code)
		}
		if len(bans.emailBans) != 0 {
			t.Error("a ban was created despite the invalid request")
		}
	}
}

func TestCreateEmailBanStoresPattern(t *testing.T) {
	bans := &stubBanRepo{}
	h := newBanHandler(bans)

	req := adminAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/bans/emails",
		strings.NewReader(`{"pattern":"*@bad.test","reason":"spam"}`)), 1)
	w := httptest.NewRecorder()
	h.HandleCreateEmailBan(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	if len(bans.emailBans) != 1 || bans.emailBans[0].Pattern != "*@bad.test" {
		t.Fatalf("email bans = %+v, want the pattern stored", bans.emailBans)
	}
	if bans.emailBans[0].CreatedBy == nil || *bans.emailBans[0].CreatedBy != 1 {
		t.Errorf("created_by = %v, want the authenticated actor 1", bans.emailBans[0].CreatedBy)
	}
	if _, ok := decodeBody(t, w)["email_ban"]; !ok {
		t.Error("response is missing the email_ban")
	}
}

func TestCreateEmailBanSurfacesStoreFailure(t *testing.T) {
	h := newBanHandler(&stubBanRepo{createErr: errStub})

	req := adminAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/bans/emails",
		strings.NewReader(`{"pattern":"*@bad.test"}`)), 1)
	w := httptest.NewRecorder()
	h.HandleCreateEmailBan(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestDeleteEmailBanRequiresAuth(t *testing.T) {
	h := newBanHandler(&stubBanRepo{})

	req := withURLParam(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/bans/emails/1", nil), "id", "1")
	w := httptest.NewRecorder()
	h.HandleDeleteEmailBan(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestDeleteEmailBanRejectsInvalidID(t *testing.T) {
	h := newBanHandler(&stubBanRepo{})

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/bans/emails/0", nil), 1)
	req = withURLParam(req, "id", "0")
	w := httptest.NewRecorder()
	h.HandleDeleteEmailBan(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestDeleteEmailBanReturns404WhenMissing(t *testing.T) {
	h := newBanHandler(&stubBanRepo{deleteErr: sql.ErrNoRows})

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/bans/emails/1", nil), 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleDeleteEmailBan(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestDeleteEmailBanSurfacesStoreFailure(t *testing.T) {
	h := newBanHandler(&stubBanRepo{deleteErr: errStub})

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/bans/emails/1", nil), 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleDeleteEmailBan(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// --- IP bans ----------------------------------------------------------------

func TestListIPBansReturnsEmptyArrayNotNull(t *testing.T) {
	h := newBanHandler(&stubBanRepo{})

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/bans/ips", nil), 1)
	w := httptest.NewRecorder()
	h.HandleListIPBans(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if items, ok := decodeBody(t, w)["ip_bans"].([]interface{}); !ok || len(items) != 0 {
		t.Errorf("ip_bans = %v, want an empty array", items)
	}
}

func TestListIPBansSurfacesStoreFailure(t *testing.T) {
	h := newBanHandler(&stubBanRepo{listErr: errStub})

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/bans/ips", nil), 1)
	w := httptest.NewRecorder()
	h.HandleListIPBans(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestCreateIPBanRequiresAuth(t *testing.T) {
	h := newBanHandler(&stubBanRepo{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/bans/ips", strings.NewReader(`{"ip_range":"1.2.3.4"}`))
	w := httptest.NewRecorder()
	h.HandleCreateIPBan(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestCreateIPBanValidatesInput(t *testing.T) {
	for _, body := range []string{`{`, `{"ip_range":""}`} {
		bans := &stubBanRepo{}
		h := newBanHandler(bans)

		req := adminAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/bans/ips", strings.NewReader(body)), 1)
		w := httptest.NewRecorder()
		h.HandleCreateIPBan(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, w.Code)
		}
		if len(bans.ipBans) != 0 {
			t.Error("a ban was created despite the invalid request")
		}
	}
}

func TestCreateIPBanStoresRange(t *testing.T) {
	bans := &stubBanRepo{}
	h := newBanHandler(bans)

	req := adminAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/bans/ips",
		strings.NewReader(`{"ip_range":"203.0.113.0/24","reason":"abuse"}`)), 1)
	w := httptest.NewRecorder()
	h.HandleCreateIPBan(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	if len(bans.ipBans) != 1 || bans.ipBans[0].IPRange != "203.0.113.0/24" {
		t.Fatalf("ip bans = %+v, want the range stored", bans.ipBans)
	}
	ban := bans.ipBans[0]
	if ban.Reason == nil || *ban.Reason != "abuse" {
		t.Errorf("reason = %v, want \"abuse\"", ban.Reason)
	}
}

func TestCreateIPBanSurfacesStoreFailure(t *testing.T) {
	h := newBanHandler(&stubBanRepo{createErr: errStub})

	req := adminAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/bans/ips",
		strings.NewReader(`{"ip_range":"1.2.3.4"}`)), 1)
	w := httptest.NewRecorder()
	h.HandleCreateIPBan(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestDeleteIPBanRequiresAuth(t *testing.T) {
	h := newBanHandler(&stubBanRepo{})

	req := withURLParam(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/bans/ips/1", nil), "id", "1")
	w := httptest.NewRecorder()
	h.HandleDeleteIPBan(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestDeleteIPBanRejectsInvalidID(t *testing.T) {
	h := newBanHandler(&stubBanRepo{})

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/bans/ips/abc", nil), 1)
	req = withURLParam(req, "id", "abc")
	w := httptest.NewRecorder()
	h.HandleDeleteIPBan(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestDeleteIPBanReturns404WhenMissing(t *testing.T) {
	h := newBanHandler(&stubBanRepo{deleteErr: sql.ErrNoRows})

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/bans/ips/1", nil), 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleDeleteIPBan(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestDeleteIPBanRemovesBan(t *testing.T) {
	bans := &stubBanRepo{}
	h := newBanHandler(bans)

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/bans/ips/4", nil), 1)
	req = withURLParam(req, "id", "4")
	w := httptest.NewRecorder()
	h.HandleDeleteIPBan(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
	if len(bans.deleted) != 1 || bans.deleted[0] != 4 {
		t.Errorf("deleted = %v, want [4]", bans.deleted)
	}
}

func TestDeleteIPBanSurfacesStoreFailure(t *testing.T) {
	h := newBanHandler(&stubBanRepo{deleteErr: errStub})

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/bans/ips/1", nil), 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleDeleteIPBan(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}
