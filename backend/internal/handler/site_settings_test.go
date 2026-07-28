package handler

import (
	"encoding/json"
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

// A setting the site does not honour has to say so where the operator is
// standing when they form a belief about it — which is this list, not a
// slog.Warn in the server log nobody reads after changing an admin field.
func TestGetAllSettingsReportsAnOverriddenRetentionWindow(t *testing.T) {
	repo := newStubSiteSettingsRepo()
	repo.values[service.SettingAnnounceLogRetentionDays] = "7"
	repo.values[service.SettingPromotionEnabled] = "true"
	// GetAll reads `all` while the resolver's lookups read `values`; the listing
	// has to contain the row for the override to be attached to anything.
	repo.all = []model.SiteSetting{
		{Key: service.SettingAnnounceLogRetentionDays, Value: "7"},
		{Key: service.SettingPromotionEnabled, Value: "true"},
	}
	h := newSiteSettingsHandler(repo)

	w := httptest.NewRecorder()
	h.HandleGetAllSettings(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}

	var body struct {
		Settings []struct {
			Key            string `json:"key"`
			Value          string `json:"value"`
			EffectiveValue string `json:"effective_value"`
			OverrideReason string `json:"override_reason"`
		} `json:"settings"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding: %v", err)
	}

	var found bool
	for _, s := range body.Settings {
		if s.Key != service.SettingAnnounceLogRetentionDays {
			// Nothing else is overridden, and a spurious note on an unrelated
			// setting would be worse than none.
			if s.EffectiveValue != "" || s.OverrideReason != "" {
				t.Errorf("%s reported an override it does not have", s.Key)
			}
			continue
		}
		found = true
		if s.Value != "7" {
			t.Errorf("value = %q, want the stored 7", s.Value)
		}
		if s.EffectiveValue != "31" {
			t.Errorf("effective_value = %q, want 31", s.EffectiveValue)
		}
		if s.OverrideReason == "" {
			t.Error("no override_reason; the operator cannot tell what to change")
		}
	}
	if !found {
		t.Fatal("announce_log_retention_days was not in the response")
	}
}

// And a setting that is being honoured carries no note, so the panel does not
// cry wolf on every row.
func TestGetAllSettingsOmitsTheOverrideWhenTheWindowIsHonoured(t *testing.T) {
	repo := newStubSiteSettingsRepo()
	repo.values[service.SettingAnnounceLogRetentionDays] = "90"
	repo.values[service.SettingPromotionEnabled] = "true"
	repo.all = []model.SiteSetting{
		{Key: service.SettingAnnounceLogRetentionDays, Value: "90"},
		{Key: service.SettingPromotionEnabled, Value: "true"},
	}
	h := newSiteSettingsHandler(repo)

	w := httptest.NewRecorder()
	h.HandleGetAllSettings(w, httptest.NewRequest(http.MethodGet, "/", nil))

	var body struct {
		Settings []map[string]interface{} `json:"settings"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	var seen bool
	for _, s := range body.Settings {
		if s["key"] == service.SettingAnnounceLogRetentionDays {
			seen = true
		}
		if _, present := s["effective_value"]; present {
			t.Errorf("%v carries effective_value while being honoured", s["key"])
		}
	}
	// Without this the test passes on an empty listing, which is how the first
	// version of it passed while the sibling test was failing.
	if !seen {
		t.Fatal("announce_log_retention_days was not in the response")
	}
}
