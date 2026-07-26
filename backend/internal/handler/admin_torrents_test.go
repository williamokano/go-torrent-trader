package handler

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/middleware"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// sampleHashHex is a well-formed 40-character hex info hash.
const sampleHashHex = "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0"

// --- info hash parsing ------------------------------------------------------

func TestParseInfoHashHex(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		ok   bool
	}{
		{"lowercase hex", sampleHashHex, true},
		{"uppercase hex", strings.ToUpper(sampleHashHex), true},
		{"surrounded by whitespace", "  " + sampleHashHex + "\n", true},
		{"a torrent name", "Some.Release.2026.1080p", false},
		{"39 hex characters", sampleHashHex[:39], false},
		{"41 hex characters", sampleHashHex + "a", false},
		{"right length, not hex", strings.Repeat("z", 40), false},
		{"empty", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hash, ok := parseInfoHashHex(tc.in)
			if ok != tc.ok {
				t.Fatalf("parseInfoHashHex(%q) ok = %v, want %v", tc.in, ok, tc.ok)
			}
			if ok && len(hash) != 20 {
				t.Errorf("decoded %d bytes, want the 20 a BitTorrent info hash has", len(hash))
			}
		})
	}
}

// --- listing filters --------------------------------------------------------

// The identifier a takedown notice gives you is a hash, and it is the only one
// that survives a rename. Pasting it into the search box has to find the torrent.
func TestAdminListTorrentsRoutesAHashInSearchToTheHashFilter(t *testing.T) {
	d := newAdminDeps()
	h := d.handler()

	req := adminAuthed(httptest.NewRequest(http.MethodGet,
		"/api/v1/admin/torrents?search="+sampleHashHex, nil), 1)
	h.HandleListTorrents(httptest.NewRecorder(), req)

	opts := d.torrents.lastOpts
	want, err := hex.DecodeString(sampleHashHex)
	if err != nil {
		t.Fatalf("decoding fixture: %v", err)
	}
	if !bytes.Equal(opts.InfoHash, want) {
		t.Errorf("InfoHash = %x, want %x", opts.InfoHash, want)
	}
	// And it must not also be searched as a name: the full-text predicate would
	// match nothing and AND itself with the hash, returning zero rows.
	if opts.Search != "" {
		t.Errorf("Search = %q, want it left empty when the term is a hash", opts.Search)
	}
}

func TestAdminListTorrentsTreatsOrdinaryTermsAsNames(t *testing.T) {
	d := newAdminDeps()
	h := d.handler()

	req := adminAuthed(httptest.NewRequest(http.MethodGet,
		"/api/v1/admin/torrents?search=Some.Release.2026", nil), 1)
	h.HandleListTorrents(httptest.NewRecorder(), req)

	opts := d.torrents.lastOpts
	if opts.Search != "Some.Release.2026" {
		t.Errorf("Search = %q, want the term unchanged", opts.Search)
	}
	if opts.InfoHash != nil {
		t.Errorf("InfoHash = %x, want nil for a name search", opts.InfoHash)
	}
}

func TestAdminListTorrentsAcceptsAnExplicitHash(t *testing.T) {
	d := newAdminDeps()
	h := d.handler()

	req := adminAuthed(httptest.NewRequest(http.MethodGet,
		"/api/v1/admin/torrents?info_hash="+strings.ToUpper(sampleHashHex), nil), 1)
	w := httptest.NewRecorder()
	h.HandleListTorrents(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	want, _ := hex.DecodeString(sampleHashHex)
	if !bytes.Equal(d.torrents.lastOpts.InfoHash, want) {
		t.Errorf("InfoHash = %x, want %x (case-insensitive)", d.torrents.lastOpts.InfoHash, want)
	}
}

// A malformed explicit hash is rejected rather than ignored. Silently dropping the
// filter would list every torrent on the tracker, which reads as "your hash matched
// everything" — the opposite of the truth.
func TestAdminListTorrentsRejectsAMalformedHash(t *testing.T) {
	for _, bad := range []string{"notahash", sampleHashHex[:39], strings.Repeat("z", 40)} {
		d := newAdminDeps()
		h := d.handler()

		req := adminAuthed(httptest.NewRequest(http.MethodGet,
			"/api/v1/admin/torrents?info_hash="+bad, nil), 1)
		w := httptest.NewRecorder()
		h.HandleListTorrents(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("info_hash=%q: status = %d, want 400", bad, w.Code)
		}
		if d.torrents.lastOpts.InfoHash != nil {
			t.Errorf("info_hash=%q: the listing ran anyway", bad)
		}
	}
}

func TestAdminListTorrentsFiltersOnBanned(t *testing.T) {
	for _, tc := range []struct {
		query string
		want  bool
	}{{"banned=true", true}, {"banned=false", false}} {
		d := newAdminDeps()
		h := d.handler()

		req := adminAuthed(httptest.NewRequest(http.MethodGet,
			"/api/v1/admin/torrents?"+tc.query, nil), 1)
		w := httptest.NewRecorder()
		h.HandleListTorrents(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", tc.query, w.Code)
		}
		got := d.torrents.lastOpts.Banned
		if got == nil || *got != tc.want {
			t.Errorf("%s: Banned = %v, want %v", tc.query, got, tc.want)
		}
		// The banned listing is only meaningful because the admin view sees hidden
		// rows; without this the filter would AND with `banned = false`.
		if !d.torrents.lastOpts.IncludeHidden {
			t.Errorf("%s: the admin listing must include hidden torrents", tc.query)
		}
	}
}

func TestAdminListTorrentsOmitsTheBannedFilterByDefault(t *testing.T) {
	d := newAdminDeps()
	h := d.handler()

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/torrents", nil), 1)
	h.HandleListTorrents(httptest.NewRecorder(), req)

	if d.torrents.lastOpts.Banned != nil {
		t.Errorf("Banned = %v, want nil so both banned and active torrents are listed",
			*d.torrents.lastOpts.Banned)
	}
}

func TestAdminListTorrentsRejectsANonBooleanBannedFilter(t *testing.T) {
	d := newAdminDeps()
	h := d.handler()

	req := adminAuthed(httptest.NewRequest(http.MethodGet,
		"/api/v1/admin/torrents?banned=maybe", nil), 1)
	w := httptest.NewRecorder()
	h.HandleListTorrents(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// The hash has to come back as well as go in: a moderator who searched by hash
// needs to confirm they have the right torrent before banning it.
func TestAdminListTorrentsReturnsTheHashAndFlags(t *testing.T) {
	rawHash, _ := hex.DecodeString(sampleHashHex)
	d := newAdminDeps()
	d.torrents.list = []model.Torrent{
		{ID: 3, Name: "A movie", InfoHash: rawHash, Size: 2048, UploaderID: 7,
			UploaderName: "member", Banned: true, Free: true, Silver: false,
			Visible: true, CreatedAt: time.Now()},
	}
	d.torrents.total = 1
	h := d.handler()

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/torrents", nil), 1)
	w := httptest.NewRecorder()
	h.HandleListTorrents(w, req)

	items := decodeBody(t, w)["torrents"].([]interface{})
	torrent := items[0].(map[string]interface{})
	if torrent["info_hash"] != sampleHashHex {
		t.Errorf("info_hash = %v, want the hex form %q", torrent["info_hash"], sampleHashHex)
	}
	for key, want := range map[string]bool{"banned": true, "free": true, "silver": false, "visible": true} {
		if got, present := torrent[key]; !present {
			t.Errorf("response is missing %q", key)
		} else if got != want {
			t.Errorf("%s = %v, want %v", key, got, want)
		}
	}
}

// --- bulk actions -----------------------------------------------------------

func bulkRequest(t *testing.T, action string, ids ...int64) *http.Request {
	t.Helper()
	body, err := json.Marshal(map[string]interface{}{"action": action, "ids": ids})
	if err != nil {
		t.Fatalf("encoding request: %v", err)
	}
	return adminAuthed(httptest.NewRequest(http.MethodPost,
		"/api/v1/admin/torrents/bulk", bytes.NewReader(body)), 1)
}

// bulkStatuses reads the per-torrent breakdown as an id→status map.
func bulkStatuses(t *testing.T, w *httptest.ResponseRecorder) map[int64]string {
	t.Helper()
	var body struct {
		Results   []service.BulkTorrentResult `json:"results"`
		Succeeded int                         `json:"succeeded"`
		Failed    int                         `json:"failed"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	out := make(map[int64]string, len(body.Results))
	for _, r := range body.Results {
		out[r.ID] = r.Status
	}
	return out
}

func threeTorrents() *stubTorrentRepo {
	return &stubTorrentRepo{torrents: map[int64]*model.Torrent{
		1: {ID: 1, Name: "one", UploaderID: 7},
		2: {ID: 2, Name: "two", UploaderID: 7},
		3: {ID: 3, Name: "three", UploaderID: 7, Banned: true},
	}}
}

func TestBulkBanMarksEveryTorrent(t *testing.T) {
	torrents := threeTorrents()
	h := newTorrentAdminHandler(torrents, &stubStorage{}, newStubUserRepo(&model.User{ID: 1, Username: "admin"}))

	w := httptest.NewRecorder()
	h.HandleBulkAction(w, bulkRequest(t, "ban", 1, 2))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	statuses := bulkStatuses(t, w)
	if statuses[1] != service.BulkStatusOK || statuses[2] != service.BulkStatusOK {
		t.Fatalf("statuses = %v, want both ok", statuses)
	}
	// Asserted against what Update was handed, not against the stub's map: the
	// service holds the same pointer GetByID returned, so an in-place mutation
	// would satisfy the map without ever persisting.
	if len(torrents.updated) != 2 {
		t.Fatalf("persisted %d torrents, want 2", len(torrents.updated))
	}
	for _, got := range torrents.updated {
		if !got.Banned {
			t.Errorf("torrent %d was written back un-banned", got.ID)
		}
	}
}

func TestBulkUnbanClearsTheFlag(t *testing.T) {
	torrents := threeTorrents()
	h := newTorrentAdminHandler(torrents, &stubStorage{}, newStubUserRepo(&model.User{ID: 1}))

	w := httptest.NewRecorder()
	h.HandleBulkAction(w, bulkRequest(t, "unban", 3))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if len(torrents.updated) != 1 || torrents.updated[0].Banned {
		t.Errorf("updated = %+v, want torrent 3 written back un-banned", torrents.updated)
	}
}

func TestBulkDeleteRemovesRowsAndFiles(t *testing.T) {
	torrents := threeTorrents()
	store := &stubStorage{}
	h := newTorrentAdminHandler(torrents, store, newStubUserRepo(&model.User{ID: 1}))

	w := httptest.NewRecorder()
	h.HandleBulkAction(w, bulkRequest(t, "delete", 1, 2, 3))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if len(torrents.deleted) != 3 {
		t.Errorf("deleted rows = %v, want all three", torrents.deleted)
	}
	// The .torrent blob has to go with the row, or storage accumulates orphans that
	// nothing will ever reference again.
	if len(store.deleted) != 3 {
		t.Errorf("deleted files = %v, want all three blobs removed", store.deleted)
	}
}

// A list of ids a moderator pasted will contain one that is already gone. Failing
// the whole batch over it would leave them to work out which of twelve it was.
func TestBulkActionReportsPerTorrentOutcomes(t *testing.T) {
	torrents := threeTorrents()
	h := newTorrentAdminHandler(torrents, &stubStorage{}, newStubUserRepo(&model.User{ID: 1}))

	w := httptest.NewRecorder()
	h.HandleBulkAction(w, bulkRequest(t, "ban", 1, 999, 2))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — a partial failure is still a valid request", w.Code)
	}

	var body struct {
		Succeeded int `json:"succeeded"`
		Failed    int `json:"failed"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if body.Succeeded != 2 || body.Failed != 1 {
		t.Errorf("succeeded/failed = %d/%d, want 2/1", body.Succeeded, body.Failed)
	}

	statuses := bulkStatuses(t, w)
	if statuses[999] != service.BulkStatusNotFound {
		t.Errorf("missing torrent reported as %q, want %q", statuses[999], service.BulkStatusNotFound)
	}
	// And the ones that could be actioned were, rather than being rolled back
	// because a sibling failed.
	if len(torrents.updated) != 2 {
		t.Errorf("persisted %d torrents, want the 2 that existed", len(torrents.updated))
	}
}

// A repeated id must be acted on once. For a delete, the second pass would report
// not_found for work that had just succeeded — reading as a partial failure when
// nothing failed.
func TestBulkActionCollapsesRepeatedIDs(t *testing.T) {
	torrents := threeTorrents()
	h := newTorrentAdminHandler(torrents, &stubStorage{}, newStubUserRepo(&model.User{ID: 1}))

	w := httptest.NewRecorder()
	h.HandleBulkAction(w, bulkRequest(t, "delete", 1, 1, 1))

	var body struct {
		Results   []service.BulkTorrentResult `json:"results"`
		Succeeded int                         `json:"succeeded"`
		Failed    int                         `json:"failed"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(body.Results) != 1 || body.Failed != 0 {
		t.Errorf("results = %+v (failed %d), want one ok entry", body.Results, body.Failed)
	}
	if len(torrents.deleted) != 1 {
		t.Errorf("deleted = %v, want the torrent removed once", torrents.deleted)
	}
}

func TestBulkActionRejectsAnUnusableRequest(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"unknown action", `{"action":"incinerate","ids":[1]}`},
		{"no action", `{"ids":[1]}`},
		{"empty id list", `{"action":"ban","ids":[]}`},
		{"no ids at all", `{"action":"ban"}`},
		{"malformed JSON", `{"action":`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			torrents := threeTorrents()
			h := newTorrentAdminHandler(torrents, &stubStorage{}, newStubUserRepo(&model.User{ID: 1}))

			req := adminAuthed(httptest.NewRequest(http.MethodPost,
				"/api/v1/admin/torrents/bulk", strings.NewReader(tc.body)), 1)
			w := httptest.NewRecorder()
			h.HandleBulkAction(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
			}
			if len(torrents.updated) != 0 || len(torrents.deleted) != 0 {
				t.Error("an invalid request still changed something")
			}
		})
	}
}

// The cap bounds one request. A delete touches object storage per torrent, so an
// unbounded list is a slow request holding a connection while it runs.
func TestBulkActionRejectsMoreThanTheCap(t *testing.T) {
	ids := make([]int64, service.MaxBulkTorrents+1)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	torrents := threeTorrents()
	h := newTorrentAdminHandler(torrents, &stubStorage{}, newStubUserRepo(&model.User{ID: 1}))

	w := httptest.NewRecorder()
	h.HandleBulkAction(w, bulkRequest(t, "ban", ids...))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if len(torrents.updated) != 0 {
		t.Error("an over-long request was partly applied before being rejected")
	}
	if !strings.Contains(w.Body.String(), fmt.Sprintf("%d", service.MaxBulkTorrents)) {
		t.Errorf("the error should say what the limit is: %s", w.Body.String())
	}
}

// Exactly at the cap is allowed — an off-by-one here would be a confusing limit.
func TestBulkActionAcceptsExactlyTheCap(t *testing.T) {
	torrents := &stubTorrentRepo{torrents: map[int64]*model.Torrent{}}
	for i := 1; i <= service.MaxBulkTorrents; i++ {
		torrents.torrents[int64(i)] = &model.Torrent{ID: int64(i), UploaderID: 7}
	}
	ids := make([]int64, service.MaxBulkTorrents)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	h := newTorrentAdminHandler(torrents, &stubStorage{}, newStubUserRepo(&model.User{ID: 1}))

	w := httptest.NewRecorder()
	h.HandleBulkAction(w, bulkRequest(t, "ban", ids...))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if len(torrents.updated) != service.MaxBulkTorrents {
		t.Errorf("persisted %d torrents, want %d", len(torrents.updated), service.MaxBulkTorrents)
	}
}

func TestBulkActionRequiresAuth(t *testing.T) {
	torrents := threeTorrents()
	h := newTorrentAdminHandler(torrents, &stubStorage{}, newStubUserRepo())

	body, _ := json.Marshal(map[string]interface{}{"action": "delete", "ids": []int64{1}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/torrents/bulk", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.HandleBulkAction(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	if len(torrents.deleted) != 0 {
		t.Error("an unauthenticated request deleted a torrent")
	}
}

// The route is behind RequireAdmin, but the ban flag is admin-only at the service
// layer too, and it must stay that way: an authorization gate that only exists at
// the route is one refactor away from being gone.
func TestBulkBanIsRefusedForANonAdmin(t *testing.T) {
	torrents := threeTorrents()
	h := newTorrentAdminHandler(torrents, &stubStorage{}, newStubUserRepo(&model.User{ID: 9}))

	body, _ := json.Marshal(map[string]interface{}{"action": "ban", "ids": []int64{1, 2}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/torrents/bulk", bytes.NewReader(body))
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, int64(9))
	// A moderator: staff, but not an admin.
	ctx = context.WithValue(ctx, middleware.PermissionsKey,
		model.Permissions{Level: 50, IsModerator: true})
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h.HandleBulkAction(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with per-torrent refusals", w.Code)
	}
	statuses := bulkStatuses(t, w)
	for _, id := range []int64{1, 2} {
		if statuses[id] != service.BulkStatusForbidden {
			t.Errorf("torrent %d reported %q, want %q", id, statuses[id], service.BulkStatusForbidden)
		}
	}
	if len(torrents.updated) != 0 {
		t.Error("a non-admin banned a torrent")
	}
}

// A repository failure on one torrent is reported as an error for that torrent and
// nothing more — the rest of the batch still runs.
func TestBulkActionIsolatesARepositoryFailure(t *testing.T) {
	torrents := threeTorrents()
	torrents.updateErr = errStub
	h := newTorrentAdminHandler(torrents, &stubStorage{}, newStubUserRepo(&model.User{ID: 1}))

	w := httptest.NewRecorder()
	h.HandleBulkAction(w, bulkRequest(t, "ban", 1, 2))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	statuses := bulkStatuses(t, w)
	for _, id := range []int64{1, 2} {
		if statuses[id] != service.BulkStatusError {
			t.Errorf("torrent %d reported %q, want %q", id, statuses[id], service.BulkStatusError)
		}
	}
}

// A bulk ban has to leave one audit entry per torrent, not one for the batch, and
// each has to say it was a ban: the question afterwards is always "who banned this
// one, and when". An event that only says "edited" cannot answer it.
func TestBulkBanEmitsABanEventPerTorrent(t *testing.T) {
	bus := event.NewInMemoryBus()
	var events []*event.TorrentEditedEvent
	bus.Subscribe(event.TorrentEdited, func(_ context.Context, evt event.Event) error {
		events = append(events, evt.(*event.TorrentEditedEvent))
		return nil
	})

	torrents := threeTorrents()
	svc := service.NewTorrentService(nil, torrents, newStubUserRepo(&model.User{ID: 1, Username: "admin"}),
		&stubStorage{}, service.TorrentServiceConfig{AnnounceURL: "http://localhost/announce"}, bus, nil)
	h := NewTorrentAdminHandler(svc)

	w := httptest.NewRecorder()
	h.HandleBulkAction(w, bulkRequest(t, "ban", 1, 2))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if len(events) != 2 {
		t.Fatalf("published %d events for 2 torrents — a bulk ban must be auditable per torrent",
			len(events))
	}
	for _, e := range events {
		if e.BannedChanged == nil {
			t.Errorf("torrent %d: the event does not record that this was a ban, so the "+
				"activity log will read 'edited torrent' and answer nothing", e.TorrentID)
			continue
		}
		if !*e.BannedChanged {
			t.Errorf("torrent %d: BannedChanged = false on a ban", e.TorrentID)
		}
	}
}

func TestBulkUnbanEmitsAnUnbanEvent(t *testing.T) {
	bus := event.NewInMemoryBus()
	var events []*event.TorrentEditedEvent
	bus.Subscribe(event.TorrentEdited, func(_ context.Context, evt event.Event) error {
		events = append(events, evt.(*event.TorrentEditedEvent))
		return nil
	})

	torrents := threeTorrents() // torrent 3 starts banned
	svc := service.NewTorrentService(nil, torrents, newStubUserRepo(&model.User{ID: 1}),
		&stubStorage{}, service.TorrentServiceConfig{AnnounceURL: "http://localhost/announce"}, bus, nil)

	w := httptest.NewRecorder()
	NewTorrentAdminHandler(svc).HandleBulkAction(w, bulkRequest(t, "unban", 3))

	if len(events) != 1 {
		t.Fatalf("published %d events, want 1", len(events))
	}
	if events[0].BannedChanged == nil || *events[0].BannedChanged {
		t.Errorf("BannedChanged = %v, want a recorded unban", events[0].BannedChanged)
	}
}

// Re-banning an already-banned torrent changed nothing, so it must not be logged as
// a ban — an audit trail that records non-events is one nobody trusts.
func TestBulkBanOnAnAlreadyBannedTorrentRecordsNoBanChange(t *testing.T) {
	bus := event.NewInMemoryBus()
	var events []*event.TorrentEditedEvent
	bus.Subscribe(event.TorrentEdited, func(_ context.Context, evt event.Event) error {
		events = append(events, evt.(*event.TorrentEditedEvent))
		return nil
	})

	torrents := threeTorrents() // torrent 3 is already banned
	svc := service.NewTorrentService(nil, torrents, newStubUserRepo(&model.User{ID: 1}),
		&stubStorage{}, service.TorrentServiceConfig{AnnounceURL: "http://localhost/announce"}, bus, nil)

	w := httptest.NewRecorder()
	NewTorrentAdminHandler(svc).HandleBulkAction(w, bulkRequest(t, "ban", 3))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if len(events) != 1 {
		t.Fatalf("published %d events, want 1", len(events))
	}
	if events[0].BannedChanged != nil {
		t.Errorf("BannedChanged = %v, want nil — nothing changed", *events[0].BannedChanged)
	}
}

// A lookup that failed is not a torrent that is missing. Reporting a saturated
// connection pool as "not found" tells a moderator their ids were stale, and they
// walk away from torrents that are live and un-banned.
func TestBulkActionDistinguishesAFailedLookupFromAMissingTorrent(t *testing.T) {
	torrents := threeTorrents()
	torrents.getErr = errStub
	h := newTorrentAdminHandler(torrents, &stubStorage{}, newStubUserRepo(&model.User{ID: 1}))

	w := httptest.NewRecorder()
	h.HandleBulkAction(w, bulkRequest(t, "ban", 1, 2))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	statuses := bulkStatuses(t, w)
	for _, id := range []int64{1, 2} {
		if statuses[id] == service.BulkStatusNotFound {
			t.Errorf("torrent %d reported as not_found when the lookup itself failed", id)
		}
		if statuses[id] != service.BulkStatusError {
			t.Errorf("torrent %d reported %q, want %q", id, statuses[id], service.BulkStatusError)
		}
	}
}

// A cancelled request must stop rather than work through the remaining ids, each of
// which would fail for the cancellation and be reported as though the torrent were
// the problem.
func TestBulkActionStopsOnACancelledRequest(t *testing.T) {
	torrents := threeTorrents()
	h := newTorrentAdminHandler(torrents, &stubStorage{}, newStubUserRepo(&model.User{ID: 1}))

	req := bulkRequest(t, "ban", 1, 2, 3)
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h.HandleBulkAction(w, req)

	if len(torrents.updated) != 0 {
		t.Errorf("persisted %d torrents on a cancelled request", len(torrents.updated))
	}
}

// The route is admin-gated, but the service must refuse a non-staff caller too:
// DeleteTorrent alone authorises owner-or-staff, so without a gate here a member
// could delete their own uploads through a moderation entry point.
func TestBulkModerateRefusesANonStaffCaller(t *testing.T) {
	for _, action := range []service.BulkAction{
		service.BulkActionBan, service.BulkActionUnban, service.BulkActionDelete,
	} {
		torrents := threeTorrents()
		svc := service.NewTorrentService(nil, torrents, newStubUserRepo(&model.User{ID: 7}),
			&stubStorage{}, service.TorrentServiceConfig{AnnounceURL: "http://localhost/announce"},
			event.NewInMemoryBus(), nil)

		// A plain member who happens to own every torrent in the list.
		_, err := svc.BulkModerate(context.Background(), action,
			[]int64{1, 2, 3}, 7, model.Permissions{Level: 20})

		if !errors.Is(err, service.ErrForbidden) {
			t.Errorf("%s: err = %v, want ErrForbidden", action, err)
		}
		if len(torrents.updated) != 0 || len(torrents.deleted) != 0 {
			t.Errorf("%s: a non-staff caller changed something", action)
		}
	}
}

// A moderator is staff, so the batch is accepted — and then ban is refused per
// torrent by EditTorrent's admin-only check. Both gates, each doing its own job.
func TestBulkModerateAcceptsAModeratorAndStillRefusesTheBan(t *testing.T) {
	torrents := threeTorrents()
	svc := service.NewTorrentService(nil, torrents, newStubUserRepo(&model.User{ID: 9}),
		&stubStorage{}, service.TorrentServiceConfig{AnnounceURL: "http://localhost/announce"},
		event.NewInMemoryBus(), nil)

	results, err := svc.BulkModerate(context.Background(), service.BulkActionBan,
		[]int64{1}, 9, model.Permissions{Level: 50, IsModerator: true})
	if err != nil {
		t.Fatalf("BulkModerate: %v", err)
	}
	if len(results) != 1 || results[0].Status != service.BulkStatusForbidden {
		t.Errorf("results = %+v, want one forbidden entry", results)
	}
}

// --- pagination echo ---

// The response echoes what the listing actually used. It previously echoed the raw
// request, so a call with no params answered page 0 of 0 while serving page 1 of 25
// — and an integrator computing ceil(total / per_page) divided by zero.
func TestAdminListTorrentsEchoesEffectivePagination(t *testing.T) {
	for _, tc := range []struct {
		query           string
		wantPage        int
		wantPerPage     int
		wantOptsPerPage int
	}{
		{"", 1, defaultAdminPerPage, defaultAdminPerPage},
		{"?page=3&per_page=10", 3, 10, 10},
		{"?per_page=500", 1, maxAdminPerPage, maxAdminPerPage},
	} {
		d := newAdminDeps()
		h := d.handler()

		req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/torrents"+tc.query, nil), 1)
		w := httptest.NewRecorder()
		h.HandleListTorrents(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("%q: status = %d, want 200; body: %s", tc.query, w.Code, w.Body.String())
		}
		body := decodeBody(t, w)
		if got := int(body["page"].(float64)); got != tc.wantPage {
			t.Errorf("%q: page = %d, want %d", tc.query, got, tc.wantPage)
		}
		if got := int(body["per_page"].(float64)); got != tc.wantPerPage {
			t.Errorf("%q: per_page = %d, want %d", tc.query, got, tc.wantPerPage)
		}
		// And the value echoed is the value the query ran with, not a separate guess.
		if d.torrents.lastOpts.PerPage != tc.wantOptsPerPage {
			t.Errorf("%q: the listing used per_page %d but reported %d",
				tc.query, d.torrents.lastOpts.PerPage, tc.wantPerPage)
		}
	}
}

func TestAdminListTorrentsRejectsUnusablePaginationAndUploader(t *testing.T) {
	for _, query := range []string{
		"?page=abc", "?page=0", "?page=-1",
		"?per_page=abc", "?per_page=0",
		"?uploader_id=abc", "?uploader_id=0",
	} {
		d := newAdminDeps()
		h := d.handler()

		req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/torrents"+query, nil), 1)
		w := httptest.NewRecorder()
		h.HandleListTorrents(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("%q: status = %d, want 400 — a dropped filter widens the result set",
				query, w.Code)
		}
	}
}

// The inverse escape hatch. Without it a torrent whose name is 40 hex characters —
// a checksum, a git SHA — could never be found by name.
func TestAdminListTorrentsCanForceANameSearch(t *testing.T) {
	d := newAdminDeps()
	h := d.handler()

	req := adminAuthed(httptest.NewRequest(http.MethodGet,
		"/api/v1/admin/torrents?name="+sampleHashHex, nil), 1)
	w := httptest.NewRecorder()
	h.HandleListTorrents(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	opts := d.torrents.lastOpts
	if opts.Search != sampleHashHex {
		t.Errorf("Search = %q, want the hex string searched as a name", opts.Search)
	}
	if opts.InfoHash != nil {
		t.Errorf("InfoHash = %x, want it cleared when name= is given", opts.InfoHash)
	}
}
