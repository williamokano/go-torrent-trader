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
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
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
		if effective, reason, overridden := effectiveOverride(r.Context(), h.settings, s); overridden {
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
// Only one setting behaves this way today. It is written as a lookup rather than
// an if so that the next one is added here, where the admin panel already renders
// it, instead of growing its own private surface — which is how the first one
// ended up invisible.
func effectiveOverride(ctx context.Context, settings *service.SiteSettingsService, s model.SiteSetting) (string, string, bool) {
	if s.Key != service.SettingAnnounceLogRetentionDays {
		return "", "", false
	}
	retention := service.ResolveAnnounceRetention(ctx, settings)
	if !retention.Overridden() {
		return "", "", false
	}
	return strconv.Itoa(retention.EffectiveDays),
		fmt.Sprintf("Raw announces are kept for %d days, not %d: the shorter window is held open by %s. "+
			"Shorten the seeding window, or turn promotion off, to make this setting take effect.",
			retention.EffectiveDays, retention.ConfiguredDays, retention.FloorReason),
		true
}
