package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	mw "github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// validFlagTypes is the set of known cheat flag types.
var validFlagTypes = map[string]bool{
	model.CheatFlagImpossibleUploadSpeed: true,
	model.CheatFlagUploadNoDownloaders:   true,
	model.CheatFlagLeftMismatch:          true,
}

// CheatFlagHandler handles admin endpoints for cheat flags.
type CheatFlagHandler struct {
	flags repository.CheatFlagRepository
}

// NewCheatFlagHandler creates a new CheatFlagHandler.
func NewCheatFlagHandler(flags repository.CheatFlagRepository) *CheatFlagHandler {
	return &CheatFlagHandler{flags: flags}
}

// HandleListCheatFlags handles GET /api/v1/admin/cheat-flags.
func (h *CheatFlagHandler) HandleListCheatFlags(w http.ResponseWriter, r *http.Request) {
	opts := repository.ListCheatFlagsOptions{}

	if userIDStr := r.URL.Query().Get("user_id"); userIDStr != "" {
		uid, err := strconv.ParseInt(userIDStr, 10, 64)
		if err == nil {
			opts.UserID = &uid
		}
	}
	if flagType := r.URL.Query().Get("flag_type"); flagType != "" {
		if !validFlagTypes[flagType] {
			ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid flag_type")
			return
		}
		opts.FlagType = &flagType
	}
	if dismissedStr := r.URL.Query().Get("dismissed"); dismissedStr != "" {
		dismissed := dismissedStr == "true"
		opts.Dismissed = &dismissed
	}

	// Normalize pagination defaults.
	opts.Page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	if opts.Page <= 0 {
		opts.Page = 1
	}
	opts.PerPage, _ = strconv.Atoi(r.URL.Query().Get("per_page"))
	if opts.PerPage <= 0 {
		opts.PerPage = 25
	}
	if opts.PerPage > 100 {
		opts.PerPage = 100
	}

	flags, total, err := h.flags.List(r.Context(), opts)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list cheat flags")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"cheat_flags": flags,
		"total":       total,
		"page":        opts.Page,
		"per_page":    opts.PerPage,
	})
}

// HandleDismissCheatFlag handles PUT /api/v1/admin/cheat-flags/{id}/dismiss.
func (h *CheatFlagHandler) HandleDismissCheatFlag(w http.ResponseWriter, r *http.Request) {
	userID, ok := mw.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid cheat flag ID")
		return
	}

	if err := h.flags.Dismiss(r.Context(), id, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			ErrorResponse(w, http.StatusNotFound, "not_found", "cheat flag not found or already dismissed")
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to dismiss cheat flag")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "cheat flag dismissed",
	})
}
