package handler

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

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
}

// NewTorrentHandler creates a new TorrentHandler.
func NewTorrentHandler(torrentSvc *service.TorrentService) *TorrentHandler {
	return &TorrentHandler{torrentSvc: torrentSvc}
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

	JSON(w, http.StatusOK, map[string]interface{}{
		"torrent": torrentResponse(torrent),
	})
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

func handleTorrentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidTorrent):
		ErrorResponse(w, http.StatusBadRequest, "invalid_torrent", err.Error())
	case errors.Is(err, service.ErrDuplicateTorrent):
		ErrorResponse(w, http.StatusConflict, "duplicate_torrent", "a torrent with this info_hash already exists")
	case errors.Is(err, service.ErrTorrentNotFound):
		ErrorResponse(w, http.StatusNotFound, "not_found", "torrent not found")
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
	return resp
}
