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

// ResolveReportAction defines the action to take when resolving a report.
type ResolveReportAction string

const (
	ResolveOnly          ResolveReportAction = "resolve"
	ResolveAndWarn       ResolveReportAction = "warn"
	ResolveAndDelete     ResolveReportAction = "delete"
)

// ResolveReportRequest holds the input for resolving a report with an action.
type ResolveReportRequest struct {
	Action ResolveReportAction `json:"action"`
}

// ReportService handles report business logic.
type ReportService struct {
	reports  repository.ReportRepository
	torrents repository.TorrentRepository
	users    repository.UserRepository
	eventBus event.Bus
	warningSvc *WarningService
	torrentSvc *TorrentService
}

// NewReportService creates a new ReportService.
func NewReportService(reports repository.ReportRepository, torrents repository.TorrentRepository, users repository.UserRepository, bus event.Bus) *ReportService {
	return &ReportService{reports: reports, torrents: torrents, users: users, eventBus: bus}
}

// SetWarningService sets the warning service for resolve-with-warn actions.
func (s *ReportService) SetWarningService(ws *WarningService) {
	s.warningSvc = ws
}

// SetTorrentService sets the torrent service for resolve-with-delete actions.
func (s *ReportService) SetTorrentService(ts *TorrentService) {
	s.torrentSvc = ts
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
		var torrentName string
		if t, err := s.torrents.GetByID(ctx, *req.TorrentID); err == nil {
			torrentName = t.Name
		}
		reporter, _ := s.users.GetByID(ctx, reporterID)
		var reporterUsername string
		if reporter != nil {
			reporterUsername = reporter.Username
		}
		s.eventBus.Publish(ctx, &event.TorrentReportedEvent{
			Base:        event.NewBase(event.TorrentReported, event.Actor{ID: reporterID, Username: reporterUsername}),
			TorrentID:   *req.TorrentID,
			TorrentName: torrentName,
			Reason:      req.Reason,
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

	resolver, _ := s.users.GetByID(ctx, resolvedByUserID)
	var resolverUsername string
	if resolver != nil {
		resolverUsername = resolver.Username
	}
	s.eventBus.Publish(ctx, &event.ReportResolvedEvent{
		Base:     event.NewBase(event.ReportResolved, event.Actor{ID: resolvedByUserID, Username: resolverUsername}),
		ReportID: reportID,
	})

	return nil
}

// ResolveWithAction resolves a report and optionally performs an action (warn user or delete torrent).
func (s *ReportService) ResolveWithAction(ctx context.Context, reportID, resolvedByUserID int64, action ResolveReportAction) error {
	// Get the report first to know the torrent/user context
	report, err := s.reports.GetByID(ctx, reportID)
	if err != nil {
		return ErrReportNotFound
	}

	// Mark as resolved
	if err := s.reports.Resolve(ctx, reportID, resolvedByUserID); err != nil {
		return ErrReportNotFound
	}

	s.eventBus.Publish(ctx, &event.ReportResolvedEvent{
		Base:     event.NewBase(event.ReportResolved, event.Actor{ID: resolvedByUserID}),
		ReportID: reportID,
	})

	switch action {
	case ResolveAndWarn:
		if report.TorrentID != nil && s.warningSvc != nil && s.torrentSvc != nil {
			// Get the torrent to find the uploader
			torrent, err := s.torrentSvc.GetByID(ctx, *report.TorrentID)
			if err == nil {
				reason := fmt.Sprintf("Warning issued from report: %s", report.Reason)
				_, _ = s.warningSvc.IssueManualWarning(ctx, torrent.UploaderID, reason, nil, resolvedByUserID)
			}
		}
	case ResolveAndDelete:
		if report.TorrentID != nil && s.torrentSvc != nil {
			adminPerms := model.Permissions{IsAdmin: true}
			_ = s.torrentSvc.DeleteTorrent(ctx, *report.TorrentID, resolvedByUserID, adminPerms)
		}
	}

	return nil
}
