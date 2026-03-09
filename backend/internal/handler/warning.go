package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// WarningHandler handles admin warning HTTP endpoints.
type WarningHandler struct {
	warnings *service.WarningService
}

// NewWarningHandler creates a new WarningHandler.
func NewWarningHandler(warnings *service.WarningService) *WarningHandler {
	return &WarningHandler{warnings: warnings}
}

// HandleIssueWarning handles POST /api/v1/admin/warnings.
func (h *WarningHandler) HandleIssueWarning(w http.ResponseWriter, r *http.Request) {
	actorID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	var req struct {
		UserID    int64   `json:"user_id"`
		Reason    string  `json:"reason"`
		ExpiresAt *string `json:"expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	if req.UserID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "user_id is required")
		return
	}
	if req.Reason == "" {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "reason is required")
		return
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid expires_at format, use RFC3339")
			return
		}
		expiresAt = &t
	}

	warning, err := h.warnings.IssueManualWarning(r.Context(), req.UserID, req.Reason, expiresAt, actorID)
	if err != nil {
		if errors.Is(err, service.ErrInvalidWarning) {
			ErrorResponse(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to issue warning")
		return
	}

	JSON(w, http.StatusCreated, map[string]interface{}{
		"warning": warning,
	})
}

// HandleListWarnings handles GET /api/v1/admin/warnings.
func (h *WarningHandler) HandleListWarnings(w http.ResponseWriter, r *http.Request) {
	opts := repository.ListWarningsOptions{}

	if userIDStr := r.URL.Query().Get("user_id"); userIDStr != "" {
		uid, err := strconv.ParseInt(userIDStr, 10, 64)
		if err == nil {
			opts.UserID = &uid
		}
	}
	if status := r.URL.Query().Get("status"); status != "" {
		opts.Status = &status
	}
	if search := r.URL.Query().Get("search"); search != "" {
		opts.Search = search
	}
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		opts.Page, _ = strconv.Atoi(pageStr)
	}
	if ppStr := r.URL.Query().Get("per_page"); ppStr != "" {
		opts.PerPage, _ = strconv.Atoi(ppStr)
	}

	warnings, total, err := h.warnings.ListWarnings(r.Context(), opts)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list warnings")
		return
	}

	if warnings == nil {
		warnings = []model.Warning{}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"warnings": warnings,
		"total":    total,
		"page":     opts.Page,
		"per_page": opts.PerPage,
	})
}

// HandleLiftWarning handles POST /api/v1/admin/warnings/{id}/lift.
func (h *WarningHandler) HandleLiftWarning(w http.ResponseWriter, r *http.Request) {
	actorID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid warning ID")
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	if err := h.warnings.LiftWarning(r.Context(), id, actorID, req.Reason); err != nil {
		if errors.Is(err, service.ErrWarningNotFound) {
			ErrorResponse(w, http.StatusNotFound, "not_found", "warning not found")
			return
		}
		if errors.Is(err, service.ErrInvalidWarning) {
			ErrorResponse(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to lift warning")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetUserWarnings handles GET /api/v1/users/{id}/warnings.
func (h *WarningHandler) HandleGetUserWarnings(resp http.ResponseWriter, r *http.Request) {
	actorID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(resp, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	userID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || userID <= 0 {
		ErrorResponse(resp, http.StatusBadRequest, "bad_request", "invalid user ID")
		return
	}

	perms := middleware.PermissionsFromContext(r.Context())
	isStaff := perms.IsStaff()
	isOwner := actorID == userID

	if !isOwner && !isStaff {
		ErrorResponse(resp, http.StatusForbidden, "forbidden", "access denied")
		return
	}

	var warnings []model.Warning
	if isStaff {
		warnings, err = h.warnings.GetAllWarnings(r.Context(), userID)
	} else {
		warnings, err = h.warnings.GetActiveWarnings(r.Context(), userID)
	}
	if err != nil {
		ErrorResponse(resp, http.StatusInternalServerError, "internal_error", "failed to get warnings")
		return
	}

	if warnings == nil {
		warnings = []model.Warning{}
	}

	JSON(resp, http.StatusOK, map[string]interface{}{
		"warnings": warnings,
	})
}
