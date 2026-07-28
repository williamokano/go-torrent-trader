package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// SiteSettingsHandler handles site settings HTTP endpoints.
type SiteSettingsHandler struct {
	settings *service.SiteSettingsService
}

// NewSiteSettingsHandler creates a new SiteSettingsHandler.
func NewSiteSettingsHandler(settings *service.SiteSettingsService) *SiteSettingsHandler {
	return &SiteSettingsHandler{settings: settings}
}

// HandleGetRegistrationMode handles GET /api/v1/auth/registration-mode (public).
func (h *SiteSettingsHandler) HandleGetRegistrationMode(w http.ResponseWriter, r *http.Request) {
	mode := h.settings.GetRegistrationMode(r.Context())
	JSON(w, http.StatusOK, map[string]interface{}{
		"mode": mode,
	})
}

// HandleGetAllSettings handles GET /api/v1/admin/settings (admin only).
func (h *SiteSettingsHandler) HandleGetAllSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.settings.GetAll(r.Context())
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to load settings")
		return
	}

	items := make([]map[string]interface{}, len(settings))
	for i, s := range settings {
		item := map[string]interface{}{
			"key":        s.Key,
			"value":      s.Value,
			"updated_at": s.UpdatedAt,
		}
		// A saved value the site does not honour has to say so here. Anywhere else
		// — a log line, a doc — is somewhere the operator is not looking at the
		// moment they form a belief about what the setting does.
		if effective, reason, overridden := effectiveOverride(r.Context(), h.settings, s.Key, s.Value); overridden {
			item["effective_value"] = effective
			item["override_reason"] = reason
		}
		items[i] = item
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"settings": items,
	})
}

// HandleUpdateSetting handles PUT /api/v1/admin/settings/{key} (admin only).
func (h *SiteSettingsHandler) HandleUpdateSetting(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "missing setting key")
		return
	}

	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	var req struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	actor := event.Actor{ID: userID}
	if err := h.settings.Set(r.Context(), key, req.Value, actor); err != nil {
		// Only a rejected value is the client's fault. Everything else is a
		// storage failure, and must not be reported as a 400 carrying the
		// internal error text.
		if errors.Is(err, service.ErrInvalidSetting) {
			ErrorResponse(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		slog.Error("failed to update site setting", "key", key, "error", err)
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to update setting")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"key":   key,
		"value": req.Value,
	})
}

// effectiveOverride reports the value the site actually acts on, when that is not
// the value stored.
//
// Only one setting behaves this way today, so this is a switch on the key rather
// than a registry. What matters is that the next one is added here, where the
// admin panel already renders it, instead of growing its own private surface —
// which is how the first one ended up invisible. If a second arrives, this
// becomes a map of key to resolver before it becomes a longer switch.
func effectiveOverride(ctx context.Context, settings *service.SiteSettingsService, key, stored string) (string, string, bool) {
	if key != service.SettingAnnounceLogRetentionDays {
		return "", "", false
	}
	retention := service.ResolveAnnounceRetention(ctx, settings)
	effective := strconv.Itoa(retention.EffectiveDays)

	switch {
	case retention.Overridden():
		return effective, fmt.Sprintf(
			"Raw announces are kept for %d days, not %d: the shorter window is held open by %s. "+
				"Shorten the seeding window, or turn promotion off, to make this setting take effect.",
			retention.EffectiveDays, retention.ConfiguredDays, retention.FloorReason), true

	// A stored value that does not survive being read is just as much "not the
	// value in force" as one a floor overrode, and it is likelier to be a
	// mistake. A negative window reads as disabled, and anything unparseable
	// falls back to the default — in both cases the panel would otherwise show a
	// number that nothing acts on. The condition is the general one rather than a
	// list of the two known cases, so a third cannot slip past silently.
	case effective != stored:
		return effective, fmt.Sprintf(
			"%q is not a usable number of days, so raw announces are kept for %d instead. "+
				"Set 0 to keep every announce indefinitely, or a positive number of days.",
			stored, retention.EffectiveDays), true
	}
	return "", "", false
}
