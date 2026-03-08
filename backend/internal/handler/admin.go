package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// AdminHandler handles admin HTTP endpoints.
type AdminHandler struct {
	admin *service.AdminService
}

// NewAdminHandler creates a new AdminHandler.
func NewAdminHandler(admin *service.AdminService) *AdminHandler {
	return &AdminHandler{admin: admin}
}

// HandleListUsers handles GET /api/v1/admin/users.
func (h *AdminHandler) HandleListUsers(w http.ResponseWriter, r *http.Request) {
	opts := repository.ListUsersOptions{}

	if search := r.URL.Query().Get("search"); search != "" {
		opts.Search = search
	}
	if gidStr := r.URL.Query().Get("group_id"); gidStr != "" {
		gid, err := strconv.ParseInt(gidStr, 10, 64)
		if err == nil {
			opts.GroupID = &gid
		}
	}
	if enabledStr := r.URL.Query().Get("enabled"); enabledStr != "" {
		enabled := enabledStr == "true"
		opts.Enabled = &enabled
	}
	if sortBy := r.URL.Query().Get("sort_by"); sortBy != "" {
		opts.SortBy = sortBy
	}
	if sortOrder := r.URL.Query().Get("sort_order"); sortOrder != "" {
		opts.SortOrder = sortOrder
	}
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		opts.Page, _ = strconv.Atoi(pageStr)
	}
	if ppStr := r.URL.Query().Get("per_page"); ppStr != "" {
		opts.PerPage, _ = strconv.Atoi(ppStr)
	}

	users, total, err := h.admin.ListUsers(r.Context(), opts)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list users")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"users":    users,
		"total":    total,
		"page":     opts.Page,
		"per_page": opts.PerPage,
	})
}

// HandleUpdateUser handles PUT /api/v1/admin/users/{id}.
func (h *AdminHandler) HandleUpdateUser(w http.ResponseWriter, r *http.Request) {
	actorID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid user ID")
		return
	}

	var req service.AdminUpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	user, err := h.admin.UpdateUser(r.Context(), actorID, id, req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAdminUserNotFound):
			ErrorResponse(w, http.StatusNotFound, "not_found", "user not found")
		case errors.Is(err, service.ErrAdminGroupNotFound):
			ErrorResponse(w, http.StatusBadRequest, "bad_request", err.Error())
		default:
			ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to update user")
		}
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"user": user,
	})
}

// HandleListGroups handles GET /api/v1/admin/groups.
func (h *AdminHandler) HandleListGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := h.admin.ListGroups(r.Context())
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list groups")
		return
	}

	items := make([]map[string]interface{}, len(groups))
	for i, g := range groups {
		items[i] = map[string]interface{}{
			"id":           g.ID,
			"name":         g.Name,
			"slug":         g.Slug,
			"level":        g.Level,
			"color":        g.Color,
			"can_upload":   g.CanUpload,
			"can_download": g.CanDownload,
			"can_invite":   g.CanInvite,
			"can_comment":  g.CanComment,
			"can_forum":    g.CanForum,
			"is_admin":     g.IsAdmin,
			"is_moderator": g.IsModerator,
			"is_immune":    g.IsImmune,
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"groups": items,
	})
}
