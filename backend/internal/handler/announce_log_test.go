package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// --- stubs ---

type stubAnnounceEventRepo struct {
	// events is one page of the log, newest first, as ListByUser returns it.
	events []repository.AnnounceEventWithTorrent
	total  int64

	listErr error
}

func (s *stubAnnounceEventRepo) Create(context.Context, *model.AnnounceEvent) error { return nil }

func (s *stubAnnounceEventRepo) ListByUser(_ context.Context, _ int64, _, _ int) ([]repository.AnnounceEventWithTorrent, int64, error) {
	if s.listErr != nil {
		return nil, 0, s.listErr
	}
	return s.events, s.total, nil
}

// Housekeeping belongs to the nightly worker. Nothing on an HTTP path may delete
// from this log, so a call here is a bug and says so.
func (s *stubAnnounceEventRepo) DeleteOlderThan(context.Context, time.Time, int) (int64, error) {
	return 0, errors.New("DeleteOlderThan: not reachable from an HTTP handler")
}

// Reindex is housekeeping too, and equally unreachable from an HTTP path, so it
// fails the same way rather than returning a plausible zero value.
func (s *stubAnnounceEventRepo) Reindex(context.Context) (repository.ReindexResult, error) {
	return repository.ReindexResult{}, errors.New("Reindex: not reachable from an HTTP handler")
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

	r := withAuth(withChiURLParam(httptest.NewRequest(http.MethodGet, "/", nil), "id", "42"), 43, false)
	w := httptest.NewRecorder()
	h.HandleList(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 reading another member's log, got %d: %s", w.Code, w.Body.String())
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

// The tracker stopped recording announce IPs in migration 080. Decoding into a
// map rather than a struct is deliberate: a typed decode ignores fields it does
// not know about, so it would pass just as happily if the handler started
// serialising an address again.
func TestAnnounceLog_ResponseCarriesNoIPAddress(t *testing.T) {
	events := &stubAnnounceEventRepo{events: sampleAnnounceEvents(), total: 2}
	h := NewAnnounceLogHandler(events, &stubAnnounceRollupRepo{}, nil)

	r := withAuth(withChiURLParam(httptest.NewRequest(http.MethodGet, "/", nil), "id", "42"), 42, false)
	w := httptest.NewRecorder()
	h.HandleList(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var body struct {
		Events []map[string]json.RawMessage `json:"events"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(body.Events) == 0 {
		t.Fatal("expected the sample events to be returned")
	}
	for i, e := range body.Events {
		if _, ok := e["ip"]; ok {
			t.Errorf("event %d serialises an ip field: %v", i, e)
		}
	}
}
