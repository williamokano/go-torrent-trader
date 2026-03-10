package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// RestrictionHandler handles admin restriction endpoints.
type RestrictionHandler struct {
	restrictionSvc *service.RestrictionService
}

// NewRestrictionHandler creates a new RestrictionHandler.
func NewRestrictionHandler(restrictionSvc *service.RestrictionService) *RestrictionHandler {
	return &RestrictionHandler{restrictionSvc: restrictionSvc}
}

type setRestrictionsRequest struct {
	CanDownload *bool   `json:"can_download"`
	CanUpload   *bool   `json:"can_upload"`
	CanChat     *bool   `json:"can_chat"`
	Reason      string  `json:"reason"`
	ExpiresAt   *string `json:"expires_at"`
}

// HandleSetRestrictions handles PUT /api/v1/admin/users/{id}/restrictions.
func (h *RestrictionHandler) HandleSetRestrictions(w http.ResponseWriter, r *http.Request) {
	actorID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	userID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || userID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid user ID")
		return
	}

	var req setRestrictionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
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
			ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid expires_at format (use RFC3339)")
			return
		}
		expiresAt = &t
	}

	// For each privilege, if explicitly set to false, apply a restriction.
	// If explicitly set to true, lift all active restrictions of that type.
	type restrictionAction struct {
		rType    string
		restrict bool
	}

	var actions []restrictionAction
	if req.CanDownload != nil {
		actions = append(actions, restrictionAction{"download", !*req.CanDownload})
	}
	if req.CanUpload != nil {
		actions = append(actions, restrictionAction{"upload", !*req.CanUpload})
	}
	if req.CanChat != nil {
		actions = append(actions, restrictionAction{"chat", !*req.CanChat})
	}

	for _, action := range actions {
		if action.restrict {
			if _, err := h.restrictionSvc.ApplyRestriction(r.Context(), userID, action.rType, req.Reason, expiresAt, &actorID); err != nil {
				ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to apply restriction: "+err.Error())
				return
			}
		} else {
			// Lift all active restrictions of this type.
			restrictions, err := h.restrictionSvc.ListByUser(r.Context(), userID)
			if err != nil {
				ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list restrictions")
				return
			}
			for _, restriction := range restrictions {
				if restriction.RestrictionType == action.rType && restriction.LiftedAt == nil {
					if err := h.restrictionSvc.LiftRestriction(r.Context(), restriction.ID, &actorID); err != nil {
						ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to lift restriction: "+err.Error())
						return
					}
				}
			}
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "restrictions updated",
	})
}

// HandleListRestrictions handles GET /api/v1/admin/users/{id}/restrictions.
func (h *RestrictionHandler) HandleListRestrictions(w http.ResponseWriter, r *http.Request) {
	userID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || userID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid user ID")
		return
	}

	restrictions, err := h.restrictionSvc.ListByUser(r.Context(), userID)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list restrictions")
		return
	}

	items := make([]map[string]interface{}, len(restrictions))
	for i, restriction := range restrictions {
		item := map[string]interface{}{
			"id":               restriction.ID,
			"user_id":          restriction.UserID,
			"restriction_type": restriction.RestrictionType,
			"reason":           restriction.Reason,
			"issued_by":        restriction.IssuedBy,
			"issued_by_username": restriction.IssuedByUsername,
			"expires_at":       restriction.ExpiresAt,
			"lifted_at":        restriction.LiftedAt,
			"lifted_by":        restriction.LiftedBy,
			"lifted_by_username": restriction.LiftedByUsername,
			"created_at":       restriction.CreatedAt.Format(time.RFC3339),
		}
		items[i] = item
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"restrictions": items,
	})
}

// HandleLiftRestriction handles DELETE /api/v1/admin/restrictions/{id}.
func (h *RestrictionHandler) HandleLiftRestriction(w http.ResponseWriter, r *http.Request) {
	actorID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	restrictionID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || restrictionID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid restriction ID")
		return
	}

	if err := h.restrictionSvc.LiftRestriction(r.Context(), restrictionID, &actorID); err != nil {
		switch {
		case errors.Is(err, service.ErrRestrictionNotFound):
			ErrorResponse(w, http.StatusNotFound, "not_found", "restriction not found")
		case errors.Is(err, service.ErrRestrictionAlreadyLifted):
			ErrorResponse(w, http.StatusBadRequest, "bad_request", "restriction already lifted")
		default:
			ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to lift restriction")
		}
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "restriction lifted",
	})
}
