package service

import (
	"context"
	"slices"
	"testing"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

type fakeActivityLogRepo struct {
	lastOpts repository.ListActivityLogsOptions
	logs     []model.ActivityLog
}

func (f *fakeActivityLogRepo) Create(_ context.Context, _ *model.ActivityLog) error {
	return nil
}

func (f *fakeActivityLogRepo) List(_ context.Context, opts repository.ListActivityLogsOptions) ([]model.ActivityLog, int64, error) {
	f.lastOpts = opts
	return f.logs, int64(len(f.logs)), nil
}

func TestActivityLogListExcludesStaffOnlyForMembers(t *testing.T) {
	repo := &fakeActivityLogRepo{}
	svc := NewActivityLogService(repo)

	if _, _, err := svc.List(context.Background(), repository.ListActivityLogsOptions{}); err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(repo.lastOpts.ExcludeEventTypes) == 0 {
		t.Fatal("expected staff-only event types to be excluded for non-staff listing")
	}
	for _, typ := range []string{string(event.BackupCreated), string(event.CheatFlagged), string(event.IPBanned)} {
		if !slices.Contains(repo.lastOpts.ExcludeEventTypes, typ) {
			t.Errorf("expected %s in exclusion list", typ)
		}
	}
}

func TestActivityLogListIncludesStaffOnlyForStaff(t *testing.T) {
	repo := &fakeActivityLogRepo{}
	svc := NewActivityLogService(repo)

	if _, _, err := svc.List(context.Background(), repository.ListActivityLogsOptions{IncludeStaffOnly: true}); err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(repo.lastOpts.ExcludeEventTypes) != 0 {
		t.Errorf("expected no exclusions for staff listing, got %v", repo.lastOpts.ExcludeEventTypes)
	}
}
