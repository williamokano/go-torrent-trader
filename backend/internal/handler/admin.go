package handler

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// AdminHandler handles admin HTTP endpoints.
type AdminHandler struct {
	admin *service.AdminService
	// feeds lets a class losing can_feed end the streams its members already
	// have open. May be nil, in which case a group edit simply does not
	// disconnect anyone.
	feeds *AnnounceHub
}

// NewAdminHandler creates a new AdminHandler.
func NewAdminHandler(admin *service.AdminService, feeds *AnnounceHub) *AdminHandler {
	return &AdminHandler{admin: admin, feeds: feeds}
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
		case errors.Is(err, service.ErrAdminInvalidUsername):
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

// HandleListUserEditHistory handles GET /api/v1/admin/users/{id}/edit-history.
func (h *AdminHandler) HandleListUserEditHistory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid user ID")
		return
	}

	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			limit = min(n, 200)
		}
	}
	offset := 0
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if n, err := strconv.Atoi(offsetStr); err == nil && n >= 0 {
			offset = n
		}
	}

	entries, total, err := h.admin.ListUserEditHistory(r.Context(), id, limit, offset)
	if err != nil {
		if errors.Is(err, service.ErrAdminUserNotFound) {
			ErrorResponse(w, http.StatusNotFound, "not_found", "user not found")
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list edit history")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"entries": entries,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
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
	for i := range groups {
		items[i] = groupToMap(&groups[i])
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"groups": items,
	})
}

// groupToMap renders a group as the JSON shape the admin UI consumes.
func groupToMap(g *model.Group) map[string]interface{} {
	return map[string]interface{}{
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
		"can_feed":     g.CanFeed,
	}
}

// groupWriteErrorStatus maps a group write error to an HTTP status. It returns
// ok=false for anything that isn't a recognized value error, so the caller can
// treat it as an internal failure without leaking the message.
func groupWriteErrorStatus(err error) (int, bool) {
	switch {
	case errors.Is(err, service.ErrGroupNotFound):
		return http.StatusNotFound, true
	case errors.Is(err, service.ErrGroupProtected):
		return http.StatusForbidden, true
	case errors.Is(err, service.ErrGroupHasMembers),
		errors.Is(err, service.ErrGroupNameTaken),
		errors.Is(err, service.ErrGroupSlugTaken):
		return http.StatusConflict, true
	case errors.Is(err, service.ErrGroupNameRequired),
		errors.Is(err, service.ErrGroupInvalidSlug),
		errors.Is(err, service.ErrGroupInvalidColor),
		errors.Is(err, service.ErrGroupInvalidLevel):
		return http.StatusBadRequest, true
	default:
		return 0, false
	}
}

// HandleCreateGroup handles POST /api/v1/admin/groups.
func (h *AdminHandler) HandleCreateGroup(w http.ResponseWriter, r *http.Request) {
	var req service.GroupWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	group, err := h.admin.CreateGroup(r.Context(), req)
	if err != nil {
		if status, ok := groupWriteErrorStatus(err); ok {
			ErrorResponse(w, status, "bad_request", err.Error())
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to create group")
		return
	}

	JSON(w, http.StatusCreated, map[string]interface{}{"group": groupToMap(group)})
}

// HandleUpdateGroup handles PUT /api/v1/admin/groups/{id}.
func (h *AdminHandler) HandleUpdateGroup(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid group ID")
		return
	}

	var req service.GroupWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	group, err := h.admin.UpdateGroup(r.Context(), id, req)
	if err != nil {
		if status, ok := groupWriteErrorStatus(err); ok {
			ErrorResponse(w, status, "bad_request", err.Error())
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to update group")
		return
	}

	// A class can have just lost the feeds, and the gate only runs at connect —
	// so its members would otherwise keep watching until they closed the tab.
	// Everyone is dropped rather than just that class's members: the hub knows
	// user ids, not groups, and a reconnect costs seconds and re-resolves.
	if h.feeds != nil {
		if dropped := h.feeds.DisconnectAll(); dropped > 0 {
			slog.Info("group permissions changed, reconnecting live feed watchers",
				"group_id", id, "streams", dropped)
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{"group": groupToMap(group)})
}

// HandleDeleteGroup handles DELETE /api/v1/admin/groups/{id}.
func (h *AdminHandler) HandleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid group ID")
		return
	}

	if err := h.admin.DeleteGroup(r.Context(), id); err != nil {
		if status, ok := groupWriteErrorStatus(err); ok {
			ErrorResponse(w, status, "bad_request", err.Error())
			return
		}
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to delete group")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{"deleted": true})
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
	actorID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	noteID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || noteID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid note ID")
		return
	}

	perms := middleware.PermissionsFromContext(r.Context())
	if err := h.admin.DeleteModNote(r.Context(), noteID, actorID, perms); err != nil {
		switch {
		case errors.Is(err, service.ErrModNoteNotFound):
			ErrorResponse(w, http.StatusNotFound, "not_found", "mod note not found")
		case errors.Is(err, service.ErrModNoteDeleteForbidden):
			ErrorResponse(w, http.StatusForbidden, "forbidden", "you can only delete your own notes")
		default:
			ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to delete mod note")
		}
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "mod note deleted",
	})
}

// HandleQuickBan handles POST /api/v1/admin/users/{id}/ban.
func (h *AdminHandler) HandleQuickBan(w http.ResponseWriter, r *http.Request) {
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

	var req service.QuickBanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	result, err := h.admin.QuickBanUser(r.Context(), actorID, id, req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAdminUserNotFound):
			ErrorResponse(w, http.StatusNotFound, "not_found", "user not found")
		case errors.Is(err, service.ErrAdminInsufficientLevel):
			ErrorResponse(w, http.StatusForbidden, "forbidden", "insufficient permissions to ban this user")
		case errors.Is(err, service.ErrAdminBanReasonRequired):
			ErrorResponse(w, http.StatusBadRequest, "bad_request", "ban reason is required")
		case errors.Is(err, service.ErrCannotBanSelf):
			ErrorResponse(w, http.StatusBadRequest, "bad_request", "cannot ban yourself")
		case errors.Is(err, service.ErrInvalidBanDuration):
			ErrorResponse(w, http.StatusBadRequest, "bad_request", "duration must be positive")
		case errors.Is(err, service.ErrCommonEmailProvider):
			ErrorResponse(w, http.StatusBadRequest, "bad_request", err.Error())
		default:
			ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to ban user")
		}
		return
	}

	JSON(w, http.StatusOK, result)
}

// HandleListTorrents handles GET /api/v1/admin/torrents.
func (h *AdminHandler) HandleListTorrents(w http.ResponseWriter, r *http.Request) {
	opts := repository.ListTorrentsOptions{}

	if search := r.URL.Query().Get("search"); search != "" {
		// A 40-character hex string is not a torrent name. Staff arrive here with
		// the identifier a takedown notice or a misbehaving swarm gave them, and
		// pasting it into the search box should find the torrent rather than
		// return nothing — so the search term is routed by what it is, not by
		// which field the caller remembered to use.
		if hash, ok := parseInfoHashHex(search); ok {
			opts.InfoHash = hash
		} else {
			opts.Search = search
		}
	}
	// Two escape hatches from that guess, one in each direction. Without `name`, a
	// torrent whose title happens to be 40 hex characters — a checksum, a git SHA,
	// an obfuscated release tag — would be permanently unfindable by name.
	if raw := r.URL.Query().Get("info_hash"); raw != "" {
		hash, ok := parseInfoHashHex(raw)
		if !ok {
			ErrorResponse(w, http.StatusBadRequest, "bad_request",
				"info_hash must be 40 hexadecimal characters")
			return
		}
		opts.InfoHash = hash
	}
	if name := r.URL.Query().Get("name"); name != "" {
		opts.Search = name
		opts.InfoHash = nil
	}
	if bannedStr := r.URL.Query().Get("banned"); bannedStr != "" {
		banned, err := strconv.ParseBool(bannedStr)
		if err != nil {
			ErrorResponse(w, http.StatusBadRequest, "bad_request", "banned must be true or false")
			return
		}
		opts.Banned = &banned
	}
	// Rejected rather than ignored. A silently dropped filter widens the result set,
	// which reads as "your search matched everything" — the opposite of the truth,
	// and the worst possible answer on a moderation screen.
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		page, err := strconv.Atoi(pageStr)
		if err != nil || page < 1 {
			ErrorResponse(w, http.StatusBadRequest, "bad_request", "page must be a positive number")
			return
		}
		opts.Page = page
	}
	if ppStr := r.URL.Query().Get("per_page"); ppStr != "" {
		perPage, err := strconv.Atoi(ppStr)
		if err != nil || perPage < 1 {
			ErrorResponse(w, http.StatusBadRequest, "bad_request", "per_page must be a positive number")
			return
		}
		opts.PerPage = perPage
	}
	if uploaderStr := r.URL.Query().Get("uploader_id"); uploaderStr != "" {
		uid, err := strconv.ParseInt(uploaderStr, 10, 64)
		if err != nil || uid < 1 {
			ErrorResponse(w, http.StatusBadRequest, "bad_request", "uploader_id must be a positive number")
			return
		}
		opts.UploaderID = &uid
	}

	// Defaulted and clamped here, not only inside the service: the service takes
	// opts by value, so its own clamping never reached the numbers echoed below —
	// a request with no params answered `page: 0, per_page: 0` while serving page 1
	// of 25, and an integrator computing ceil(total / per_page) divided by zero.
	if opts.Page < 1 {
		opts.Page = 1
	}
	if opts.PerPage < 1 {
		opts.PerPage = defaultAdminPerPage
	}
	if opts.PerPage > maxAdminPerPage {
		opts.PerPage = maxAdminPerPage
	}

	torrents, total, err := h.admin.ListTorrents(r.Context(), opts)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list torrents")
		return
	}

	items := make([]map[string]interface{}, len(torrents))
	for i, t := range torrents {
		items[i] = map[string]interface{}{
			"id":   t.ID,
			"name": t.Name,
			// Hex, and present on every row: a moderator who searched by hash needs
			// to confirm they have the right torrent, and one who is about to file
			// a report needs to copy it back out.
			"info_hash":   hex.EncodeToString(t.InfoHash),
			"size":        t.Size,
			"seeders":     t.Seeders,
			"leechers":    t.Leechers,
			"uploader_id": t.UploaderID,
			"uploader":    t.UploaderName,
			"banned":      t.Banned,
			"free":        t.Free,
			"silver":      t.Silver,
			"visible":     t.Visible,
			"created_at":  t.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"torrents": items,
		"total":    total,
		"page":     opts.Page,
		"per_page": opts.PerPage,
	})
}

// infoHashHexLength is the hex width of a 20-byte BitTorrent info hash.
const infoHashHexLength = 40

// Pagination bounds for the admin torrent listing. Kept in step with what
// AdminService.ListTorrents enforces and with what openapi.yaml documents, since
// the response echoes these back as a contract.
const (
	defaultAdminPerPage = 25
	maxAdminPerPage     = 100
)

// maxBulkBodyBytes bounds the bulk request body. The id-count cap is checked after
// decoding, so without this an arbitrarily large array would be unmarshalled into
// memory before being rejected.
const maxBulkBodyBytes = 64 << 10

// parseInfoHashHex decodes a hex info hash, reporting whether the input was one.
// Case-insensitive, since a hash pasted out of a takedown notice or a client's UI
// arrives in whichever case that tool chose.
func parseInfoHashHex(s string) ([]byte, bool) {
	s = strings.TrimSpace(s)
	if len(s) != infoHashHexLength {
		return nil, false
	}
	hash, err := hex.DecodeString(strings.ToLower(s))
	if err != nil {
		return nil, false
	}
	return hash, true
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
		switch {
		case errors.Is(err, service.ErrTorrentNotFound):
			ErrorResponse(w, http.StatusNotFound, "not_found", "torrent not found")
		case errors.Is(err, service.ErrForbidden):
			ErrorResponse(w, http.StatusForbidden, "forbidden", "insufficient permissions to delete this torrent")
		default:
			ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to delete torrent")
		}
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "torrent deleted",
	})
}

// bulkTorrentRequest is the body of POST /api/v1/admin/torrents/bulk.
type bulkTorrentRequest struct {
	Action string  `json:"action"`
	IDs    []int64 `json:"ids"`
}

// HandleBulkAction handles POST /api/v1/admin/torrents/bulk — ban, unban or delete
// several torrents in one request.
//
// Returns 200 with a per-torrent breakdown even when some of them failed. A bulk
// request over ids a moderator pasted will routinely include one that is already
// gone, and a 4xx for the whole batch would leave them to work out which. The
// status code answers "was the request valid"; the body answers "what happened".
func (h *TorrentAdminHandler) HandleBulkAction(w http.ResponseWriter, r *http.Request) {
	actorID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBulkBodyBytes)

	var req bulkTorrentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	results, err := h.torrentSvc.BulkModerate(r.Context(),
		service.BulkAction(req.Action), req.IDs, actorID, middleware.PermissionsFromContext(r.Context()))
	if err != nil {
		switch {
		// Only the request being unusable reaches here — an unknown action, an
		// empty list, or more ids than one request may carry.
		case errors.Is(err, service.ErrInvalidTorrent):
			ErrorResponse(w, http.StatusBadRequest, "bad_request", err.Error())
		case errors.Is(err, service.ErrForbidden):
			ErrorResponse(w, http.StatusForbidden, "forbidden", "staff access required")
		default:
			ErrorResponse(w, http.StatusInternalServerError, "internal_error", "bulk action failed")
		}
		return
	}

	succeeded := 0
	for _, res := range results {
		if res.Status == service.BulkStatusOK {
			succeeded++
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"action":    req.Action,
		"results":   results,
		"succeeded": succeeded,
		"failed":    len(results) - succeeded,
	})
}
