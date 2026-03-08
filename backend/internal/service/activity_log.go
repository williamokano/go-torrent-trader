package service

import (
	"context"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// ActivityLogService handles product activity log persistence. It does NOT
// know about event types — listeners in the listener package call Create.
type ActivityLogService struct {
	logs repository.ActivityLogRepository
}

// NewActivityLogService creates a new ActivityLogService.
func NewActivityLogService(logs repository.ActivityLogRepository) *ActivityLogService {
	return &ActivityLogService{logs: logs}
}

// Create persists a new activity log entry.
func (s *ActivityLogService) Create(ctx context.Context, log *model.ActivityLog) error {
	return s.logs.Create(ctx, log)
}

// List returns a paginated list of activity logs.
func (s *ActivityLogService) List(ctx context.Context, opts repository.ListActivityLogsOptions) ([]model.ActivityLog, int64, error) {
	if opts.Page <= 0 {
		opts.Page = 1
	}
	if opts.PerPage <= 0 {
		opts.PerPage = 25
	}
	if opts.PerPage > 100 {
		opts.PerPage = 100
	}
	return s.logs.List(ctx, opts)
}
