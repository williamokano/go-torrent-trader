package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

func newSiteSettingsHandler(repo *stubSiteSettingsRepo) *SiteSettingsHandler {
	return NewSiteSettingsHandler(service.NewSiteSettingsService(repo, event.NewInMemoryBus()))
}

// --- registration mode (public) ---------------------------------------------

// The registration mode endpoint is public — the login page reads it before the
// user has a session — and must default to the safe invite_only when unset.
func TestGetRegistrationModeDefaultsToInviteOnly(t *testing.T) {
	h := newSiteSettingsHandler(newStubSiteSettingsRepo())

	w := httptest.NewRecorder()
	h.HandleGetRegistrationMode(w, httptest.NewRequest(http.MethodGet, "/api/v1/auth/registration-mode", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := decodeBody(t, w)["mode"]; got != service.RegistrationModeInviteOnly {
		t.Errorf("mode = %v, want %q when the setting is absent", got, service.RegistrationModeInviteOnly)
	}
}

func TestGetRegistrationModeReturnsStoredValue(t *testing.T) {
	repo := newStubSiteSettingsRepo()
	repo.values[service.SettingRegistrationMode] = service.RegistrationModeOpen
	h := newSiteSettingsHandler(repo)

	w := httptest.NewRecorder()
	h.HandleGetRegistrationMode(w, httptest.NewRequest(http.MethodGet, "/api/v1/auth/registration-mode", nil))

	if got := decodeBody(t, w)["mode"]; got != service.RegistrationModeOpen {
		t.Errorf("mode = %v, want %q", got, service.RegistrationModeOpen)
	}
}

// A garbage value in the database must not open registration by accident.
func TestGetRegistrationModeFallsBackOnGarbageValue(t *testing.T) {
	repo := newStubSiteSettingsRepo()
	repo.values[service.SettingRegistrationMode] = "wide_open"
	h := newSiteSettingsHandler(repo)

	w := httptest.NewRecorder()
	h.HandleGetRegistrationMode(w, httptest.NewRequest(http.MethodGet, "/api/v1/auth/registration-mode", nil))

	if got := decodeBody(t, w)["mode"]; got != service.RegistrationModeInviteOnly {
		t.Errorf("mode = %v, want the safe %q for an unrecognised stored value", got, service.RegistrationModeInviteOnly)
	}
}

// --- all settings -----------------------------------------------------------

func TestGetAllSettingsReturnsItems(t *testing.T) {
	repo := newStubSiteSettingsRepo()
	repo.all = []model.SiteSetting{
		{Key: "registration_mode", Value: "open", UpdatedAt: time.Now()},
		{Key: "chat_rate_limit_max", Value: "5", UpdatedAt: time.Now()},
	}
	h := newSiteSettingsHandler(repo)

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil), 1)
	w := httptest.NewRecorder()
	h.HandleGetAllSettings(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	items, ok := decodeBody(t, w)["settings"].([]interface{})
	if !ok || len(items) != 2 {
		t.Fatalf("settings = %v, want 2 items", items)
	}
	first := items[0].(map[string]interface{})
	for _, key := range []string{"key", "value", "updated_at"} {
		if _, present := first[key]; !present {
			t.Errorf("setting is missing key %q", key)
		}
	}
}

func TestGetAllSettingsSurfacesStoreFailure(t *testing.T) {
	repo := newStubSiteSettingsRepo()
	repo.getAllErr = errStub
	h := newSiteSettingsHandler(repo)

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil), 1)
	w := httptest.NewRecorder()
	h.HandleGetAllSettings(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// --- update -----------------------------------------------------------------

func TestUpdateSettingRejectsMissingKey(t *testing.T) {
	h := newSiteSettingsHandler(newStubSiteSettingsRepo())

	// No chi route param at all — the key is empty.
	req := adminAuthed(httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/", strings.NewReader(`{"value":"open"}`)), 1)
	w := httptest.NewRecorder()
	h.HandleUpdateSetting(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a missing setting key", w.Code)
	}
}

func TestUpdateSettingRequiresAuth(t *testing.T) {
	h := newSiteSettingsHandler(newStubSiteSettingsRepo())

	req := withURLParam(httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/registration_mode",
		strings.NewReader(`{"value":"open"}`)), "key", "registration_mode")
	w := httptest.NewRecorder()
	h.HandleUpdateSetting(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestUpdateSettingRejectsBadJSON(t *testing.T) {
	h := newSiteSettingsHandler(newStubSiteSettingsRepo())

	req := adminAuthed(httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/registration_mode", strings.NewReader("{")), 1)
	req = withURLParam(req, "key", "registration_mode")
	w := httptest.NewRecorder()
	h.HandleUpdateSetting(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a malformed body", w.Code)
	}
}

// registration_mode is a validated key: an unknown value must be rejected
// rather than silently written.
func TestUpdateSettingRejectsInvalidRegistrationMode(t *testing.T) {
	repo := newStubSiteSettingsRepo()
	h := newSiteSettingsHandler(repo)

	req := adminAuthed(httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/registration_mode",
		strings.NewReader(`{"value":"anyone_welcome"}`)), 1)
	req = withURLParam(req, "key", "registration_mode")
	w := httptest.NewRecorder()
	h.HandleUpdateSetting(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if _, written := repo.set["registration_mode"]; written {
		t.Error("the invalid value was written to the store anyway")
	}
}

func TestUpdateSettingPersistsValue(t *testing.T) {
	repo := newStubSiteSettingsRepo()
	h := newSiteSettingsHandler(repo)

	req := adminAuthed(httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/registration_mode",
		strings.NewReader(`{"value":"open"}`)), 1)
	req = withURLParam(req, "key", "registration_mode")
	w := httptest.NewRecorder()
	h.HandleUpdateSetting(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if repo.set["registration_mode"] != "open" {
		t.Errorf("stored value = %q, want \"open\"", repo.set["registration_mode"])
	}
	body := decodeBody(t, w)
	if body["key"] != "registration_mode" || body["value"] != "open" {
		t.Errorf("response = %v, want the echoed key/value", body)
	}
}

// Arbitrary (unvalidated) keys are allowed through — only known keys carry
// value constraints.
func TestUpdateSettingAcceptsUnvalidatedKey(t *testing.T) {
	repo := newStubSiteSettingsRepo()
	h := newSiteSettingsHandler(repo)

	req := adminAuthed(httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/chat_rate_limit_max",
		strings.NewReader(`{"value":"9"}`)), 1)
	req = withURLParam(req, "key", "chat_rate_limit_max")
	w := httptest.NewRecorder()
	h.HandleUpdateSetting(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if repo.set["chat_rate_limit_max"] != "9" {
		t.Errorf("stored value = %q, want \"9\"", repo.set["chat_rate_limit_max"])
	}
}

// A write that fails in the store must not be reported as a success.
func TestUpdateSettingDoesNotReportSuccessOnStoreFailure(t *testing.T) {
	repo := newStubSiteSettingsRepo()
	repo.setErr = errStub
	h := newSiteSettingsHandler(repo)

	req := adminAuthed(httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/chat_rate_limit_max",
		strings.NewReader(`{"value":"9"}`)), 1)
	req = withURLParam(req, "key", "chat_rate_limit_max")
	w := httptest.NewRecorder()
	h.HandleUpdateSetting(w, req)

	if w.Code < 400 {
		t.Errorf("status = %d, want an error status when the store write fails", w.Code)
	}
	if _, hasErr := decodeBody(t, w)["error"]; !hasErr {
		t.Error("response has no error object")
	}
}
