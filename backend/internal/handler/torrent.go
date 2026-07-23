package handler

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/metadata"
	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

const maxTorrentFileSize = 10 << 20 // 10 MB

// TorrentHandler handles torrent HTTP endpoints.
type TorrentHandler struct {
	torrentSvc   *service.TorrentService
	peerRepo     repository.PeerRepository
	userRepo     repository.UserRepository
	categoryRepo repository.CategoryRepository
}

// NewTorrentHandler creates a new TorrentHandler.
func NewTorrentHandler(torrentSvc *service.TorrentService, peerRepo repository.PeerRepository, userRepo repository.UserRepository, categoryRepo repository.CategoryRepository) *TorrentHandler {
	return &TorrentHandler{torrentSvc: torrentSvc, peerRepo: peerRepo, userRepo: userRepo, categoryRepo: categoryRepo}
}

// HandleUpload handles POST /api/v1/torrents.
func (h *TorrentHandler) HandleUpload(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}

	// Check upload restriction.
	if h.userRepo != nil {
		if user, uErr := h.userRepo.GetByID(r.Context(), userID); uErr == nil && !user.CanUpload {
			ErrorResponse(w, http.StatusForbidden, "forbidden", "Your upload privileges have been suspended")
			return
		}
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

	// Optional category-schema metadata, submitted as a JSON object string.
	var meta map[string]any
	if raw := r.FormValue("metadata"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &meta); err != nil {
			ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid metadata JSON")
			return
		}
	}

	req := service.UploadTorrentRequest{
		Name:        r.FormValue("name"),
		Description: r.FormValue("description"),
		Nfo:         r.FormValue("nfo"),
		CategoryID:  categoryID,
		Anonymous:   anonymous,
		Metadata:    meta,
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

	filters, err := h.parseMetadataFilters(r.Context(), opts.CategoryID, r.URL.Query())
	if err != nil {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	opts.MetadataFilters = filters

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

	userID, _ := middleware.UserIDFromContext(r.Context())
	perms := middleware.PermissionsFromContext(r.Context())

	// Viewer-gated: a pending/rejected torrent is a 404 for anyone but its uploader
	// and staff (unless the site opts into public visibility of pending items).
	torrent, err := h.torrentSvc.GetByIDForViewer(r.Context(), id, userID, perms)
	if err != nil {
		handleTorrentError(w, err)
		return
	}

	tResp := torrentResponse(torrent)

	// Build category breadcrumb chain and attach the effective metadata schema so
	// the detail page can render field labels without a second request.
	if h.categoryRepo != nil {
		tResp["category_path"] = h.buildCategoryPath(r.Context(), torrent.CategoryID)
		if fields, err := service.ResolveCategorySchema(r.Context(), h.categoryRepo, torrent.CategoryID); err == nil {
			if fields == nil {
				fields = []metadata.FieldDef{}
			}
			tResp["metadata_schema"] = fields
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
	perms := middleware.PermissionsFromContext(r.Context())

	// Check download restriction.
	if h.userRepo != nil {
		if user, uErr := h.userRepo.GetByID(r.Context(), userID); uErr == nil && !user.CanDownload {
			ErrorResponse(w, http.StatusForbidden, "forbidden", "Your download privileges have been suspended")
			return
		}
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid torrent ID")
		return
	}

	data, filename, err := h.torrentSvc.DownloadTorrent(r.Context(), id, userID, perms)
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
	case errors.Is(err, metadata.ErrInvalidValues):
		ErrorResponse(w, http.StatusUnprocessableEntity, "validation_error", err.Error())
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
	case errors.Is(err, service.ErrNotPending):
		ErrorResponse(w, http.StatusConflict, "not_pending", "torrent is not pending moderation")
	case errors.Is(err, service.ErrModerationUnavailable):
		ErrorResponse(w, http.StatusServiceUnavailable, "moderation_unavailable", "moderation is unavailable")
	default:
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "an unexpected error occurred")
	}
}

func torrentResponse(t *model.Torrent) map[string]interface{} {
	uploaderName := t.UploaderName
	if uploaderName == "" {
		if t.Anonymous {
			uploaderName = "Anonymous"
		} else {
			uploaderName = "Unknown"
		}
	}

	uploaderID := t.UploaderID
	if t.Anonymous {
		uploaderID = 0
	}

	resp := map[string]interface{}{
		"id":                 t.ID,
		"name":               t.Name,
		"info_hash":          hex.EncodeToString(t.InfoHash),
		"size":               t.Size,
		"category_id":        t.CategoryID,
		"category_name":      t.CategoryName,
		"category_image_url": t.CategoryImageURL,
		"uploader_id":        uploaderID,
		"anonymous":          t.Anonymous,
		"uploader_name":      uploaderName,
		"uploader_warned":    t.UploaderWarned,
		"seeders":            t.Seeders,
		"leechers":           t.Leechers,
		"times_completed":    t.TimesCompleted,
		"comments_count":     t.CommentsCount,
		"file_count":         t.FileCount,
		"free":               t.Free,
		"silver":             t.Silver,
		"banned":             t.Banned,
		"created_at":         t.CreatedAt,
		"updated_at":         t.UpdatedAt,
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
	if len(t.Metadata) > 0 {
		resp["metadata"] = json.RawMessage(t.Metadata)
	} else {
		resp["metadata"] = json.RawMessage("{}")
	}
	resp["moderation"] = moderationResponse(t)
	return resp
}

// moderationResponse builds the moderation sub-object attached to every torrent
// response: always the status, plus assignee/approver details when present. The
// assigned moderator is queue state, so it's only surfaced while the torrent is
// still restricted (pending/rejected) — an approved torrent exposes only its
// public "approved by" credit, not who happened to claim the review.
func moderationResponse(t *model.Torrent) map[string]interface{} {
	m := map[string]interface{}{
		"status":        t.ModerationStatus,
		"message_count": t.MessageCount,
	}
	if t.AssignedModeratorID != nil && t.ModerationRestricted() {
		m["assigned_moderator_id"] = *t.AssignedModeratorID
		m["assigned_moderator_name"] = t.AssignedModeratorName
	}
	if t.ApprovedBy != nil {
		m["approved_by_id"] = *t.ApprovedBy
		m["approved_by_name"] = t.ApprovedByName
	}
	if t.ApprovedAt != nil {
		m["approved_at"] = *t.ApprovedAt
	}
	return m
}

// metadataFilterPrefix marks browse/search query params that filter by a
// category-schema metadata field, e.g. ?meta_year=2024&meta_year__gte=2000.
const metadataFilterPrefix = "meta_"

// parseMetadataFilters builds typed metadata predicates (BE-3.13a) from
// meta_<key> (equality), meta_<key>__gte and meta_<key>__lte (numeric range)
// query params. A raw query value is an untyped string, so it resolves the
// selected category's effective schema to coerce each value to its field's real
// type before building a predicate. Metadata filtering therefore requires a
// valid category: any meta_* param without one — or naming a field the schema
// doesn't define, or a range on a non-numeric field — is a client error the
// caller surfaces as 400.
func (h *TorrentHandler) parseMetadataFilters(ctx context.Context, categoryID *int64, q url.Values) ([]repository.MetadataFilter, error) {
	// Collect the meta_* params actually present (non-empty), so unrecognized
	// ones can be rejected rather than silently ignored.
	var present []string
	for name, vals := range q {
		if strings.HasPrefix(name, metadataFilterPrefix) && len(vals) > 0 && vals[0] != "" {
			present = append(present, name)
		}
	}
	if len(present) == 0 {
		return nil, nil
	}
	if categoryID == nil {
		return nil, fmt.Errorf("metadata filters require a category (cat)")
	}
	if h.categoryRepo == nil {
		return nil, fmt.Errorf("metadata filtering is unavailable")
	}

	schema, err := service.ResolveCategorySchema(ctx, h.categoryRepo, *categoryID)
	if err != nil {
		return nil, fmt.Errorf("could not resolve category schema for metadata filters")
	}

	recognized := make(map[string]bool)
	var filters []repository.MetadataFilter

	for i := range schema {
		f := schema[i]
		base := metadataFilterPrefix + f.Key

		if raw := q.Get(base); raw != "" {
			val, cErr := metadata.CoerceFilterValue(f, raw)
			if cErr != nil {
				return nil, cErr
			}
			// Multiselect stores an array; wrap the single value so JSONB
			// containment matches rows whose array includes it.
			if f.Type == metadata.TypeMultiselect {
				if s, ok := val.(string); ok {
					val = []string{s}
				}
			}
			filters = append(filters, repository.MetadataFilter{Key: f.Key, Op: repository.MetaFilterEq, Value: val})
			recognized[base] = true
		}

		// Range filters only make sense for numeric fields.
		if f.Type == metadata.TypeNumber {
			for suffix, op := range map[string]repository.MetadataFilterOp{
				"__gte": repository.MetaFilterGte,
				"__lte": repository.MetaFilterLte,
			} {
				name := base + suffix
				raw := q.Get(name)
				if raw == "" {
					continue
				}
				val, cErr := metadata.CoerceFilterValue(f, raw)
				if cErr != nil {
					return nil, cErr
				}
				filters = append(filters, repository.MetadataFilter{Key: f.Key, Op: op, Value: val})
				recognized[name] = true
			}
		}
	}

	for _, name := range present {
		if !recognized[name] {
			return nil, fmt.Errorf("unknown metadata filter %q for this category", name)
		}
	}

	return filters, nil
}

// buildCategoryPath walks up the parent chain and returns the full breadcrumb
// from root to leaf, e.g. [{"id":1,"name":"Software"},{"id":5,"name":"Windows"}]
func (h *TorrentHandler) buildCategoryPath(ctx context.Context, categoryID int64) []map[string]interface{} {
	var chain []map[string]interface{}
	seen := make(map[int64]bool) // guard against cycles
	currentID := categoryID

	for !seen[currentID] {
		seen[currentID] = true

		cat, err := h.categoryRepo.GetByID(ctx, currentID)
		if err != nil {
			break
		}
		entry := map[string]interface{}{
			"id":   cat.ID,
			"name": cat.Name,
		}
		if cat.ImageURL != nil {
			entry["image_url"] = *cat.ImageURL
		}
		chain = append(chain, entry)
		if cat.ParentID == nil {
			break
		}
		currentID = *cat.ParentID
	}

	// Reverse so root is first
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

func peerResponse(p *model.Peer) map[string]interface{} {
	return map[string]interface{}{
		"user_id":       p.UserID,
		"uploaded":      p.Uploaded,
		"downloaded":    p.Downloaded,
		"left_bytes":    p.LeftBytes,
		"port":          p.Port,
		"seeder":        p.Seeder,
		"agent":         p.Agent,
		"last_announce": p.LastAnnounce,
	}
}
