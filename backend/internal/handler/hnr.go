package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// HnRHandler handles admin endpoints for hit-and-run tracking configuration
// and the daemon's run log.
type HnRHandler struct {
	svc *service.HnRService
}

// NewHnRHandler creates a new HnRHandler.
func NewHnRHandler(svc *service.HnRService) *HnRHandler {
	return &HnRHandler{svc: svc}
}

func hnrErrorStatus(err error) (int, bool) {
	switch {
	case errors.Is(err, service.ErrHnRGroupNotFound), errors.Is(err, service.ErrHnRRuleNotFound),
		errors.Is(err, service.ErrHnRStageNotFound):
		return http.StatusNotFound, true
	case errors.Is(err, service.ErrHnRInvalidStage):
		return http.StatusBadRequest, true
	case errors.Is(err, service.ErrHnRStaffGroup):
		return http.StatusConflict, true
	case errors.Is(err, service.ErrHnRInvalidThreshold):
		return http.StatusBadRequest, true
	case errors.Is(err, service.ErrHnRDaemonUnavailable):
		return http.StatusServiceUnavailable, true
	case errors.Is(err, repository.ErrHnRRecordNotClearable):
		return http.StatusNotFound, true
	case errors.Is(err, repository.ErrInsufficientBonusPoints):
		// Matches BonusStoreHandler's mapping of the same sentinel for a
		// store purchase — one convention across every "spend points" path.
		return http.StatusConflict, true
	default:
		return 0, false
	}
}

// HandleListRules handles GET /api/v1/admin/hnr/rules.
func (h *HnRHandler) HandleListRules(w http.ResponseWriter, r *http.Request) {
	rules, err := h.svc.ListRules(r.Context())
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list hit-and-run rules")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{"rules": rules})
}

// HandleUpsertRule handles PUT /api/v1/admin/hnr/rules/{groupId}.
func (h *HnRHandler) HandleUpsertRule(w http.ResponseWriter, r *http.Request) {
	groupID, err := strconv.ParseInt(chi.URLParam(r, "groupId"), 10, 64)
	if err != nil || groupID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid group ID")
		return
	}
	var in service.HnRRuleInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	rule, err := h.svc.UpsertRule(r.Context(), groupID, in)
	if err != nil {
		if status, ok := hnrErrorStatus(err); ok {
			ErrorResponse(w, status, "bad_request", err.Error())
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to save hit-and-run rule")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"rule": map[string]interface{}{
			"group_id":               rule.GroupID,
			"required_seed_hours":    rule.RequiredSeedHours,
			"required_ratio":         rule.RequiredRatio,
			"inactivity_grace_hours": rule.InactivityGraceHours,
			"max_days_to_satisfy":    rule.MaxDaysToSatisfy,
		},
	})
}

// HandleDeleteRule handles DELETE /api/v1/admin/hnr/rules/{groupId}.
func (h *HnRHandler) HandleDeleteRule(w http.ResponseWriter, r *http.Request) {
	groupID, err := strconv.ParseInt(chi.URLParam(r, "groupId"), 10, 64)
	if err != nil || groupID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid group ID")
		return
	}
	if err := h.svc.DeleteRule(r.Context(), groupID); err != nil {
		if status, ok := hnrErrorStatus(err); ok {
			ErrorResponse(w, status, "bad_request", err.Error())
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to delete hit-and-run rule")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{"deleted": true})
}

// HandleListStages handles GET /api/v1/admin/hnr/stages.
func (h *HnRHandler) HandleListStages(w http.ResponseWriter, r *http.Request) {
	stages, err := h.svc.ListStages(r.Context())
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list hit-and-run penalty stages")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{"stages": stages})
}

// HandleUpsertStage handles PUT /api/v1/admin/hnr/stages/{stage}.
func (h *HnRHandler) HandleUpsertStage(w http.ResponseWriter, r *http.Request) {
	stage, err := strconv.Atoi(chi.URLParam(r, "stage"))
	if err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid stage")
		return
	}
	var in service.HnRStageInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	row, err := h.svc.UpsertStage(r.Context(), stage, in)
	if err != nil {
		if status, ok := hnrErrorStatus(err); ok {
			ErrorResponse(w, status, "bad_request", err.Error())
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to save hit-and-run penalty stage")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{"stage": row})
}

// HandleDeleteStage handles DELETE /api/v1/admin/hnr/stages/{stage}.
func (h *HnRHandler) HandleDeleteStage(w http.ResponseWriter, r *http.Request) {
	stage, err := strconv.Atoi(chi.URLParam(r, "stage"))
	if err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid stage")
		return
	}
	if err := h.svc.DeleteStage(r.Context(), stage); err != nil {
		if status, ok := hnrErrorStatus(err); ok {
			ErrorResponse(w, status, "bad_request", err.Error())
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to delete hit-and-run penalty stage")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{"deleted": true})
}

// HandleRunNow handles POST /api/v1/admin/hnr/run — a manual trigger that
// forces an evaluation sweep immediately, going through the same two-stage
// advisory lock as the scheduled run, so it is always safe to press even
// while a scheduled run happens to be in flight.
func (h *HnRHandler) HandleRunNow(w http.ResponseWriter, r *http.Request) {
	actorID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}
	summary, err := h.svc.RunDaemon(r.Context(), model.HnRRunTriggerManual, &actorID)
	if err != nil {
		if status, ok := hnrErrorStatus(err); ok {
			ErrorResponse(w, status, "bad_request", err.Error())
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "hit-and-run run failed")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"skipped":   summary.Skipped,
		"run_id":    summary.RunID,
		"scanned":   summary.Counts.Scanned,
		"breached":  summary.Counts.Breached,
		"satisfied": summary.Counts.Satisfied,
	})
}

// HandleListForUser handles GET /api/v1/hnr — the authenticated member's own
// obligations, in the section order the member page needs (breach first,
// then monitored, then resolved — see hnrStateOrder), with DisplayStatus
// evaluated live so the page is never stale between daemon runs.
func (h *HnRHandler) HandleListForUser(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}
	views, err := h.svc.ListForUser(r.Context(), userID)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to load hit-and-run records")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{"records": views})
}

// HandleClearRecord handles POST /api/v1/hnr/{id}/clear — pays off one open
// obligation with bonus points. The price is always computed server-side
// (see service.ClearRecord); the request body carries nothing to trust.
func (h *HnRHandler) HandleClearRecord(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}
	recordID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || recordID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid record ID")
		return
	}
	result, err := h.svc.ClearRecord(r.Context(), userID, recordID)
	if err != nil {
		if status, ok := hnrErrorStatus(err); ok {
			ErrorResponse(w, status, "bad_request", err.Error())
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to clear the record")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"price":       result.Price,
		"new_balance": result.NewBalance,
	})
}

// HandleClearAll handles POST /api/v1/hnr/clear-all — clears every open
// obligation the member can currently afford, cheapest first. A partial
// clear is still a 200: the response says how many actually cleared and
// whether the balance ran out before the rest.
func (h *HnRHandler) HandleClearAll(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}
	result, err := h.svc.ClearAll(r.Context(), userID)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to clear records")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"cleared":                     result.Cleared,
		"total_spent":                 result.TotalSpent,
		"new_balance":                 result.NewBalance,
		"stopped_insufficient_points": result.StoppedInsufficientPoints,
	})
}

// HandleQuoteClear handles GET /api/v1/hnr/{id}/clear-price — what clearing
// this record would cost right now, without spending anything.
func (h *HnRHandler) HandleQuoteClear(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}
	recordID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || recordID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid record ID")
		return
	}
	price, err := h.svc.QuoteClear(r.Context(), userID, recordID)
	if err != nil {
		if status, ok := hnrErrorStatus(err); ok {
			ErrorResponse(w, status, "bad_request", err.Error())
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to price the clear")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{"price": price})
}

// HandleListRuns handles GET /api/v1/admin/hnr/runs — the daemon's run log,
// most recent first, for staff visibility into when it last ran and what it did.
func (h *HnRHandler) HandleListRuns(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	runs, err := h.svc.ListRuns(r.Context(), limit)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list hit-and-run runs")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{"runs": runs})
}

func hnrRecordResponse(rec model.HnRRecord) map[string]interface{} {
	return map[string]interface{}{
		"id":             rec.ID,
		"user_id":        rec.UserID,
		"username":       rec.Username,
		"torrent_id":     rec.TorrentID,
		"torrent_name":   rec.TorrentName,
		"torrent_size":   rec.TorrentSize,
		"torrent_exempt": rec.TorrentExempt,
		"state":          rec.State,
		"completed_at":   rec.CompletedAt,
		"last_seen_at":   rec.LastSeenAt,
		"seeded_seconds": rec.SeededSeconds,
		"uploaded":       rec.Uploaded,
		"breached_at":    rec.BreachedAt,
		"resolved_at":    rec.ResolvedAt,
	}
}

// HandleAdminListRecords handles GET /api/v1/admin/hnr/records — the staff
// records list, filterable by state, user id, or a username/torrent-name
// search, paginated.
func (h *HnRHandler) HandleAdminListRecords(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	opts := repository.HnRAdminListOptions{Search: q.Get("search")}
	if v := q.Get("state"); v != "" {
		opts.State = &v
	}
	if v := q.Get("user_id"); v != "" {
		if uid, err := strconv.ParseInt(v, 10, 64); err == nil && uid > 0 {
			opts.UserID = &uid
		}
	}
	if v := q.Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			opts.Page = n
		}
	}
	if v := q.Get("per_page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			opts.PerPage = n
		}
	}

	records, total, err := h.svc.AdminList(r.Context(), opts)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list hit-and-run records")
		return
	}
	items := make([]map[string]interface{}, len(records))
	for i, rec := range records {
		items[i] = hnrRecordResponse(rec)
	}
	JSON(w, http.StatusOK, map[string]interface{}{"records": items, "total": total})
}

// HandleStats handles GET /api/v1/admin/hnr/stats — the staff dashboard's
// whole-table breakdown and top-offenders leaderboard.
func (h *HnRHandler) HandleStats(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	stats, err := h.svc.Stats(r.Context(), limit)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to load hit-and-run stats")
		return
	}
	offenders := make([]map[string]interface{}, len(stats.TopOffenders))
	for i, o := range stats.TopOffenders {
		offenders[i] = map[string]interface{}{
			"user_id":       o.UserID,
			"username":      o.Username,
			"active_hnr":    o.ActiveHnR,
			"total_records": o.TotalRecords,
			"stage":         o.Stage,
		}
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"active_hnr":     stats.Aggregate.ActiveHnR,
		"monitored":      stats.Aggregate.Monitored,
		"satisfied":      stats.Aggregate.Satisfied,
		"cleared":        stats.Aggregate.Cleared,
		"waived":         stats.Aggregate.Waived,
		"breached_today": stats.Aggregate.BreachedToday,
		"top_offenders":  offenders,
	})
}
