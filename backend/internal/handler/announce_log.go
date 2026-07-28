package handler

import (
	"encoding/hex"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// announceLogMonthsShown is how many monthly totals the listing returns. Ten years
// is well past any retention window, so the cap only exists to keep an old
// account's response bounded.
const announceLogMonthsShown = 120

// AnnounceLogHandler serves a member's announce log: the raw rows the tracker
// retained about their client, plus the monthly totals that outlive them.
//
// This exists for ratio disputes — "your client reported this" is the evidence in
// every one of them — which is why staff can read it as well as the member. It is
// deliberately read-only and on-screen: there is no bulk export, and the tracker
// does not record announce IP addresses at all (see migration 080).
type AnnounceLogHandler struct {
	events   repository.AnnounceEventRepository
	rollups  repository.AnnounceRollupRepository
	settings *service.SiteSettingsService
}

// NewAnnounceLogHandler creates a new AnnounceLogHandler.
func NewAnnounceLogHandler(
	events repository.AnnounceEventRepository,
	rollups repository.AnnounceRollupRepository,
	settings *service.SiteSettingsService,
) *AnnounceLogHandler {
	return &AnnounceLogHandler{events: events, rollups: rollups, settings: settings}
}

// HandleList handles GET /api/v1/users/{id}/announce-log — the retained raw
// announces plus the monthly totals kept beyond retention.
func (h *AnnounceLogHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.authorize(w, r)
	if !ok {
		return
	}

	page, perPage := parsePagination(r)

	events, total, err := h.events.ListByUser(r.Context(), userID, page, perPage)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list announce log")
		return
	}

	periods, err := h.rollups.ListByUser(r.Context(), userID, announceLogMonthsShown)
	if err != nil {
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "failed to list monthly totals")
		return
	}

	items := make([]map[string]interface{}, len(events))
	for i := range events {
		items[i] = announceEventJSON(&events[i])
	}

	monthly := make([]map[string]interface{}, len(periods))
	for i := range periods {
		p := &periods[i]
		monthly[i] = map[string]interface{}{
			"year_month":         p.YearMonth,
			"uploaded":           p.Uploaded,
			"downloaded":         p.Downloaded,
			"counted_downloaded": p.CountedDownloaded,
			"announces":          p.Announces,
			"seed_announces":     p.SeedAnnounces,
			"ratio":              safeRatio(p.Uploaded, p.CountedDownloaded),
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"events":   items,
		"total":    total,
		"page":     page,
		"per_page": perPage,
		"monthly":  monthly,
		// So the page can say how far back the raw rows go instead of leaving a
		// member to guess why last year's announces are missing.
		"retention_days": h.retentionDays(r),
	})
}

// authorize resolves the target member from the URL and enforces owner-or-staff.
// Same gate as the activity tabs: this is a member's own transfer detail, and the
// per-announce byte deltas are exactly what a rival would want in order to work out
// what someone is seeding and when they are online.
func (h *AnnounceLogHandler) authorize(w http.ResponseWriter, r *http.Request) (int64, bool) {
	viewerID, _ := middleware.UserIDFromContext(r.Context())
	viewerPerms := middleware.PermissionsFromContext(r.Context())

	userID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || userID <= 0 {
		ErrorResponse(w, http.StatusBadRequest, "bad_request", "invalid user ID")
		return 0, false
	}

	if viewerID != userID && !viewerPerms.IsStaff() {
		ErrorResponse(w, http.StatusForbidden, "forbidden", "you cannot view this user's announce log")
		return 0, false
	}
	return userID, true
}

// retentionDays reports how long raw announces actually survive — the effective
// window, not the configured one. Zero is meaningful to the caller: it means
// pruning is disabled and the raw log is kept indefinitely, not that it is kept
// for no time at all.
//
// The comment here used to say that two readers of one setting disagreeing on
// its default was "a bug that only shows up as the site telling a member one
// thing while the prune does another". It guarded the default and missed the
// floor, so the site did exactly that: class promotion holds the window open at
// 31 days by default, and a member on a site configured for 7 was told 7 about
// rows that live for 31. Wrong in the direction that matters, since a member
// reading this is being told how long their transfer history is retained.
//
// Both this and the prune now read service.ResolveAnnounceRetention, which is
// the only place the answer is worked out.
func (h *AnnounceLogHandler) retentionDays(r *http.Request) int {
	return service.ResolveAnnounceRetention(r.Context(), h.settings).EffectiveDays
}

func announceEventJSON(e *repository.AnnounceEventWithTorrent) map[string]interface{} {
	return map[string]interface{}{
		"id":                       e.ID,
		"torrent_id":               e.TorrentID,
		"torrent_name":             e.TorrentName,
		"event":                    e.Event,
		"port":                     e.Port,
		"peer_id":                  hex.EncodeToString(e.PeerID),
		"uploaded":                 e.Uploaded,
		"downloaded":               e.Downloaded,
		"left_bytes":               e.LeftBytes,
		"uploaded_delta":           e.UploadedDelta,
		"downloaded_delta":         e.DownloadedDelta,
		"counted_downloaded_delta": e.CountedDownloadedDelta,
		"seeder":                   e.Seeder,
		"announced_at":             e.AnnouncedAt,
	}
}
