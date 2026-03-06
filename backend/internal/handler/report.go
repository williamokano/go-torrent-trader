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

// ReportHandler handles report HTTP endpoints.
type ReportHandler struct {
	reports *service.ReportService
}

// NewReportHandler creates a new ReportHandler.
func NewReportHandler(reports *service.ReportService) *ReportHandler {
	return &ReportHandler{reports: reports}
}

// HandleCreate handles POST /api/v1/reports.
func (h *ReportHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	var req service.CreateReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	report, err := h.reports.Create(r.Context(), userID, req)
	if err != nil {
		handleReportError(w, err)
		return
	}

	JSON(w, http.StatusCreated, map[string]interface{}{
		"report": reportResponse(report),
	})
}

// HandleList handles GET /api/v1/reports (admin only).
func (h *ReportHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	opts := repository.ListReportsOptions{}

	if status := r.URL.Query().Get("status"); status != "" {
		opts.Status = &status
	}

	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		opts.Page, _ = strconv.Atoi(pageStr)
	}

	if ppStr := r.URL.Query().Get("per_page"); ppStr != "" {
		opts.PerPage, _ = strconv.Atoi(ppStr)
	}

	reports, total, err := h.reports.List(r.Context(), opts)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list reports")
		return
	}

	items := make([]map[string]interface{}, len(reports))
	for i := range reports {
		items[i] = reportResponse(&reports[i])
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"reports":  items,
		"total":    total,
		"page":     opts.Page,
		"per_page": opts.PerPage,
	})
}

// HandleResolve handles PUT /api/v1/reports/{id}/resolve (admin only).
func (h *ReportHandler) HandleResolve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid report ID")
		return
	}

	if err := h.reports.Resolve(r.Context(), id, userID); err != nil {
		handleReportError(w, err)
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "report resolved",
	})
}

func handleReportError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidReport):
		ErrorResponse(w, http.StatusBadRequest, "bad_request", err.Error())
	case errors.Is(err, service.ErrDuplicateReport):
		ErrorResponse(w, http.StatusConflict, "duplicate_report", err.Error())
	case errors.Is(err, service.ErrReportNotFound):
		ErrorResponse(w, http.StatusNotFound, "not_found", "report not found")
	default:
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "an unexpected error occurred")
	}
}

func reportResponse(r *model.Report) map[string]interface{} {
	resp := map[string]interface{}{
		"id":          r.ID,
		"reporter_id": r.ReporterID,
		"reason":      r.Reason,
		"resolved":    r.Resolved,
		"created_at":  r.CreatedAt,
	}
	if r.TorrentID != nil {
		resp["torrent_id"] = *r.TorrentID
	}
	if r.ResolvedBy != nil {
		resp["resolved_by"] = *r.ResolvedBy
	}
	if r.ResolvedAt != nil {
		resp["resolved_at"] = *r.ResolvedAt
	}
	return resp
}
