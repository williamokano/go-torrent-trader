package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// Coverage for the admin endpoints that the router-level tests in admin_test.go
// do not reach: user detail, mod notes, quick ban, torrent search and delete.

// --- stubs ------------------------------------------------------------------

type stubModNoteRepo struct {
	byID    map[int64]*model.ModNote
	byUser  []model.ModNote
	created []*model.ModNote
	deleted []int64

	createErr error
	listErr   error
	deleteErr error

	nextID int64
}

func newStubModNoteRepo() *stubModNoteRepo {
	return &stubModNoteRepo{byID: map[int64]*model.ModNote{}, nextID: 1}
}

func (s *stubModNoteRepo) Create(_ context.Context, note *model.ModNote) error {
	if s.createErr != nil {
		return s.createErr
	}
	note.ID = s.nextID
	s.nextID++
	s.created = append(s.created, note)
	stored := *note
	s.byID[note.ID] = &stored
	return nil
}
func (s *stubModNoteRepo) GetByID(_ context.Context, id int64) (*model.ModNote, error) {
	n, ok := s.byID[id]
	if !ok {
		return nil, errors.New("mod note not found")
	}
	return n, nil
}
func (s *stubModNoteRepo) ListByUser(_ context.Context, _ int64) ([]model.ModNote, error) {
	return s.byUser, s.listErr
}
func (s *stubModNoteRepo) Delete(_ context.Context, id int64) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.deleted = append(s.deleted, id)
	return nil
}

type stubTorrentRepo struct {
	torrents map[int64]*model.Torrent
	list     []model.Torrent
	total    int64
	lastOpts repository.ListTorrentsOptions
	deleted  []int64
	// updated records what Update was asked to persist. GetByID hands out the
	// stored pointer, so a service that mutates it in place would leave the map
	// looking correct without ever writing — asserting on this instead is what
	// makes a persistence test about persistence.
	updated []model.Torrent

	listErr   error
	deleteErr error
	updateErr error
	// getErr stands in for a lookup that failed rather than found nothing — a pool
	// timeout, a dropped connection. Kept separate from a miss because the bulk
	// endpoint reports the two differently to the moderator.
	getErr error
}

func (s *stubTorrentRepo) GetByID(_ context.Context, id int64) (*model.Torrent, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	t, ok := s.torrents[id]
	if !ok {
		// sql.ErrNoRows, wrapped the way postgres.TorrentRepo wraps it. A bare
		// errors.New here would be indistinguishable from a connection failure and
		// would hide the branch that tells them apart — which is exactly the
		// distinction the bulk endpoint reports to a moderator.
		return nil, fmt.Errorf("scanning torrent %d: %w", id, sql.ErrNoRows)
	}
	return t, nil
}
func (s *stubTorrentRepo) GetByInfoHash(_ context.Context, _ []byte) (*model.Torrent, error) {
	return nil, errors.New("torrent not found")
}
func (s *stubTorrentRepo) List(_ context.Context, opts repository.ListTorrentsOptions) ([]model.Torrent, int64, error) {
	s.lastOpts = opts
	if s.listErr != nil {
		return nil, 0, s.listErr
	}
	return s.list, s.total, nil
}
func (s *stubTorrentRepo) ListByUploader(_ context.Context, _ int64, _ int) ([]model.Torrent, error) {
	return nil, nil
}
func (s *stubTorrentRepo) Create(_ context.Context, _ *model.Torrent) error { return nil }
func (s *stubTorrentRepo) Update(_ context.Context, t *model.Torrent) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	s.updated = append(s.updated, *t) // copy: the caller keeps hold of the pointer
	return nil
}
func (s *stubTorrentRepo) Delete(_ context.Context, id int64) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.deleted = append(s.deleted, id)
	return nil
}
func (s *stubTorrentRepo) IncrementSeeders(_ context.Context, _ int64, _ int) error  { return nil }
func (s *stubTorrentRepo) IncrementLeechers(_ context.Context, _ int64, _ int) error { return nil }
func (s *stubTorrentRepo) IncrementTimesCompleted(_ context.Context, _ int64) error  { return nil }

type stubStorage struct {
	deleted []string
}

func (s *stubStorage) Put(_ context.Context, _ string, _ io.Reader) error { return nil }
func (s *stubStorage) Get(_ context.Context, _ string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (s *stubStorage) Delete(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	return nil
}
func (s *stubStorage) Exists(_ context.Context, _ string) (bool, error) { return true, nil }
func (s *stubStorage) URL(_ context.Context, key string) (string, error) {
	return "/files/" + key, nil
}

type stubBanRepo struct {
	emailBans []model.BannedEmail
	ipBans    []model.BannedIP
	deleted   []int64

	createErr error
	listErr   error
	deleteErr error
}

func (s *stubBanRepo) CreateEmailBan(_ context.Context, ban *model.BannedEmail) error {
	if s.createErr != nil {
		return s.createErr
	}
	s.emailBans = append(s.emailBans, *ban)
	return nil
}
func (s *stubBanRepo) DeleteEmailBan(_ context.Context, id int64) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.deleted = append(s.deleted, id)
	return nil
}
func (s *stubBanRepo) ListEmailBans(_ context.Context) ([]model.BannedEmail, error) {
	return s.emailBans, s.listErr
}
func (s *stubBanRepo) IsEmailBanned(_ context.Context, _ string) (bool, error) { return false, nil }
func (s *stubBanRepo) CreateIPBan(_ context.Context, ban *model.BannedIP) error {
	if s.createErr != nil {
		return s.createErr
	}
	s.ipBans = append(s.ipBans, *ban)
	return nil
}
func (s *stubBanRepo) DeleteIPBan(_ context.Context, id int64) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.deleted = append(s.deleted, id)
	return nil
}
func (s *stubBanRepo) ListIPBans(_ context.Context) ([]model.BannedIP, error) {
	return s.ipBans, s.listErr
}
func (s *stubBanRepo) IsIPBanned(_ context.Context, _ string) (bool, error) { return false, nil }

// --- harness ----------------------------------------------------------------

type adminDeps struct {
	users    *stubUserRepo
	groups   *stubGroupRepo
	modNotes *stubModNoteRepo
	torrents *stubTorrentRepo
	warnings *stubWarningRepo
	messages *stubMessageRepo
	bans     *stubBanRepo
	svc      *service.AdminService
}

// newAdminDeps wires an admin (id 1, group 1 level 100) and a member (id 7,
// group 5 level 20).
func newAdminDeps() *adminDeps {
	ip := "203.0.113.7"
	users := newStubUserRepo(
		&model.User{ID: 1, Username: "admin", Email: "admin@site.test", GroupID: 1, Enabled: true},
		&model.User{ID: 7, Username: "member", Email: "member@example.org", GroupID: 5, Enabled: true, IP: &ip,
			Uploaded: 100, Downloaded: 50},
	)
	groups := &stubGroupRepo{groups: map[int64]*model.Group{
		1: {ID: 1, Name: "Admin", Level: 100, IsAdmin: true},
		5: {ID: 5, Name: "User", Level: 20},
	}}
	d := &adminDeps{
		users:    users,
		groups:   groups,
		modNotes: newStubModNoteRepo(),
		torrents: &stubTorrentRepo{torrents: map[int64]*model.Torrent{}},
		warnings: newStubWarningRepo(),
		messages: newStubMessageRepo(),
		bans:     &stubBanRepo{},
	}

	bus := event.NewInMemoryBus()
	svc := service.NewAdminService(users, groups, bus)
	svc.SetModNoteRepo(d.modNotes)
	svc.SetTorrentRepo(d.torrents)
	svc.SetWarningRepo(d.warnings)
	svc.SetMessageRepo(d.messages)
	svc.SetBanService(service.NewBanService(d.bans, bus))
	d.svc = svc
	return d
}

func (d *adminDeps) handler() *AdminHandler { return NewAdminHandler(d.svc, nil) }

// --- user detail ------------------------------------------------------------

func TestGetUserDetailRejectsInvalidID(t *testing.T) {
	h := newAdminDeps().handler()

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/abc", nil), 1)
	req = withURLParam(req, "id", "abc")
	w := httptest.NewRecorder()
	h.HandleGetUserDetail(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestGetUserDetailReturns404WhenMissing(t *testing.T) {
	h := newAdminDeps().handler()

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/999", nil), 1)
	req = withURLParam(req, "id", "999")
	w := httptest.NewRecorder()
	h.HandleGetUserDetail(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestGetUserDetailAggregatesUploadsWarningsAndNotes(t *testing.T) {
	d := newAdminDeps()
	d.torrents.list = []model.Torrent{{ID: 3, Name: "A movie", Size: 1024, CreatedAt: time.Now()}}
	d.warnings.active = 2
	d.modNotes.byUser = []model.ModNote{
		{ID: 1, UserID: 7, AuthorID: 1, Note: "watch this one", AuthorUsername: "admin"},
	}
	h := d.handler()

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/7", nil), 1)
	req = withURLParam(req, "id", "7")
	w := httptest.NewRecorder()
	h.HandleGetUserDetail(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	user := decodeBody(t, w)["user"].(map[string]interface{})
	if user["username"] != "member" || user["group_name"] != "User" {
		t.Errorf("user = %v, want the member with their group name resolved", user)
	}
	if user["ratio"] != float64(2) { // 100 uploaded / 50 downloaded
		t.Errorf("ratio = %v, want 2", user["ratio"])
	}
	if user["warnings_count"] != float64(2) {
		t.Errorf("warnings_count = %v, want 2", user["warnings_count"])
	}
	if uploads, ok := user["recent_uploads"].([]interface{}); !ok || len(uploads) != 1 {
		t.Errorf("recent_uploads = %v, want 1 item", user["recent_uploads"])
	}
	if notes, ok := user["mod_notes"].([]interface{}); !ok || len(notes) != 1 {
		t.Errorf("mod_notes = %v, want 1 item", user["mod_notes"])
	}
	// The admin view must include soft-hidden/banned torrents.
	if !d.torrents.lastOpts.IncludeHidden {
		t.Error("recent uploads were fetched without IncludeHidden — admins must see hidden torrents")
	}
}

// The detail view degrades gracefully: a failing side query must not 500 the
// whole page.
func TestGetUserDetailStillRendersWhenSideQueriesFail(t *testing.T) {
	d := newAdminDeps()
	d.torrents.listErr = errStub
	d.modNotes.listErr = errStub
	h := d.handler()

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/7", nil), 1)
	req = withURLParam(req, "id", "7")
	w := httptest.NewRecorder()
	h.HandleGetUserDetail(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	user := decodeBody(t, w)["user"].(map[string]interface{})
	if uploads, ok := user["recent_uploads"].([]interface{}); !ok || len(uploads) != 0 {
		t.Errorf("recent_uploads = %v, want an empty array", user["recent_uploads"])
	}
	if notes, ok := user["mod_notes"].([]interface{}); !ok || len(notes) != 0 {
		t.Errorf("mod_notes = %v, want an empty array", user["mod_notes"])
	}
}

// --- mod notes --------------------------------------------------------------

func TestCreateModNoteRequiresAuth(t *testing.T) {
	h := newAdminDeps().handler()

	req := withURLParam(httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/7/notes",
		strings.NewReader(`{"note":"x"}`)), "id", "7")
	w := httptest.NewRecorder()
	h.HandleCreateModNote(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestCreateModNoteRejectsInvalidIDAndBody(t *testing.T) {
	h := newAdminDeps().handler()

	cases := []struct{ name, id, body string }{
		{"non-numeric user id", "abc", `{"note":"x"}`},
		{"zero user id", "0", `{"note":"x"}`},
		{"malformed json", "7", `{`},
		{"empty note", "7", `{"note":""}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := adminAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/"+tc.id+"/notes",
				strings.NewReader(tc.body)), 1)
			req = withURLParam(req, "id", tc.id)
			w := httptest.NewRecorder()
			h.HandleCreateModNote(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", w.Code)
			}
		})
	}
}

func TestCreateModNoteReturns404ForUnknownUser(t *testing.T) {
	h := newAdminDeps().handler()

	req := adminAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/999/notes",
		strings.NewReader(`{"note":"x"}`)), 1)
	req = withURLParam(req, "id", "999")
	w := httptest.NewRecorder()
	h.HandleCreateModNote(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestCreateModNoteStoresNoteWithAuthor(t *testing.T) {
	d := newAdminDeps()
	h := d.handler()

	req := adminAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/7/notes",
		strings.NewReader(`{"note":"suspected cheater"}`)), 1)
	req = withURLParam(req, "id", "7")
	w := httptest.NewRecorder()
	h.HandleCreateModNote(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	if len(d.modNotes.created) != 1 {
		t.Fatalf("created %d notes, want 1", len(d.modNotes.created))
	}
	note := d.modNotes.created[0]
	if note.UserID != 7 || note.AuthorID != 1 || note.Note != "suspected cheater" {
		t.Errorf("note = %+v, want it attributed to the authenticated admin", note)
	}
	resp := decodeBody(t, w)["note"].(map[string]interface{})
	if resp["author_username"] != "admin" {
		t.Errorf("author_username = %v, want \"admin\"", resp["author_username"])
	}
}

func TestCreateModNoteSurfacesStoreFailure(t *testing.T) {
	d := newAdminDeps()
	d.modNotes.createErr = errStub
	h := d.handler()

	req := adminAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/7/notes",
		strings.NewReader(`{"note":"x"}`)), 1)
	req = withURLParam(req, "id", "7")
	w := httptest.NewRecorder()
	h.HandleCreateModNote(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestDeleteModNoteRequiresAuth(t *testing.T) {
	h := newAdminDeps().handler()

	req := withURLParam(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/notes/1", nil), "id", "1")
	w := httptest.NewRecorder()
	h.HandleDeleteModNote(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestDeleteModNoteRejectsInvalidID(t *testing.T) {
	h := newAdminDeps().handler()

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/notes/0", nil), 1)
	req = withURLParam(req, "id", "0")
	w := httptest.NewRecorder()
	h.HandleDeleteModNote(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestDeleteModNoteReturns404WhenMissing(t *testing.T) {
	h := newAdminDeps().handler()

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/notes/9", nil), 1)
	req = withURLParam(req, "id", "9")
	w := httptest.NewRecorder()
	h.HandleDeleteModNote(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// A moderator may only delete their own notes; an admin may delete anyone's.
func TestDeleteModNoteForbiddenForSomeoneElsesNote(t *testing.T) {
	d := newAdminDeps()
	d.modNotes.byID[9] = &model.ModNote{ID: 9, UserID: 7, AuthorID: 1} // written by the admin
	h := d.handler()

	req := staffAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/notes/9", nil), 2) // a moderator
	req = withURLParam(req, "id", "9")
	w := httptest.NewRecorder()
	h.HandleDeleteModNote(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 — a moderator may only delete their own notes", w.Code)
	}
	if len(d.modNotes.deleted) != 0 {
		t.Error("the note was deleted anyway")
	}
}

func TestDeleteModNoteAdminCanDeleteAnyNote(t *testing.T) {
	d := newAdminDeps()
	d.modNotes.byID[9] = &model.ModNote{ID: 9, UserID: 7, AuthorID: 2} // written by a moderator
	h := d.handler()

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/notes/9", nil), 1)
	req = withURLParam(req, "id", "9")
	w := httptest.NewRecorder()
	h.HandleDeleteModNote(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if len(d.modNotes.deleted) != 1 || d.modNotes.deleted[0] != 9 {
		t.Errorf("deleted = %v, want [9]", d.modNotes.deleted)
	}
}

func TestDeleteModNoteAuthorCanDeleteTheirOwnNote(t *testing.T) {
	d := newAdminDeps()
	d.modNotes.byID[9] = &model.ModNote{ID: 9, UserID: 7, AuthorID: 2}
	h := d.handler()

	req := staffAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/notes/9", nil), 2)
	req = withURLParam(req, "id", "9")
	w := httptest.NewRecorder()
	h.HandleDeleteModNote(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

// --- quick ban --------------------------------------------------------------

func TestQuickBanRequiresAuth(t *testing.T) {
	h := newAdminDeps().handler()

	req := withURLParam(httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/7/ban",
		strings.NewReader(`{"reason":"spam"}`)), "id", "7")
	w := httptest.NewRecorder()
	h.HandleQuickBan(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestQuickBanValidatesInput(t *testing.T) {
	cases := []struct {
		name, id, body string
	}{
		{"non-numeric user id", "abc", `{"reason":"spam"}`},
		{"malformed json", "7", `{`},
		{"missing reason", "7", `{}`},
		{"non-positive duration", "7", `{"reason":"spam","duration_days":0}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := newAdminDeps()
			h := d.handler()

			req := adminAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/"+tc.id+"/ban",
				strings.NewReader(tc.body)), 1)
			req = withURLParam(req, "id", tc.id)
			w := httptest.NewRecorder()
			h.HandleQuickBan(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", w.Code)
			}
			if !d.users.users[7].Enabled {
				t.Error("the target was disabled despite the invalid request")
			}
		})
	}
}

func TestQuickBanRefusesSelfBan(t *testing.T) {
	d := newAdminDeps()
	h := d.handler()

	req := adminAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/1/ban",
		strings.NewReader(`{"reason":"oops"}`)), 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleQuickBan(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 — an admin must not be able to ban themselves", w.Code)
	}
	if !d.users.users[1].Enabled {
		t.Error("the admin disabled their own account")
	}
}

// A moderator must not be able to ban someone at or above their own level.
func TestQuickBanForbiddenAgainstHigherRankedUser(t *testing.T) {
	d := newAdminDeps()
	// User 2 is a moderator (level 20, same as the member's group).
	d.groups.groups[2] = &model.Group{ID: 2, Name: "Mod", Level: 20, IsModerator: true}
	d.users.users[2] = &model.User{ID: 2, Username: "mod", GroupID: 2, Enabled: true}
	h := d.handler()

	req := staffAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/7/ban",
		strings.NewReader(`{"reason":"spam"}`)), 2)
	req = withURLParam(req, "id", "7")
	w := httptest.NewRecorder()
	h.HandleQuickBan(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 — the actor does not outrank the target", w.Code)
	}
	if !d.users.users[7].Enabled {
		t.Error("the target was banned despite the level check failing")
	}
}

func TestQuickBanDisablesUserAndSendsPM(t *testing.T) {
	d := newAdminDeps()
	h := d.handler()

	req := adminAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/7/ban",
		strings.NewReader(`{"reason":"cheating"}`)), 1)
	req = withURLParam(req, "id", "7")
	w := httptest.NewRecorder()
	h.HandleQuickBan(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if d.users.users[7].Enabled {
		t.Error("the target is still enabled after a quick ban")
	}
	if len(d.messages.created) != 1 || d.messages.created[0].ReceiverID != 7 {
		t.Errorf("PMs = %+v, want one addressed to the banned user", d.messages.created)
	}
	if len(d.warnings.created) != 1 || d.warnings.created[0].Status != model.WarningStatusEscalated {
		t.Errorf("warnings = %+v, want one escalated audit record", d.warnings.created)
	}

	body := decodeBody(t, w)
	if body["banned"] != true {
		t.Errorf("banned = %v, want true", body["banned"])
	}
	if body["ip_banned"] != false || body["email_banned"] != false {
		t.Errorf("ip_banned/email_banned = %v/%v, want false when not requested", body["ip_banned"], body["email_banned"])
	}
}

func TestQuickBanWithDurationSetsDisabledUntil(t *testing.T) {
	d := newAdminDeps()
	h := d.handler()

	req := adminAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/7/ban",
		strings.NewReader(`{"reason":"cheating","duration_days":7}`)), 1)
	req = withURLParam(req, "id", "7")
	w := httptest.NewRecorder()
	h.HandleQuickBan(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	until := d.users.users[7].DisabledUntil
	if until == nil {
		t.Fatal("disabled_until = nil, want a timestamp roughly 7 days out")
	}
	if d := time.Until(*until); d < 6*24*time.Hour || d > 8*24*time.Hour {
		t.Errorf("disabled_until is %v away, want roughly 7 days", d)
	}
}

func TestQuickBanCanAlsoBanIPAndEmailDomain(t *testing.T) {
	d := newAdminDeps()
	h := d.handler()

	req := adminAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/7/ban",
		strings.NewReader(`{"reason":"cheating","ban_ip":true,"ban_email":true}`)), 1)
	req = withURLParam(req, "id", "7")
	w := httptest.NewRecorder()
	h.HandleQuickBan(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if len(d.bans.ipBans) != 1 || d.bans.ipBans[0].IPRange != "203.0.113.7" {
		t.Errorf("ip bans = %+v, want the target's IP", d.bans.ipBans)
	}
	if len(d.bans.emailBans) != 1 || d.bans.emailBans[0].Pattern != "*@example.org" {
		t.Errorf("email bans = %+v, want a domain pattern", d.bans.emailBans)
	}
	body := decodeBody(t, w)
	if body["ip_banned"] != true || body["email_banned"] != true {
		t.Errorf("ip_banned/email_banned = %v/%v, want both true", body["ip_banned"], body["email_banned"])
	}
	if body["email_pattern"] != "*@example.org" {
		t.Errorf("email_pattern = %v, want *@example.org", body["email_pattern"])
	}
}

// Banning a whole free-mail domain would lock out thousands of legitimate users.
func TestQuickBanRefusesToBanCommonEmailProviders(t *testing.T) {
	d := newAdminDeps()
	d.users.users[7].Email = "cheater@gmail.com"
	h := d.handler()

	req := adminAuthed(httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/7/ban",
		strings.NewReader(`{"reason":"cheating","ban_email":true}`)), 1)
	req = withURLParam(req, "id", "7")
	w := httptest.NewRecorder()
	h.HandleQuickBan(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if len(d.bans.emailBans) != 0 {
		t.Errorf("gmail.com was domain-banned: %+v", d.bans.emailBans)
	}
	// The check must run before anything is mutated.
	if !d.users.users[7].Enabled {
		t.Error("the user was disabled even though the request was rejected")
	}
}

// --- admin torrent list -----------------------------------------------------

func TestAdminListTorrentsReturnsItems(t *testing.T) {
	d := newAdminDeps()
	d.torrents.list = []model.Torrent{
		{ID: 3, Name: "A movie", Size: 2048, Seeders: 5, Leechers: 1, UploaderID: 7,
			UploaderName: "member", Banned: true, CreatedAt: time.Now()},
	}
	d.torrents.total = 1
	h := d.handler()

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/torrents?search=movie&uploader_id=7&page=2&per_page=10", nil), 1)
	w := httptest.NewRecorder()
	h.HandleListTorrents(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	opts := d.torrents.lastOpts
	if opts.Search != "movie" || opts.Page != 2 || opts.PerPage != 10 {
		t.Errorf("opts = %+v, want search=movie page=2 per_page=10", opts)
	}
	if opts.UploaderID == nil || *opts.UploaderID != 7 {
		t.Errorf("uploader_id = %v, want 7", opts.UploaderID)
	}
	if !opts.IncludeHidden {
		t.Error("the admin listing must include hidden and banned torrents")
	}

	items := decodeBody(t, w)["torrents"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("torrents = %v, want 1 item", items)
	}
	torrent := items[0].(map[string]interface{})
	for _, key := range []string{"id", "name", "size", "seeders", "leechers", "uploader_id", "uploader", "banned", "created_at"} {
		if _, present := torrent[key]; !present {
			t.Errorf("torrent response is missing key %q", key)
		}
	}
	if torrent["banned"] != true {
		t.Error("banned = false, want the banned flag surfaced to admins")
	}
}

func TestAdminListTorrentsSurfacesStoreFailure(t *testing.T) {
	d := newAdminDeps()
	d.torrents.listErr = errStub
	h := d.handler()

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/torrents", nil), 1)
	w := httptest.NewRecorder()
	h.HandleListTorrents(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// --- admin torrent delete ---------------------------------------------------

func newTorrentAdminHandler(torrents *stubTorrentRepo, store *stubStorage, users *stubUserRepo) *TorrentAdminHandler {
	svc := service.NewTorrentService(nil, torrents, users, store,
		service.TorrentServiceConfig{AnnounceURL: "http://localhost/announce"}, event.NewInMemoryBus(), nil)
	return NewTorrentAdminHandler(svc)
}

func TestAdminDeleteTorrentRequiresAuth(t *testing.T) {
	h := newTorrentAdminHandler(&stubTorrentRepo{torrents: map[int64]*model.Torrent{}}, &stubStorage{}, newStubUserRepo())

	req := withURLParam(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/torrents/3", nil), "id", "3")
	w := httptest.NewRecorder()
	h.HandleDeleteTorrent(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestAdminDeleteTorrentRejectsInvalidID(t *testing.T) {
	h := newTorrentAdminHandler(&stubTorrentRepo{torrents: map[int64]*model.Torrent{}}, &stubStorage{}, newStubUserRepo())

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/torrents/0", nil), 1)
	req = withURLParam(req, "id", "0")
	w := httptest.NewRecorder()
	h.HandleDeleteTorrent(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestAdminDeleteTorrentReturns404WhenMissing(t *testing.T) {
	h := newTorrentAdminHandler(&stubTorrentRepo{torrents: map[int64]*model.Torrent{}}, &stubStorage{}, newStubUserRepo())

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/torrents/3", nil), 1)
	req = withURLParam(req, "id", "3")
	w := httptest.NewRecorder()
	h.HandleDeleteTorrent(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestAdminDeleteTorrentRemovesRowAndFile(t *testing.T) {
	torrents := &stubTorrentRepo{torrents: map[int64]*model.Torrent{
		3: {ID: 3, Name: "A movie", UploaderID: 7},
	}}
	store := &stubStorage{}
	users := newStubUserRepo(&model.User{ID: 1, Username: "admin"})
	h := newTorrentAdminHandler(torrents, store, users)

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/torrents/3", nil), 1)
	req = withURLParam(req, "id", "3")
	w := httptest.NewRecorder()
	h.HandleDeleteTorrent(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if len(torrents.deleted) != 1 || torrents.deleted[0] != 3 {
		t.Errorf("deleted rows = %v, want [3]", torrents.deleted)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "torrents/3.torrent" {
		t.Errorf("deleted files = %v, want the .torrent blob removed too", store.deleted)
	}
}

func TestAdminDeleteTorrentSurfacesStoreFailure(t *testing.T) {
	torrents := &stubTorrentRepo{
		torrents:  map[int64]*model.Torrent{3: {ID: 3, UploaderID: 7}},
		deleteErr: errStub,
	}
	h := newTorrentAdminHandler(torrents, &stubStorage{}, newStubUserRepo(&model.User{ID: 1}))

	req := adminAuthed(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/torrents/3", nil), 1)
	req = withURLParam(req, "id", "3")
	w := httptest.NewRecorder()
	h.HandleDeleteTorrent(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 — a failed delete must not report success", w.Code)
	}
}
