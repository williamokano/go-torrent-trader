package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// ModerationHandler handles torrent submission-moderation HTTP endpoints (BE-8.22).
type ModerationHandler struct {
	torrentSvc *service.TorrentService
}

// NewModerationHandler creates a new ModerationHandler.
func NewModerationHandler(torrentSvc *service.TorrentService) *ModerationHandler {
	return &ModerationHandler{torrentSvc: torrentSvc}
}

// HandleQueue handles GET /api/v1/admin/moderation/torrents (staff only).
func (h *ModerationHandler) HandleQueue(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserIDFromContext(r.Context())

	opts := repository.ModerationQueueOptions{
		Status:   r.URL.Query().Get("status"),
		Assigned: repository.ModAssignedAll,
	}

	switch r.URL.Query().Get("assigned") {
	case string(repository.ModAssignedMine):
		opts.Assigned = repository.ModAssignedMine
		opts.ModeratorID = userID
	case string(repository.ModAssignedUnassigned):
		opts.Assigned = repository.ModAssignedUnassigned
	}

	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		opts.Page, _ = strconv.Atoi(pageStr)
	}
	if ppStr := r.URL.Query().Get("per_page"); ppStr != "" {
		opts.PerPage, _ = strconv.Atoi(ppStr)
	}

	torrents, total, err := h.torrentSvc.ListModerationQueue(r.Context(), opts)
	if err != nil {
		handleTorrentError(w, err)
		return
	}

	items := make([]map[string]interface{}, len(torrents))
	for i := range torrents {
		items[i] = torrentResponse(&torrents[i])
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"torrents": items,
		"total":    total,
		"page":     opts.Page,
		"per_page": opts.PerPage,
	})
}

// HandleApprove handles POST /api/v1/torrents/{id}/moderation/approve.
// Authorization is enforced in the service (staff, extended to self-approving
// Uploaders in BE-8.22c), so this endpoint is not staff-gated at the route.
func (h *ModerationHandler) HandleApprove(w http.ResponseWriter, r *http.Request) {
	h.act(w, r, func(id, userID int64) (*model.Torrent, error) {
		perms := middleware.PermissionsFromContext(r.Context())
		return h.torrentSvc.ApproveTorrent(r.Context(), id, userID, perms)
	})
}

// HandleReject handles POST /api/v1/admin/moderation/torrents/{id}/reject (staff).
func (h *ModerationHandler) HandleReject(w http.ResponseWriter, r *http.Request) {
	h.act(w, r, func(id, userID int64) (*model.Torrent, error) {
		perms := middleware.PermissionsFromContext(r.Context())
		return h.torrentSvc.RejectTorrent(r.Context(), id, userID, perms)
	})
}

// HandleClaim handles POST /api/v1/admin/moderation/torrents/{id}/claim (staff).
func (h *ModerationHandler) HandleClaim(w http.ResponseWriter, r *http.Request) {
	h.act(w, r, func(id, userID int64) (*model.Torrent, error) {
		return h.torrentSvc.ClaimModeration(r.Context(), id, userID)
	})
}

// HandleUnclaim handles POST /api/v1/admin/moderation/torrents/{id}/unclaim (staff).
func (h *ModerationHandler) HandleUnclaim(w http.ResponseWriter, r *http.Request) {
	h.act(w, r, func(id, _ int64) (*model.Torrent, error) {
		return h.torrentSvc.UnclaimModeration(r.Context(), id)
	})
}

// HandleListMessages handles GET /api/v1/torrents/{id}/moderation/messages.
// Readable by staff and the uploader (enforced in the service).
func (h *ModerationHandler) HandleListMessages(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}
	perms := middleware.PermissionsFromContext(r.Context())

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid torrent ID")
		return
	}

	msgs, err := h.torrentSvc.ListModerationMessages(r.Context(), id, userID, perms)
	if err != nil {
		handleTorrentError(w, err)
		return
	}

	items := make([]map[string]interface{}, len(msgs))
	for i := range msgs {
		items[i] = moderationMessageResponse(&msgs[i])
	}
	JSON(w, http.StatusOK, map[string]interface{}{"messages": items})
}

// HandlePostMessage handles POST /api/v1/torrents/{id}/moderation/messages.
func (h *ModerationHandler) HandlePostMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}
	perms := middleware.PermissionsFromContext(r.Context())

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid torrent ID")
		return
	}

	var body struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	msg, err := h.torrentSvc.PostModerationMessage(r.Context(), id, userID, perms, body.Body)
	if err != nil {
		handleTorrentError(w, err)
		return
	}
	JSON(w, http.StatusCreated, map[string]interface{}{"message": moderationMessageResponse(msg)})
}

func moderationMessageResponse(m *model.TorrentModerationMessage) map[string]interface{} {
	return map[string]interface{}{
		"id":              m.ID,
		"torrent_id":      m.TorrentID,
		"author_id":       m.AuthorID,
		"author_username": m.AuthorUsername,
		"body":            m.Body,
		"created_at":      m.CreatedAt,
	}
}

// act is the shared shell for the single-torrent moderation actions: it extracts
// the authenticated user and torrent id, runs the action, and renders the updated
// torrent (or maps the error).
func (h *ModerationHandler) act(w http.ResponseWriter, r *http.Request, fn func(id, userID int64) (*model.Torrent, error)) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid torrent ID")
		return
	}

	torrent, err := fn(id, userID)
	if err != nil {
		handleTorrentError(w, err)
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"torrent": torrentResponse(torrent),
	})
}
