package handler

import (
	"net/http"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// MetadataAuditHandler serves the "torrents missing required metadata" report.
type MetadataAuditHandler struct {
	audit *service.MetadataAuditService
}

// NewMetadataAuditHandler creates a MetadataAuditHandler.
func NewMetadataAuditHandler(audit *service.MetadataAuditService) *MetadataAuditHandler {
	return &MetadataAuditHandler{audit: audit}
}

// HandleMetadataIssues handles GET /api/v1/torrents/metadata-issues.
// Any authenticated user sees their own uploads; only admins may request the
// site-wide report via ?scope=all.
func (h *MetadataAuditHandler) HandleMetadataIssues(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}
	perms := middleware.PermissionsFromContext(r.Context())

	scopeAll := r.URL.Query().Get("scope") == "all"
	if scopeAll && !perms.IsAdmin {
		ErrorResponse(w, http.StatusForbidden, "forbidden", "only admins can view all uploaders' torrents")
		return
	}

	var uploaderID *int64
	scope := "all"
	if !scopeAll {
		uid := userID
		uploaderID = &uid
		scope = "mine"
	}

	issues, err := h.audit.Issues(r.Context(), uploaderID)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to build the metadata issues report")
		return
	}

	JSON(w, http.StatusOK, map[string]any{
		"issues": issues,
		"scope":  scope,
	})
}
