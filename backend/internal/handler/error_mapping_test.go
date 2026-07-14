package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// A storage failure is not the client's fault. Reporting it as a 400 tells the
// caller their request was malformed — and returning err.Error() verbatim hands
// them the internal error text.
func TestUpdateSettingReportsStorageFailureAsInternalError(t *testing.T) {
	repo := newStubSiteSettingsRepo()
	repo.setErr = errors.New("connection refused: dial tcp 10.0.0.5:5432")
	h := newSiteSettingsHandler(repo)

	req := adminAuthed(withURLParam(
		httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/some_key",
			strings.NewReader(`{"value":"anything"}`)),
		"key", "some_key"), 1)
	w := httptest.NewRecorder()
	h.HandleUpdateSetting(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 — a database failure is not a bad request", w.Code)
	}
	if strings.Contains(w.Body.String(), "connection refused") || strings.Contains(w.Body.String(), "10.0.0.5") {
		t.Errorf("response leaks the internal error to the client: %s", w.Body.String())
	}
}

// A genuinely invalid value is still the client's fault and must stay a 400.
func TestUpdateSettingRejectsInvalidValueAsBadRequest(t *testing.T) {
	h := newSiteSettingsHandler(newStubSiteSettingsRepo())

	req := adminAuthed(withURLParam(
		httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/registration_mode",
			strings.NewReader(`{"value":"not_a_mode"}`)),
		"key", service.SettingRegistrationMode), 1)
	w := httptest.NewRecorder()
	h.HandleUpdateSetting(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 — an invalid registration mode is a client error", w.Code)
	}
}

// ErrForbidden must surface as 403. Mapping it to 500 tells the caller the
// server broke when in fact it correctly refused them — and it buries a real
// authorisation denial among the 5xx noise for anyone reading the logs.
//
// The route sits behind staff middleware today, so this is only reachable if
// the handler is ever reused. That is exactly why it should be right.
func TestAdminDeleteTorrentMapsForbiddenTo403(t *testing.T) {
	// A torrent owned by someone else...
	torrents := &stubTorrentRepo{torrents: map[int64]*model.Torrent{
		5: {ID: 5, UploaderID: 999, Name: "someone-elses"},
	}}
	h := newTorrentAdminHandler(torrents, &stubStorage{}, newStubUserRepo())

	// ...deleted by a non-owner who is not staff: the service returns ErrForbidden.
	req := authed(withURLParam(
		httptest.NewRequest(http.MethodDelete, "/api/v1/admin/torrents/5", nil), "id", "5"), 1, 1)
	w := httptest.NewRecorder()
	h.HandleDeleteTorrent(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 — ErrForbidden is a refusal, not a server fault", w.Code)
	}
}
