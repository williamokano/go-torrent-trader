package handler

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// --- cheat flag repository stub ---------------------------------------------

type stubCheatFlagRepo struct {
	flags     []model.CheatFlag
	total     int64
	lastOpts  repository.ListCheatFlagsOptions
	dismissed []int64
	dismisser []int64

	listErr    error
	dismissErr error
}

func (s *stubCheatFlagRepo) Create(_ context.Context, _ *model.CheatFlag) error { return nil }
func (s *stubCheatFlagRepo) GetByID(_ context.Context, _ int64) (*model.CheatFlag, error) {
	return nil, sql.ErrNoRows
}
func (s *stubCheatFlagRepo) List(_ context.Context, opts repository.ListCheatFlagsOptions) ([]model.CheatFlag, int64, error) {
	s.lastOpts = opts
	if s.listErr != nil {
		return nil, 0, s.listErr
	}
	return s.flags, s.total, nil
}
func (s *stubCheatFlagRepo) Dismiss(_ context.Context, id, dismissedBy int64) error {
	if s.dismissErr != nil {
		return s.dismissErr
	}
	s.dismissed = append(s.dismissed, id)
	s.dismisser = append(s.dismisser, dismissedBy)
	return nil
}
func (s *stubCheatFlagRepo) HasRecentUndismissed(_ context.Context, _, _ int64, _ string, _ int) (bool, error) {
	return false, nil
}

// --- list -------------------------------------------------------------------

func TestListCheatFlagsReturnsItems(t *testing.T) {
	flags := &stubCheatFlagRepo{
		flags: []model.CheatFlag{{ID: 1, UserID: 7, FlagType: model.CheatFlagLeftMismatch}},
		total: 1,
	}
	h := NewCheatFlagHandler(flags)

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/cheat-flags", nil), 1)
	w := httptest.NewRecorder()
	h.HandleListCheatFlags(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := decodeBody(t, w)
	items, ok := body["cheat_flags"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("cheat_flags = %v, want 1 item", body["cheat_flags"])
	}
	if body["total"].(float64) != 1 {
		t.Errorf("total = %v, want 1", body["total"])
	}
	if body["page"].(float64) != 1 || body["per_page"].(float64) != 25 {
		t.Errorf("page/per_page = %v/%v, want the defaults 1/25", body["page"], body["per_page"])
	}
}

func TestListCheatFlagsAppliesFilters(t *testing.T) {
	flags := &stubCheatFlagRepo{}
	h := NewCheatFlagHandler(flags)

	url := "/api/v1/admin/cheat-flags?user_id=7&flag_type=" + model.CheatFlagImpossibleUploadSpeed + "&dismissed=false"
	req := adminAuthed(httptest.NewRequest(http.MethodGet, url, nil), 1)
	w := httptest.NewRecorder()
	h.HandleListCheatFlags(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	opts := flags.lastOpts
	if opts.UserID == nil || *opts.UserID != 7 {
		t.Errorf("opts.UserID = %v, want 7", opts.UserID)
	}
	if opts.FlagType == nil || *opts.FlagType != model.CheatFlagImpossibleUploadSpeed {
		t.Errorf("opts.FlagType = %v, want %q", opts.FlagType, model.CheatFlagImpossibleUploadSpeed)
	}
	if opts.Dismissed == nil || *opts.Dismissed {
		t.Errorf("opts.Dismissed = %v, want false", opts.Dismissed)
	}
}

// An unknown flag_type is rejected rather than passed into the query.
func TestListCheatFlagsRejectsUnknownFlagType(t *testing.T) {
	flags := &stubCheatFlagRepo{}
	h := NewCheatFlagHandler(flags)

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/cheat-flags?flag_type=made_up", nil), 1)
	w := httptest.NewRecorder()
	h.HandleListCheatFlags(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an unknown flag_type", w.Code)
	}
	if flags.lastOpts.FlagType != nil {
		t.Error("the invalid flag_type reached the repository")
	}
}

func TestListCheatFlagsNormalisesPagination(t *testing.T) {
	cases := []struct {
		name        string
		query       string
		wantPage    int
		wantPerPage int
	}{
		{"defaults", "", 1, 25},
		{"explicit", "?page=4&per_page=50", 4, 50},
		{"non-positive falls back", "?page=0&per_page=-5", 1, 25},
		{"per_page is capped at 100", "?per_page=1000", 1, 100},
		{"garbage falls back", "?page=abc&per_page=abc", 1, 25},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			flags := &stubCheatFlagRepo{}
			h := NewCheatFlagHandler(flags)

			req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/cheat-flags"+tc.query, nil), 1)
			w := httptest.NewRecorder()
			h.HandleListCheatFlags(w, req)

			if flags.lastOpts.Page != tc.wantPage || flags.lastOpts.PerPage != tc.wantPerPage {
				t.Errorf("page/per_page = %d/%d, want %d/%d",
					flags.lastOpts.Page, flags.lastOpts.PerPage, tc.wantPage, tc.wantPerPage)
			}
		})
	}
}

func TestListCheatFlagsSurfacesStoreFailure(t *testing.T) {
	h := NewCheatFlagHandler(&stubCheatFlagRepo{listErr: errStub})

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/admin/cheat-flags", nil), 1)
	w := httptest.NewRecorder()
	h.HandleListCheatFlags(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// --- dismiss ----------------------------------------------------------------

func TestDismissCheatFlagRequiresAuth(t *testing.T) {
	h := NewCheatFlagHandler(&stubCheatFlagRepo{})

	req := withURLParam(httptest.NewRequest(http.MethodPut, "/api/v1/admin/cheat-flags/1/dismiss", nil), "id", "1")
	w := httptest.NewRecorder()
	h.HandleDismissCheatFlag(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestDismissCheatFlagRejectsInvalidID(t *testing.T) {
	h := NewCheatFlagHandler(&stubCheatFlagRepo{})

	for _, id := range []string{"abc", "0", "-2"} {
		req := adminAuthed(httptest.NewRequest(http.MethodPut, "/api/v1/admin/cheat-flags/"+id+"/dismiss", nil), 1)
		req = withURLParam(req, "id", id)
		w := httptest.NewRecorder()
		h.HandleDismissCheatFlag(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("id %q: status = %d, want 400", id, w.Code)
		}
	}
}

func TestDismissCheatFlagRecordsTheDismisser(t *testing.T) {
	flags := &stubCheatFlagRepo{}
	h := NewCheatFlagHandler(flags)

	req := adminAuthed(httptest.NewRequest(http.MethodPut, "/api/v1/admin/cheat-flags/8/dismiss", nil), 1)
	req = withURLParam(req, "id", "8")
	w := httptest.NewRecorder()
	h.HandleDismissCheatFlag(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if len(flags.dismissed) != 1 || flags.dismissed[0] != 8 {
		t.Errorf("dismissed = %v, want [8]", flags.dismissed)
	}
	if len(flags.dismisser) != 1 || flags.dismisser[0] != 1 {
		t.Errorf("dismissed_by = %v, want the authenticated actor [1]", flags.dismisser)
	}
	if decodeBody(t, w)["message"] != "cheat flag dismissed" {
		t.Error("response is missing the confirmation message")
	}
}

func TestDismissCheatFlagReturns404WhenMissingOrAlreadyDismissed(t *testing.T) {
	h := NewCheatFlagHandler(&stubCheatFlagRepo{dismissErr: sql.ErrNoRows})

	req := adminAuthed(httptest.NewRequest(http.MethodPut, "/api/v1/admin/cheat-flags/8/dismiss", nil), 1)
	req = withURLParam(req, "id", "8")
	w := httptest.NewRecorder()
	h.HandleDismissCheatFlag(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestDismissCheatFlagSurfacesStoreFailure(t *testing.T) {
	h := NewCheatFlagHandler(&stubCheatFlagRepo{dismissErr: errStub})

	req := adminAuthed(httptest.NewRequest(http.MethodPut, "/api/v1/admin/cheat-flags/8/dismiss", nil), 1)
	req = withURLParam(req, "id", "8")
	w := httptest.NewRecorder()
	h.HandleDismissCheatFlag(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}
