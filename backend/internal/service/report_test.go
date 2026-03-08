package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// --- mock report repo ---

type mockReportRepo struct {
	mu      sync.Mutex
	reports []*model.Report
	nextID  int64
}

func newMockReportRepo() *mockReportRepo {
	return &mockReportRepo{nextID: 1}
}

func (m *mockReportRepo) Create(_ context.Context, report *model.Report) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	report.ID = m.nextID
	m.nextID++
	m.reports = append(m.reports, report)
	return nil
}

func (m *mockReportRepo) GetByID(_ context.Context, id int64) (*model.Report, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.reports {
		if r.ID == id {
			return r, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *mockReportRepo) ExistsByReporterAndTorrent(_ context.Context, reporterID int64, torrentID *int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.reports {
		if r.ReporterID == reporterID {
			if torrentID == nil && r.TorrentID == nil {
				return true, nil
			}
			if torrentID != nil && r.TorrentID != nil && *torrentID == *r.TorrentID {
				return true, nil
			}
		}
	}
	return false, nil
}

func (m *mockReportRepo) List(_ context.Context, opts repository.ListReportsOptions) ([]model.Report, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var filtered []model.Report
	for _, r := range m.reports {
		if opts.Status != nil {
			switch *opts.Status {
			case "resolved":
				if !r.Resolved {
					continue
				}
			case "pending":
				if r.Resolved {
					continue
				}
			}
		}
		filtered = append(filtered, *r)
	}

	total := int64(len(filtered))
	page := opts.Page
	if page <= 0 {
		page = 1
	}
	perPage := opts.PerPage
	if perPage <= 0 {
		perPage = 25
	}
	start := (page - 1) * perPage
	if start >= len(filtered) {
		return nil, total, nil
	}
	end := start + perPage
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[start:end], total, nil
}

func (m *mockReportRepo) Resolve(_ context.Context, id, resolvedByUserID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.reports {
		if r.ID == id {
			r.Resolved = true
			r.ResolvedBy = &resolvedByUserID
			return nil
		}
	}
	return errors.New("not found")
}

// --- tests ---

func TestReportService_Create_Success(t *testing.T) {
	repo := newMockReportRepo()
	svc := NewReportService(repo, event.NewInMemoryBus())

	torrentID := int64(42)
	report, err := svc.Create(context.Background(), 1, CreateReportRequest{
		TorrentID: &torrentID,
		Reason:    "fake content",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.ID != 1 {
		t.Errorf("expected ID 1, got %d", report.ID)
	}
	if report.ReporterID != 1 {
		t.Errorf("expected ReporterID 1, got %d", report.ReporterID)
	}
	if report.TorrentID == nil || *report.TorrentID != 42 {
		t.Errorf("expected TorrentID 42, got %v", report.TorrentID)
	}
}

func TestReportService_Create_EmptyReason(t *testing.T) {
	repo := newMockReportRepo()
	svc := NewReportService(repo, event.NewInMemoryBus())

	_, err := svc.Create(context.Background(), 1, CreateReportRequest{
		Reason: "",
	})
	if !errors.Is(err, ErrInvalidReport) {
		t.Errorf("expected ErrInvalidReport, got %v", err)
	}
}

func TestReportService_Create_Duplicate(t *testing.T) {
	repo := newMockReportRepo()
	svc := NewReportService(repo, event.NewInMemoryBus())

	torrentID := int64(42)
	_, err := svc.Create(context.Background(), 1, CreateReportRequest{
		TorrentID: &torrentID,
		Reason:    "fake content",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Same user, same torrent — should be rejected
	_, err = svc.Create(context.Background(), 1, CreateReportRequest{
		TorrentID: &torrentID,
		Reason:    "still fake",
	})
	if !errors.Is(err, ErrDuplicateReport) {
		t.Errorf("expected ErrDuplicateReport, got %v", err)
	}
}

func TestReportService_Create_DifferentTorrents(t *testing.T) {
	repo := newMockReportRepo()
	svc := NewReportService(repo, event.NewInMemoryBus())

	tid1 := int64(1)
	tid2 := int64(2)

	_, err := svc.Create(context.Background(), 1, CreateReportRequest{
		TorrentID: &tid1,
		Reason:    "report 1",
	})
	if err != nil {
		t.Fatalf("first report: %v", err)
	}

	// Same user, different torrent — should succeed
	_, err = svc.Create(context.Background(), 1, CreateReportRequest{
		TorrentID: &tid2,
		Reason:    "report 2",
	})
	if err != nil {
		t.Errorf("expected success for different torrent, got %v", err)
	}
}

func TestReportService_Create_NilTorrentID(t *testing.T) {
	repo := newMockReportRepo()
	svc := NewReportService(repo, event.NewInMemoryBus())

	report, err := svc.Create(context.Background(), 1, CreateReportRequest{
		Reason: "general report",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.TorrentID != nil {
		t.Errorf("expected nil TorrentID, got %v", report.TorrentID)
	}

	// Duplicate nil torrent report
	_, err = svc.Create(context.Background(), 1, CreateReportRequest{
		Reason: "another general report",
	})
	if !errors.Is(err, ErrDuplicateReport) {
		t.Errorf("expected ErrDuplicateReport for nil torrent, got %v", err)
	}
}

func TestReportService_List(t *testing.T) {
	repo := newMockReportRepo()
	svc := NewReportService(repo, event.NewInMemoryBus())

	tid := int64(1)
	_, _ = svc.Create(context.Background(), 1, CreateReportRequest{TorrentID: &tid, Reason: "r1"})
	tid2 := int64(2)
	_, _ = svc.Create(context.Background(), 2, CreateReportRequest{TorrentID: &tid2, Reason: "r2"})

	reports, total, err := svc.List(context.Background(), repository.ListReportsOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Errorf("expected total 2, got %d", total)
	}
	if len(reports) != 2 {
		t.Errorf("expected 2 reports, got %d", len(reports))
	}
}

func TestReportService_List_FilterByStatus(t *testing.T) {
	repo := newMockReportRepo()
	svc := NewReportService(repo, event.NewInMemoryBus())

	tid := int64(1)
	_, _ = svc.Create(context.Background(), 1, CreateReportRequest{TorrentID: &tid, Reason: "r1"})
	tid2 := int64(2)
	_, _ = svc.Create(context.Background(), 2, CreateReportRequest{TorrentID: &tid2, Reason: "r2"})

	// Resolve first report
	_ = svc.Resolve(context.Background(), 1, 99)

	pending := "pending"
	reports, total, err := svc.List(context.Background(), repository.ListReportsOptions{Status: &pending})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 pending, got %d", total)
	}
	if len(reports) != 1 {
		t.Errorf("expected 1 report, got %d", len(reports))
	}

	resolved := "resolved"
	reports, total, err = svc.List(context.Background(), repository.ListReportsOptions{Status: &resolved})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 resolved, got %d", total)
	}
	if len(reports) != 1 {
		t.Errorf("expected 1 report, got %d", len(reports))
	}
}

func TestReportService_Resolve_Success(t *testing.T) {
	repo := newMockReportRepo()
	svc := NewReportService(repo, event.NewInMemoryBus())

	tid := int64(1)
	_, _ = svc.Create(context.Background(), 1, CreateReportRequest{TorrentID: &tid, Reason: "r1"})

	err := svc.Resolve(context.Background(), 1, 99)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReportService_Resolve_NotFound(t *testing.T) {
	repo := newMockReportRepo()
	svc := NewReportService(repo, event.NewInMemoryBus())

	err := svc.Resolve(context.Background(), 999, 99)
	if !errors.Is(err, ErrReportNotFound) {
		t.Errorf("expected ErrReportNotFound, got %v", err)
	}
}
