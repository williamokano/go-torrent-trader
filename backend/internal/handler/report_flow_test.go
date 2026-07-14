package handler

import (
	"context"
	"errors"
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

// Error and action paths for the report handler that report_test.go leaves open.

type stubReportRepo struct {
	byID     map[int64]*model.Report
	list     []model.Report
	total    int64
	exists   bool
	created  []*model.Report
	resolved []int64
	lastOpts repository.ListReportsOptions

	createErr  error
	listErr    error
	resolveErr error

	nextID int64
}

func newStubReportRepo() *stubReportRepo {
	return &stubReportRepo{byID: map[int64]*model.Report{}, nextID: 1}
}

func (s *stubReportRepo) Create(_ context.Context, report *model.Report) error {
	if s.createErr != nil {
		return s.createErr
	}
	report.ID = s.nextID
	s.nextID++
	s.created = append(s.created, report)
	stored := *report
	s.byID[report.ID] = &stored
	return nil
}
func (s *stubReportRepo) GetByID(_ context.Context, id int64) (*model.Report, error) {
	r, ok := s.byID[id]
	if !ok {
		return nil, errors.New("report not found")
	}
	return r, nil
}
func (s *stubReportRepo) ExistsByReporterAndTorrent(_ context.Context, _ int64, _ *int64) (bool, error) {
	return s.exists, nil
}
func (s *stubReportRepo) List(_ context.Context, opts repository.ListReportsOptions) ([]model.Report, int64, error) {
	s.lastOpts = opts
	if s.listErr != nil {
		return nil, 0, s.listErr
	}
	return s.list, s.total, nil
}
func (s *stubReportRepo) Resolve(_ context.Context, id, _ int64) error {
	if s.resolveErr != nil {
		return s.resolveErr
	}
	s.resolved = append(s.resolved, id)
	return nil
}

func newReportHandler(reports *stubReportRepo, torrents *stubTorrentRepo, users *stubUserRepo) *ReportHandler {
	return NewReportHandler(service.NewReportService(reports, torrents, users, event.NewInMemoryBus()))
}

func TestCreateReportRequiresAuth(t *testing.T) {
	h := newReportHandler(newStubReportRepo(), &stubTorrentRepo{}, newStubUserRepo())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/reports", strings.NewReader(`{"reason":"fake"}`))
	w := httptest.NewRecorder()
	h.HandleCreate(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestCreateReportValidatesInput(t *testing.T) {
	for _, body := range []string{`{`, `{"reason":""}`} {
		reports := newStubReportRepo()
		h := newReportHandler(reports, &stubTorrentRepo{}, newStubUserRepo())

		req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/reports", strings.NewReader(body)), 7, 20)
		w := httptest.NewRecorder()
		h.HandleCreate(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, w.Code)
		}
		if len(reports.created) != 0 {
			t.Error("a report was created despite the invalid request")
		}
	}
}

func TestCreateReportStoresReport(t *testing.T) {
	reports := newStubReportRepo()
	torrents := &stubTorrentRepo{torrents: map[int64]*model.Torrent{3: {ID: 3, Name: "A movie"}}}
	h := newReportHandler(reports, torrents, newStubUserRepo(&model.User{ID: 7, Username: "member"}))

	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/reports",
		strings.NewReader(`{"torrent_id":3,"reason":"fake release"}`)), 7, 20)
	w := httptest.NewRecorder()
	h.HandleCreate(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	if len(reports.created) != 1 || reports.created[0].ReporterID != 7 {
		t.Fatalf("reports = %+v, want one from the authenticated user", reports.created)
	}
	report := decodeBody(t, w)["report"].(map[string]interface{})
	if report["torrent_id"] != float64(3) || report["reason"] != "fake release" {
		t.Errorf("report = %v, want torrent 3 and the given reason", report)
	}
}

// One report per user per torrent — a second submission is a conflict.
func TestCreateReportRejectsDuplicate(t *testing.T) {
	reports := newStubReportRepo()
	reports.exists = true
	h := newReportHandler(reports, &stubTorrentRepo{}, newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/reports",
		strings.NewReader(`{"torrent_id":3,"reason":"fake"}`)), 7, 20)
	w := httptest.NewRecorder()
	h.HandleCreate(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409 for a duplicate report", w.Code)
	}
	if len(reports.created) != 0 {
		t.Error("a duplicate report was stored")
	}
}

func TestCreateReportSurfacesStoreFailure(t *testing.T) {
	reports := newStubReportRepo()
	reports.createErr = errStub
	h := newReportHandler(reports, &stubTorrentRepo{}, newStubUserRepo())

	req := authed(httptest.NewRequest(http.MethodPost, "/api/v1/reports",
		strings.NewReader(`{"reason":"fake"}`)), 7, 20)
	w := httptest.NewRecorder()
	h.HandleCreate(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestListReportsAppliesStatusFilter(t *testing.T) {
	reports := newStubReportRepo()
	resolvedAt := time.Now()
	resolvedBy := int64(1)
	torrentID := int64(3)
	reports.list = []model.Report{{
		ID: 1, ReporterID: 7, ReporterUsername: "member", TorrentID: &torrentID,
		TorrentName: "A movie", Reason: "fake", Resolved: true,
		ResolvedBy: &resolvedBy, ResolvedAt: &resolvedAt,
	}}
	reports.total = 1
	h := newReportHandler(reports, &stubTorrentRepo{}, newStubUserRepo())

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/reports?status=resolved&page=2&per_page=10", nil), 1)
	w := httptest.NewRecorder()
	h.HandleList(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if reports.lastOpts.Status == nil || *reports.lastOpts.Status != "resolved" {
		t.Errorf("status filter = %v, want \"resolved\"", reports.lastOpts.Status)
	}
	if reports.lastOpts.Page != 2 || reports.lastOpts.PerPage != 10 {
		t.Errorf("page/per_page = %d/%d, want 2/10", reports.lastOpts.Page, reports.lastOpts.PerPage)
	}

	items := decodeBody(t, w)["reports"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("reports = %v, want 1 item", items)
	}
	report := items[0].(map[string]interface{})
	for _, key := range []string{"id", "reporter_id", "reporter_username", "reason", "resolved",
		"torrent_id", "torrent_name", "resolved_by", "resolved_at"} {
		if _, present := report[key]; !present {
			t.Errorf("report response is missing key %q", key)
		}
	}
}

func TestListReportsSurfacesStoreFailure(t *testing.T) {
	reports := newStubReportRepo()
	reports.listErr = errStub
	h := newReportHandler(reports, &stubTorrentRepo{}, newStubUserRepo())

	req := adminAuthed(httptest.NewRequest(http.MethodGet, "/api/v1/reports", nil), 1)
	w := httptest.NewRecorder()
	h.HandleList(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestResolveReportRequiresAuth(t *testing.T) {
	h := newReportHandler(newStubReportRepo(), &stubTorrentRepo{}, newStubUserRepo())

	req := withURLParam(httptest.NewRequest(http.MethodPut, "/api/v1/reports/1/resolve", nil), "id", "1")
	w := httptest.NewRecorder()
	h.HandleResolve(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestResolveReportRejectsInvalidID(t *testing.T) {
	h := newReportHandler(newStubReportRepo(), &stubTorrentRepo{}, newStubUserRepo())

	req := adminAuthed(httptest.NewRequest(http.MethodPut, "/api/v1/reports/0/resolve", nil), 1)
	req = withURLParam(req, "id", "0")
	w := httptest.NewRecorder()
	h.HandleResolve(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestResolveReportRejectsBadJSON(t *testing.T) {
	h := newReportHandler(newStubReportRepo(), &stubTorrentRepo{}, newStubUserRepo())

	req := adminAuthed(httptest.NewRequest(http.MethodPut, "/api/v1/reports/1/resolve", strings.NewReader("{")), 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleResolve(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestResolveReportMarksResolved(t *testing.T) {
	reports := newStubReportRepo()
	reports.byID[1] = &model.Report{ID: 1, ReporterID: 7, Reason: "fake"}
	h := newReportHandler(reports, &stubTorrentRepo{}, newStubUserRepo(&model.User{ID: 1, Username: "admin"}))

	req := adminAuthed(httptest.NewRequest(http.MethodPut, "/api/v1/reports/1/resolve", nil), 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleResolve(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if len(reports.resolved) != 1 || reports.resolved[0] != 1 {
		t.Errorf("resolved = %v, want [1]", reports.resolved)
	}
}

func TestResolveReportReturns404WhenMissing(t *testing.T) {
	reports := newStubReportRepo()
	reports.resolveErr = errStub
	h := newReportHandler(reports, &stubTorrentRepo{}, newStubUserRepo())

	req := adminAuthed(httptest.NewRequest(http.MethodPut, "/api/v1/reports/1/resolve", nil), 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleResolve(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// resolve+delete removes the reported torrent, then marks the report resolved.
func TestResolveReportWithDeleteRemovesTorrent(t *testing.T) {
	torrentID := int64(3)
	reports := newStubReportRepo()
	reports.byID[1] = &model.Report{ID: 1, ReporterID: 7, TorrentID: &torrentID, Reason: "fake"}
	torrents := &stubTorrentRepo{torrents: map[int64]*model.Torrent{3: {ID: 3, Name: "A movie", UploaderID: 9}}}
	users := newStubUserRepo(&model.User{ID: 1, Username: "admin"})

	reportSvc := service.NewReportService(reports, torrents, users, event.NewInMemoryBus())
	reportSvc.SetTorrentService(service.NewTorrentService(nil, torrents, users, &stubStorage{},
		service.TorrentServiceConfig{}, event.NewInMemoryBus(), nil))
	h := NewReportHandler(reportSvc)

	req := adminAuthed(httptest.NewRequest(http.MethodPut, "/api/v1/reports/1/resolve",
		strings.NewReader(`{"action":"delete"}`)), 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleResolve(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if len(torrents.deleted) != 1 || torrents.deleted[0] != 3 {
		t.Errorf("deleted torrents = %v, want [3]", torrents.deleted)
	}
	if len(reports.resolved) != 1 {
		t.Error("the report was not marked resolved after the delete succeeded")
	}
}

// A report with no torrent cannot be resolved with a torrent action.
func TestResolveReportWithDeleteRejectsTorrentlessReport(t *testing.T) {
	reports := newStubReportRepo()
	reports.byID[1] = &model.Report{ID: 1, ReporterID: 7, Reason: "user abuse"} // no TorrentID
	h := newReportHandler(reports, &stubTorrentRepo{}, newStubUserRepo())

	req := adminAuthed(httptest.NewRequest(http.MethodPut, "/api/v1/reports/1/resolve",
		strings.NewReader(`{"action":"delete"}`)), 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleResolve(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if len(reports.resolved) != 0 {
		t.Error("the report was resolved even though the action was invalid")
	}
}

// If the action fails, the report must NOT be marked resolved.
func TestResolveReportWithDeleteLeavesReportOpenWhenDeleteFails(t *testing.T) {
	torrentID := int64(3)
	reports := newStubReportRepo()
	reports.byID[1] = &model.Report{ID: 1, ReporterID: 7, TorrentID: &torrentID, Reason: "fake"}
	torrents := &stubTorrentRepo{
		torrents:  map[int64]*model.Torrent{3: {ID: 3, UploaderID: 9}},
		deleteErr: errStub,
	}
	users := newStubUserRepo(&model.User{ID: 1})

	reportSvc := service.NewReportService(reports, torrents, users, event.NewInMemoryBus())
	reportSvc.SetTorrentService(service.NewTorrentService(nil, torrents, users, &stubStorage{},
		service.TorrentServiceConfig{}, event.NewInMemoryBus(), nil))
	h := NewReportHandler(reportSvc)

	req := adminAuthed(httptest.NewRequest(http.MethodPut, "/api/v1/reports/1/resolve",
		strings.NewReader(`{"action":"delete"}`)), 1)
	req = withURLParam(req, "id", "1")
	w := httptest.NewRecorder()
	h.HandleResolve(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
	if len(reports.resolved) != 0 {
		t.Error("the report was marked resolved even though the torrent delete failed")
	}
}
