package handler

import (
	"encoding/json"
	"net/http"

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
		items[i] = map[string]interface{}{
			"key":        s.Key,
			"value":      s.Value,
			"updated_at": s.UpdatedAt,
		}
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
		ErrorResponse(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"key":   key,
		"value": req.Value,
	})
}
