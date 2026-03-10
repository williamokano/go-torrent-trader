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

// HandleGetUserDetail handles GET /api/v1/admin/users/{id}.
func (h *AdminHandler) HandleGetUserDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid user ID")
		return
	}

	detail, err := h.admin.GetUserDetail(r.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrAdminUserNotFound) {
			ErrorResponse(w, http.StatusNotFound, "not_found", "user not found")
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to get user detail")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"user": detail,
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

// HandleResetPassword handles PUT /api/v1/admin/users/{id}/reset-password.
func (h *AdminHandler) HandleResetPassword(w http.ResponseWriter, r *http.Request) {
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

	var req struct {
		NewPassword *string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Allow empty body — treat as auto-generate
		req.NewPassword = nil
	}

	password := ""
	if req.NewPassword != nil {
		password = *req.NewPassword
	}

	newPass, err := h.admin.ResetPassword(r.Context(), actorID, id, password)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAdminUserNotFound):
			ErrorResponse(w, http.StatusNotFound, "not_found", "user not found")
		case errors.Is(err, service.ErrAdminInsufficientLevel):
			ErrorResponse(w, http.StatusForbidden, "forbidden", "insufficient permissions to reset this user's password")
		case errors.Is(err, service.ErrAdminPasswordTooShort):
			ErrorResponse(w, http.StatusBadRequest, "bad_request", "password must be at least 8 characters")
		default:
			ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to reset password")
		}
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"new_password": newPass,
	})
}

// HandleResetPasskey handles PUT /api/v1/admin/users/{id}/reset-passkey.
func (h *AdminHandler) HandleResetPasskey(w http.ResponseWriter, r *http.Request) {
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

	newPasskey, err := h.admin.ResetPasskey(r.Context(), actorID, id)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAdminUserNotFound):
			ErrorResponse(w, http.StatusNotFound, "not_found", "user not found")
		case errors.Is(err, service.ErrAdminInsufficientLevel):
			ErrorResponse(w, http.StatusForbidden, "forbidden", "insufficient permissions to reset this user's passkey")
		default:
			ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to reset passkey")
		}
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"new_passkey": newPasskey,
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

// HandleCreateModNote handles POST /api/v1/admin/users/{id}/notes.
func (h *AdminHandler) HandleCreateModNote(w http.ResponseWriter, r *http.Request) {
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

	var req struct {
		Note string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	note, err := h.admin.CreateModNote(r.Context(), userID, actorID, req.Note)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAdminUserNotFound):
			ErrorResponse(w, http.StatusNotFound, "not_found", "user not found")
		case errors.Is(err, service.ErrInvalidModNote):
			ErrorResponse(w, http.StatusBadRequest, "bad_request", err.Error())
		default:
			ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to create mod note")
		}
		return
	}

	JSON(w, http.StatusCreated, map[string]interface{}{
		"note": note,
	})
}

// HandleDeleteModNote handles DELETE /api/v1/admin/notes/{id}.
func (h *AdminHandler) HandleDeleteModNote(w http.ResponseWriter, r *http.Request) {
	noteID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || noteID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid note ID")
		return
	}

	if err := h.admin.DeleteModNote(r.Context(), noteID); err != nil {
		if errors.Is(err, service.ErrModNoteNotFound) {
			ErrorResponse(w, http.StatusNotFound, "not_found", "mod note not found")
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to delete mod note")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "mod note deleted",
	})
}

// HandleListTorrents handles GET /api/v1/admin/torrents.
func (h *AdminHandler) HandleListTorrents(w http.ResponseWriter, r *http.Request) {
	opts := repository.ListTorrentsOptions{}

	if search := r.URL.Query().Get("search"); search != "" {
		opts.Search = search
	}
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		opts.Page, _ = strconv.Atoi(pageStr)
	}
	if ppStr := r.URL.Query().Get("per_page"); ppStr != "" {
		opts.PerPage, _ = strconv.Atoi(ppStr)
	}
	if uploaderStr := r.URL.Query().Get("uploader_id"); uploaderStr != "" {
		uid, err := strconv.ParseInt(uploaderStr, 10, 64)
		if err == nil {
			opts.UploaderID = &uid
		}
	}

	torrents, total, err := h.admin.ListTorrents(r.Context(), opts)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list torrents")
		return
	}

	items := make([]map[string]interface{}, len(torrents))
	for i, t := range torrents {
		items[i] = map[string]interface{}{
			"id":         t.ID,
			"name":       t.Name,
			"size":       t.Size,
			"seeders":    t.Seeders,
			"leechers":   t.Leechers,
			"uploader_id": t.UploaderID,
			"uploader":   t.UploaderName,
			"banned":     t.Banned,
			"created_at": t.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"torrents": items,
		"total":    total,
		"page":     opts.Page,
		"per_page": opts.PerPage,
	})
}

// TorrentAdminHandler handles admin torrent operations that need TorrentService.
type TorrentAdminHandler struct {
	torrentSvc *service.TorrentService
}

// NewTorrentAdminHandler creates a new TorrentAdminHandler.
func NewTorrentAdminHandler(torrentSvc *service.TorrentService) *TorrentAdminHandler {
	return &TorrentAdminHandler{torrentSvc: torrentSvc}
}

// HandleDeleteTorrent handles DELETE /api/v1/admin/torrents/{id}.
func (h *TorrentAdminHandler) HandleDeleteTorrent(w http.ResponseWriter, r *http.Request) {
	actorID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	torrentID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || torrentID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid torrent ID")
		return
	}

	adminPerms := middleware.PermissionsFromContext(r.Context())
	if err := h.torrentSvc.DeleteTorrent(r.Context(), torrentID, actorID, adminPerms); err != nil {
		if errors.Is(err, service.ErrTorrentNotFound) {
			ErrorResponse(w, http.StatusNotFound, "not_found", "torrent not found")
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to delete torrent")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "torrent deleted",
	})
}
