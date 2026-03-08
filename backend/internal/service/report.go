package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

var (
	ErrDuplicateReport = errors.New("you have already reported this item")
	ErrReportNotFound  = errors.New("report not found")
	ErrInvalidReport   = errors.New("invalid report")
)

// CreateReportRequest holds the input for submitting a report.
type CreateReportRequest struct {
	TorrentID *int64 `json:"torrent_id"`
	Reason    string `json:"reason"`
}

// ReportService handles report business logic.
type ReportService struct {
	reports  repository.ReportRepository
	eventBus event.Bus
}

// NewReportService creates a new ReportService.
func NewReportService(reports repository.ReportRepository, bus event.Bus) *ReportService {
	return &ReportService{reports: reports, eventBus: bus}
}

// Create submits a new report. One report per user per torrent is enforced.
func (s *ReportService) Create(ctx context.Context, reporterID int64, req CreateReportRequest) (*model.Report, error) {
	if req.Reason == "" {
		return nil, fmt.Errorf("%w: reason is required", ErrInvalidReport)
	}

	// One report per user per torrent
	exists, err := s.reports.ExistsByReporterAndTorrent(ctx, reporterID, req.TorrentID)
	if err != nil {
		return nil, fmt.Errorf("check duplicate: %w", err)
	}
	if exists {
		return nil, ErrDuplicateReport
	}

	report := &model.Report{
		ReporterID: reporterID,
		TorrentID:  req.TorrentID,
		Reason:     req.Reason,
	}

	if err := s.reports.Create(ctx, report); err != nil {
		return nil, fmt.Errorf("create report: %w", err)
	}

	if req.TorrentID != nil {
		s.eventBus.Publish(ctx, &event.TorrentReportedEvent{
			Base:      event.NewBase(event.TorrentReported, event.Actor{ID: reporterID}),
			TorrentID: *req.TorrentID,
			Reason:    req.Reason,
		})
	}

	return report, nil
}

// List returns a paginated list of reports (admin only — authorization enforced at handler layer).
func (s *ReportService) List(ctx context.Context, opts repository.ListReportsOptions) ([]model.Report, int64, error) {
	if opts.Page <= 0 {
		opts.Page = 1
	}
	if opts.PerPage <= 0 {
		opts.PerPage = 25
	}
	if opts.PerPage > 100 {
		opts.PerPage = 100
	}
	return s.reports.List(ctx, opts)
}

// Resolve marks a report as resolved.
func (s *ReportService) Resolve(ctx context.Context, reportID, resolvedByUserID int64) error {
	if err := s.reports.Resolve(ctx, reportID, resolvedByUserID); err != nil {
		return ErrReportNotFound
	}

	s.eventBus.Publish(ctx, &event.ReportResolvedEvent{
		Base:     event.NewBase(event.ReportResolved, event.Actor{ID: resolvedByUserID}),
		ReportID: reportID,
	})

	return nil
}
