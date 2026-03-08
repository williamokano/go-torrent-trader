package handler

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

const maxTorrentFileSize = 10 << 20 // 10 MB

// TorrentHandler handles torrent HTTP endpoints.
type TorrentHandler struct {
	torrentSvc *service.TorrentService
	peerRepo   repository.PeerRepository
	userRepo   repository.UserRepository
}

// NewTorrentHandler creates a new TorrentHandler.
func NewTorrentHandler(torrentSvc *service.TorrentService, peerRepo repository.PeerRepository, userRepo repository.UserRepository) *TorrentHandler {
	return &TorrentHandler{torrentSvc: torrentSvc, peerRepo: peerRepo, userRepo: userRepo}
}

// HandleUpload handles POST /api/v1/torrents.
func (h *TorrentHandler) HandleUpload(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxTorrentFileSize)

	if err := r.ParseMultipartForm(maxTorrentFileSize); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid multipart form or file too large")
		return
	}

	file, _, err := r.FormFile("torrent_file")
	if err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "torrent_file is required")
		return
	}
	defer func() { _ = file.Close() }()

	fileData, err := io.ReadAll(file)
	if err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "failed to read torrent file")
		return
	}

	categoryID, _ := strconv.ParseInt(r.FormValue("category_id"), 10, 64)
	if categoryID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "valid category_id is required")
		return
	}

	anonymous, _ := strconv.ParseBool(r.FormValue("anonymous"))

	req := service.UploadTorrentRequest{
		Name:        r.FormValue("name"),
		Description: r.FormValue("description"),
		Nfo:         r.FormValue("nfo"),
		CategoryID:  categoryID,
		Anonymous:   anonymous,
	}

	torrent, err := h.torrentSvc.Upload(r.Context(), fileData, req, userID)
	if err != nil {
		handleTorrentError(w, err)
		return
	}

	JSON(w, http.StatusCreated, map[string]interface{}{
		"torrent": torrentResponse(torrent),
	})
}

// HandleList handles GET /api/v1/torrents.
func (h *TorrentHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	opts := repository.ListTorrentsOptions{
		Search:    r.URL.Query().Get("search"),
		SortBy:    r.URL.Query().Get("sort"),
		SortOrder: r.URL.Query().Get("order"),
	}

	if catStr := r.URL.Query().Get("cat"); catStr != "" {
		catID, err := strconv.ParseInt(catStr, 10, 64)
		if err == nil && catID > 0 {
			opts.CategoryID = &catID
		}
	}

	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		opts.Page, _ = strconv.Atoi(pageStr)
	}

	if ppStr := r.URL.Query().Get("per_page"); ppStr != "" {
		opts.PerPage, _ = strconv.Atoi(ppStr)
	}

	if afterStr := r.URL.Query().Get("created_after"); afterStr != "" {
		if t, err := time.Parse(time.RFC3339, afterStr); err == nil {
			opts.CreatedAfter = &t
		}
	}

	if maxSeedersStr := r.URL.Query().Get("max_seeders"); maxSeedersStr != "" {
		if n, err := strconv.Atoi(maxSeedersStr); err == nil {
			opts.MaxSeeders = &n
		}
	}

	if uploaderStr := r.URL.Query().Get("uploader_id"); uploaderStr != "" {
		if uid, err := strconv.ParseInt(uploaderStr, 10, 64); err == nil && uid > 0 {
			opts.UploaderID = &uid
		}
	}

	torrents, total, err := h.torrentSvc.List(r.Context(), opts)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list torrents")
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

// HandleGetByID handles GET /api/v1/torrents/{id}.
func (h *TorrentHandler) HandleGetByID(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid torrent ID")
		return
	}

	torrent, err := h.torrentSvc.GetByID(r.Context(), id)
	if err != nil {
		handleTorrentError(w, err)
		return
	}

	tResp := torrentResponse(torrent)

	// Enrich with uploader info (unless anonymous)
	if !torrent.Anonymous && h.userRepo != nil {
		if uploader, err := h.userRepo.GetByID(r.Context(), torrent.UploaderID); err == nil {
			tResp["uploader_name"] = uploader.Username
		}
	}

	resp := map[string]interface{}{
		"torrent": tResp,
	}

	if h.peerRepo != nil {
		if peers, err := h.peerRepo.ListByTorrent(r.Context(), id, 50); err == nil {
			peerItems := make([]map[string]interface{}, len(peers))
			for i := range peers {
				peerItems[i] = peerResponse(&peers[i])
			}
			resp["peers"] = peerItems
		}
	}

	JSON(w, http.StatusOK, resp)
}

// HandleDownload handles GET /api/v1/torrents/{id}/download.
func (h *TorrentHandler) HandleDownload(w http.ResponseWriter, r *http.Request) {
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

	data, filename, err := h.torrentSvc.DownloadTorrent(r.Context(), id, userID)
	if err != nil {
		handleTorrentError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/x-bittorrent")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// HandleEdit handles PUT /api/v1/torrents/{id}.
func (h *TorrentHandler) HandleEdit(w http.ResponseWriter, r *http.Request) {
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

	var req service.EditTorrentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	torrent, err := h.torrentSvc.EditTorrent(r.Context(), id, userID, perms, req)
	if err != nil {
		handleTorrentError(w, err)
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"torrent": torrentResponse(torrent),
	})
}

// HandleDelete handles DELETE /api/v1/torrents/{id}.
func (h *TorrentHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
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
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if body.Reason == "" {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "reason is required")
		return
	}

	if err := h.torrentSvc.DeleteTorrent(r.Context(), id, userID, perms); err != nil {
		handleTorrentError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleRequestReseed handles POST /api/v1/torrents/{id}/reseed.
func (h *TorrentHandler) HandleRequestReseed(w http.ResponseWriter, r *http.Request) {
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

	if err := h.torrentSvc.RequestReseed(r.Context(), id, userID); err != nil {
		handleTorrentError(w, err)
		return
	}

	JSON(w, http.StatusCreated, map[string]interface{}{
		"message": "reseed request created",
	})
}

// HandleGetReseedCount handles GET /api/v1/torrents/{id}/reseed.
func (h *TorrentHandler) HandleGetReseedCount(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid torrent ID")
		return
	}

	count, err := h.torrentSvc.GetReseedCount(r.Context(), id)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to get reseed count")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"count": count,
	})
}

func handleTorrentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidTorrent):
		ErrorResponse(w, http.StatusBadRequest, "invalid_torrent", err.Error())
	case errors.Is(err, service.ErrDuplicateTorrent):
		ErrorResponse(w, http.StatusConflict, "duplicate_torrent", "a torrent with this info_hash already exists")
	case errors.Is(err, service.ErrTorrentNotFound):
		ErrorResponse(w, http.StatusNotFound, "not_found", "torrent not found")
	case errors.Is(err, service.ErrForbidden):
		ErrorResponse(w, http.StatusForbidden, "forbidden", "you do not have permission to perform this action")
	case errors.Is(err, service.ErrDuplicateReseedRequest):
		ErrorResponse(w, http.StatusConflict, "duplicate_reseed_request", err.Error())
	default:
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "an unexpected error occurred")
	}
}

func torrentResponse(t *model.Torrent) map[string]interface{} {
	resp := map[string]interface{}{
		"id":              t.ID,
		"name":            t.Name,
		"info_hash":       hex.EncodeToString(t.InfoHash),
		"size":            t.Size,
		"category_id":     t.CategoryID,
		"category_name":   t.CategoryName,
		"uploader_id":     t.UploaderID,
		"anonymous":       t.Anonymous,
		"seeders":         t.Seeders,
		"leechers":        t.Leechers,
		"times_completed": t.TimesCompleted,
		"comments_count":  t.CommentsCount,
		"file_count":      t.FileCount,
		"created_at":      t.CreatedAt,
		"updated_at":      t.UpdatedAt,
	}
	if t.Description != nil {
		resp["description"] = *t.Description
	}
	if t.Nfo != nil {
		resp["nfo"] = *t.Nfo
	}
	if t.Files != nil && len(*t.Files) > 0 {
		resp["files"] = json.RawMessage(*t.Files)
	}
	return resp
}

func peerResponse(p *model.Peer) map[string]interface{} {
	return map[string]interface{}{
		"user_id":       p.UserID,
		"uploaded":      p.Uploaded,
		"downloaded":    p.Downloaded,
		"left_bytes":    p.LeftBytes,
		"seeder":        p.Seeder,
		"agent":         p.Agent,
		"last_announce": p.LastAnnounce,
	}
}
