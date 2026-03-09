package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// UserActivityHandler handles user torrent activity HTTP endpoints.
type UserActivityHandler struct {
	torrentSvc   *service.TorrentService
	peerRepo     repository.PeerRepository
	transferRepo repository.TransferHistoryRepository
}

// NewUserActivityHandler creates a new UserActivityHandler.
func NewUserActivityHandler(
	torrentSvc *service.TorrentService,
	peerRepo repository.PeerRepository,
	transferRepo repository.TransferHistoryRepository,
) *UserActivityHandler {
	return &UserActivityHandler{
		torrentSvc:   torrentSvc,
		peerRepo:     peerRepo,
		transferRepo: transferRepo,
	}
}

// HandleUserTorrents handles GET /api/v1/users/{id}/torrents — public uploaded torrents.
// This endpoint is publicly accessible. Anonymous torrents are filtered out unless
// the viewer is the profile owner or staff.
func (h *UserActivityHandler) HandleUserTorrents(w http.ResponseWriter, r *http.Request) {
	// Auth is optional — endpoint is public
	viewerID, hasAuth := middleware.UserIDFromContext(r.Context())
	viewerPerms := middleware.PermissionsFromContext(r.Context())

	profileUserID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || profileUserID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid user ID")
		return
	}

	page, perPage := parsePagination(r)

	opts := repository.ListTorrentsOptions{
		UploaderID: &profileUserID,
		Page:       page,
		PerPage:    perPage,
		SortBy:     "created_at",
		SortOrder:  "desc",
	}

	torrents, total, err := h.torrentSvc.List(r.Context(), opts)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list user torrents")
		return
	}

	isOwnerOrStaff := hasAuth && (viewerID == profileUserID || viewerPerms.IsStaff())

	// Filter out anonymous torrents for non-owner/non-staff viewers
	var items []map[string]interface{}
	for i := range torrents {
		t := &torrents[i]

		if t.Anonymous && !isOwnerOrStaff {
			total-- // adjust total to reflect filtered results
			continue
		}

		items = append(items, map[string]interface{}{
			"id":              t.ID,
			"name":            t.Name,
			"size":            t.Size,
			"seeders":         t.Seeders,
			"leechers":        t.Leechers,
			"times_completed": t.TimesCompleted,
			"category_name":   t.CategoryName,
			"created_at":      t.CreatedAt,
		})
	}

	if items == nil {
		items = []map[string]interface{}{}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"torrents": items,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

// HandleUserActivity handles GET /api/v1/users/{id}/activity — seeding/leeching/history (owner + staff).
func (h *UserActivityHandler) HandleUserActivity(w http.ResponseWriter, r *http.Request) {
	viewerID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
		return
	}
	viewerPerms := middleware.PermissionsFromContext(r.Context())

	profileUserID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || profileUserID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid user ID")
		return
	}

	// Only the profile owner or staff can see activity
	if viewerID != profileUserID && !viewerPerms.IsStaff() {
		ErrorResponse(w, http.StatusForbidden, "forbidden", "you cannot view this user's activity")
		return
	}

	page, perPage := parsePagination(r)
	tab := r.URL.Query().Get("tab") // "seeding", "leeching", "history"

	switch tab {
	case "seeding":
		h.handleSeedingTab(w, r, profileUserID, page, perPage)
	case "leeching":
		h.handleLeechingTab(w, r, profileUserID, page, perPage)
	case "history":
		h.handleHistoryTab(w, r, profileUserID, page, perPage)
	default:
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "tab must be one of: seeding, leeching, history")
	}
}

func (h *UserActivityHandler) handleSeedingTab(w http.ResponseWriter, r *http.Request, userID int64, page, perPage int) {
	peers, total, err := h.peerRepo.ListByUserSeeding(r.Context(), userID, page, perPage)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list seeding activity")
		return
	}

	items := make([]map[string]interface{}, len(peers))
	for i := range peers {
		p := &peers[i]
		items[i] = map[string]interface{}{
			"torrent_id":    p.TorrentID,
			"torrent_name":  p.TorrentName,
			"uploaded":      p.Uploaded,
			"downloaded":    p.Downloaded,
			"ratio":         safeRatio(p.Uploaded, p.Downloaded),
			"seeder":        p.Seeder,
			"ip":            p.IP,
			"port":          p.Port,
			"last_announce": p.LastAnnounce,
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"activity": items,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

func (h *UserActivityHandler) handleLeechingTab(w http.ResponseWriter, r *http.Request, userID int64, page, perPage int) {
	peers, total, err := h.peerRepo.ListByUserLeeching(r.Context(), userID, page, perPage)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list leeching activity")
		return
	}

	items := make([]map[string]interface{}, len(peers))
	for i := range peers {
		p := &peers[i]
		items[i] = map[string]interface{}{
			"torrent_id":    p.TorrentID,
			"torrent_name":  p.TorrentName,
			"uploaded":      p.Uploaded,
			"downloaded":    p.Downloaded,
			"ratio":         safeRatio(p.Uploaded, p.Downloaded),
			"seeder":        p.Seeder,
			"ip":            p.IP,
			"port":          p.Port,
			"last_announce": p.LastAnnounce,
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"activity": items,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

func (h *UserActivityHandler) handleHistoryTab(w http.ResponseWriter, r *http.Request, userID int64, page, perPage int) {
	history, total, err := h.transferRepo.ListByUser(r.Context(), userID, page, perPage)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list transfer history")
		return
	}

	items := make([]map[string]interface{}, len(history))
	for i := range history {
		entry := &history[i]
		items[i] = map[string]interface{}{
			"torrent_id":    entry.TorrentID,
			"torrent_name":  entry.TorrentName,
			"uploaded":      entry.Uploaded,
			"downloaded":    entry.Downloaded,
			"ratio":         safeRatio(entry.Uploaded, entry.Downloaded),
			"seeder":        entry.Seeder,
			"completed_at":  entry.CompletedAt,
			"last_announce": entry.LastAnnounce,
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"activity": items,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

func parsePagination(r *http.Request) (page, perPage int) {
	page = 1
	perPage = 25
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if pp := r.URL.Query().Get("per_page"); pp != "" {
		if v, err := strconv.Atoi(pp); err == nil && v > 0 && v <= 100 {
			perPage = v
		}
	}
	return page, perPage
}

func safeRatio(uploaded, downloaded int64) float64 {
	if downloaded == 0 {
		if uploaded == 0 {
			return 0
		}
		return -1 // sentinel for "infinite" ratio — frontend formatRatio handles this
	}
	return float64(uploaded) / float64(downloaded)
}
