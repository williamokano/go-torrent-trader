package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// InviteDistributionHandler handles admin endpoints for the auto invite
// distribution rules.
type InviteDistributionHandler struct {
	svc *service.InviteDistributionService
}

// NewInviteDistributionHandler creates a new InviteDistributionHandler.
func NewInviteDistributionHandler(svc *service.InviteDistributionService) *InviteDistributionHandler {
	return &InviteDistributionHandler{svc: svc}
}

func inviteDistributionErrorStatus(err error) (int, bool) {
	switch {
	case errors.Is(err, service.ErrInviteDistGroupNotFound), errors.Is(err, service.ErrInviteDistRuleNotFound):
		return http.StatusNotFound, true
	case errors.Is(err, service.ErrInviteDistStaffGroup):
		return http.StatusConflict, true
	case errors.Is(err, service.ErrInviteDistInvalidThreshold), errors.Is(err, service.ErrInviteDistInvalidRange):
		return http.StatusBadRequest, true
	default:
		return 0, false
	}
}

// HandleListRules handles GET /api/v1/admin/invite-distribution/rules.
func (h *InviteDistributionHandler) HandleListRules(w http.ResponseWriter, r *http.Request) {
	rules, err := h.svc.ListRules(r.Context())
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list invite distribution rules")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{"rules": rules})
}

// HandleUpsertRule handles PUT /api/v1/admin/invite-distribution/rules/{groupId}.
func (h *InviteDistributionHandler) HandleUpsertRule(w http.ResponseWriter, r *http.Request) {
	groupID, err := strconv.ParseInt(chi.URLParam(r, "groupId"), 10, 64)
	if err != nil || groupID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid group ID")
		return
	}
	var in service.InviteDistributionRuleInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	rule, err := h.svc.UpsertRule(r.Context(), groupID, in)
	if err != nil {
		if status, ok := inviteDistributionErrorStatus(err); ok {
			ErrorResponse(w, status, "bad_request", err.Error())
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to save invite distribution rule")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"rule": map[string]interface{}{
			"group_id":             rule.GroupID,
			"min_ratio":            rule.MinRatio,
			"min_downloaded_bytes": rule.MinDownloadedBytes,
			"max_downloaded_bytes": rule.MaxDownloadedBytes,
			"max_invites":          rule.MaxInvites,
		},
	})
}

// HandleDeleteRule handles DELETE /api/v1/admin/invite-distribution/rules/{groupId}.
func (h *InviteDistributionHandler) HandleDeleteRule(w http.ResponseWriter, r *http.Request) {
	groupID, err := strconv.ParseInt(chi.URLParam(r, "groupId"), 10, 64)
	if err != nil || groupID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid group ID")
		return
	}
	if err := h.svc.DeleteRule(r.Context(), groupID); err != nil {
		if status, ok := inviteDistributionErrorStatus(err); ok {
			ErrorResponse(w, status, "bad_request", err.Error())
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to delete invite distribution rule")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{"deleted": true})
}

// HandleRunNow handles POST /api/v1/admin/invite-distribution/run — a manual
// trigger that forces evaluation immediately, bypassing the interval so
// freshly-changed rules take effect now. It still respects the master enable
// flag.
func (h *InviteDistributionHandler) HandleRunNow(w http.ResponseWriter, r *http.Request) {
	summary, err := h.svc.Run(r.Context(), true)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "invite distribution run failed")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"skipped": summary.Skipped,
		"reason":  summary.Reason,
		"granted": summary.Granted,
	})
}
