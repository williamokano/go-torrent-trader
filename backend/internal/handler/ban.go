package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// BanHandler handles admin ban HTTP endpoints.
type BanHandler struct {
	bans *service.BanService
}

// NewBanHandler creates a new BanHandler.
func NewBanHandler(bans *service.BanService) *BanHandler {
	return &BanHandler{bans: bans}
}

// HandleListEmailBans handles GET /api/v1/admin/bans/emails.
func (h *BanHandler) HandleListEmailBans(w http.ResponseWriter, r *http.Request) {
	bans, err := h.bans.ListEmailBans(r.Context())
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list email bans")
		return
	}
	if bans == nil {
		bans = []model.BannedEmail{}
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"email_bans": bans,
	})
}

// HandleCreateEmailBan handles POST /api/v1/admin/bans/emails.
func (h *BanHandler) HandleCreateEmailBan(w http.ResponseWriter, r *http.Request) {
	actorID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	var req struct {
		Pattern string  `json:"pattern"`
		Reason  *string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if req.Pattern == "" {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "pattern is required")
		return
	}

	ban := &model.BannedEmail{
		Pattern: req.Pattern,
		Reason:  req.Reason,
	}

	if err := h.bans.BanEmail(r.Context(), actorID, "", ban); err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to create email ban")
		return
	}

	JSON(w, http.StatusCreated, map[string]interface{}{
		"email_ban": ban,
	})
}

// HandleDeleteEmailBan handles DELETE /api/v1/admin/bans/emails/{id}.
func (h *BanHandler) HandleDeleteEmailBan(w http.ResponseWriter, r *http.Request) {
	actorID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid ban ID")
		return
	}

	if err := h.bans.UnbanEmail(r.Context(), actorID, "", id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			ErrorResponse(w, http.StatusNotFound, "not_found", "email ban not found")
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to delete email ban")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleListIPBans handles GET /api/v1/admin/bans/ips.
func (h *BanHandler) HandleListIPBans(w http.ResponseWriter, r *http.Request) {
	bans, err := h.bans.ListIPBans(r.Context())
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list IP bans")
		return
	}
	if bans == nil {
		bans = []model.BannedIP{}
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"ip_bans": bans,
	})
}

// HandleCreateIPBan handles POST /api/v1/admin/bans/ips.
func (h *BanHandler) HandleCreateIPBan(w http.ResponseWriter, r *http.Request) {
	actorID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	var req struct {
		IPRange string  `json:"ip_range"`
		Reason  *string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if req.IPRange == "" {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "ip_range is required")
		return
	}

	ban := &model.BannedIP{
		IPRange: req.IPRange,
		Reason:  req.Reason,
	}

	if err := h.bans.BanIP(r.Context(), actorID, "", ban); err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to create IP ban")
		return
	}

	JSON(w, http.StatusCreated, map[string]interface{}{
		"ip_ban": ban,
	})
}

// HandleDeleteIPBan handles DELETE /api/v1/admin/bans/ips/{id}.
func (h *BanHandler) HandleDeleteIPBan(w http.ResponseWriter, r *http.Request) {
	actorID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid ban ID")
		return
	}

	if err := h.bans.UnbanIP(r.Context(), actorID, "", id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			ErrorResponse(w, http.StatusNotFound, "not_found", "IP ban not found")
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to delete IP ban")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
