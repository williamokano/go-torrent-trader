package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// PromotionHandler handles admin endpoints for the auto class promotion ladder.
type PromotionHandler struct {
	svc *service.PromotionService
}

// NewPromotionHandler creates a new PromotionHandler.
func NewPromotionHandler(svc *service.PromotionService) *PromotionHandler {
	return &PromotionHandler{svc: svc}
}

func promotionErrorStatus(err error) (int, bool) {
	switch {
	case errors.Is(err, service.ErrPromotionGroupNotFound), errors.Is(err, service.ErrPromotionRuleNotFound):
		return http.StatusNotFound, true
	case errors.Is(err, service.ErrPromotionStaffGroup):
		return http.StatusConflict, true
	case errors.Is(err, service.ErrPromotionInvalidThreshold):
		return http.StatusBadRequest, true
	default:
		return 0, false
	}
}

// HandleListRules handles GET /api/v1/admin/promotion/rules.
func (h *PromotionHandler) HandleListRules(w http.ResponseWriter, r *http.Request) {
	rules, err := h.svc.ListRules(r.Context())
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list promotion rules")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{"rules": rules})
}

// HandleUpsertRule handles PUT /api/v1/admin/promotion/rules/{groupId}.
func (h *PromotionHandler) HandleUpsertRule(w http.ResponseWriter, r *http.Request) {
	groupID, err := strconv.ParseInt(chi.URLParam(r, "groupId"), 10, 64)
	if err != nil || groupID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid group ID")
		return
	}
	var in service.PromotionRuleInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	rule, err := h.svc.UpsertRule(r.Context(), groupID, in)
	if err != nil {
		if status, ok := promotionErrorStatus(err); ok {
			ErrorResponse(w, status, "bad_request", err.Error())
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to save promotion rule")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"rule": map[string]interface{}{
			"group_id":       rule.GroupID,
			"min_ratio":      rule.MinRatio,
			"min_uploaded":   rule.MinUploaded,
			"min_age_days":   rule.MinAgeDays,
			"min_torrents":   rule.MinTorrents,
			"min_seed_hours": rule.MinSeedHours,
		},
	})
}

// HandleDeleteRule handles DELETE /api/v1/admin/promotion/rules/{groupId}.
func (h *PromotionHandler) HandleDeleteRule(w http.ResponseWriter, r *http.Request) {
	groupID, err := strconv.ParseInt(chi.URLParam(r, "groupId"), 10, 64)
	if err != nil || groupID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid group ID")
		return
	}
	if err := h.svc.DeleteRule(r.Context(), groupID); err != nil {
		if status, ok := promotionErrorStatus(err); ok {
			ErrorResponse(w, status, "bad_request", err.Error())
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to delete promotion rule")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{"deleted": true})
}

// HandleRunNow handles POST /api/v1/admin/promotion/run — a manual trigger that
// still respects the enable flag and interval (the response reports if skipped).
func (h *PromotionHandler) HandleRunNow(w http.ResponseWriter, r *http.Request) {
	summary, err := h.svc.Run(r.Context())
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "promotion run failed")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"skipped":  summary.Skipped,
		"reason":   summary.Reason,
		"promoted": summary.Promoted,
		"demoted":  summary.Demoted,
	})
}
