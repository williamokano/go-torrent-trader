package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// --- mock NotificationRepository -------------------------------------------

type mockNotificationRepo struct {
	deleteOldCount int64
	deleteOldErr   error
	deleteOldCall  *time.Time // captures the cutoff arg; nil when never called
}

func (m *mockNotificationRepo) Create(_ context.Context, _ *model.Notification) error { return nil }
func (m *mockNotificationRepo) GetByID(_ context.Context, _ int64) (*model.Notification, error) {
	return nil, nil
}
func (m *mockNotificationRepo) List(_ context.Context, _ int64, _ repository.ListNotificationsOptions) ([]model.Notification, int64, error) {
	return nil, 0, nil
}
func (m *mockNotificationRepo) MarkRead(_ context.Context, _, _ int64) error        { return nil }
func (m *mockNotificationRepo) MarkAllRead(_ context.Context, _ int64) error        { return nil }
func (m *mockNotificationRepo) CountUnread(_ context.Context, _ int64) (int, error) { return 0, nil }

func (m *mockNotificationRepo) DeleteOld(_ context.Context, before time.Time) (int64, error) {
	m.deleteOldCall = &before
	return m.deleteOldCount, m.deleteOldErr
}

func (m *mockNotificationRepo) CountUnreadSince(_ context.Context, _ int64, _ time.Time) (int, error) {
	return 0, nil
}

func (m *mockNotificationRepo) ListUnreadSince(_ context.Context, _ int64, _ time.Time, _ int) ([]model.Notification, error) {
	return nil, nil
}

// --- tests -----------------------------------------------------------------

func TestMaintenancePurgesNotificationsPastRetention(t *testing.T) {
	repo := &mockNotificationRepo{deleteOldCount: 3}
	deps := &WorkerDeps{
		NotificationRepo:      repo,
		NotificationRetention: 90 * 24 * time.Hour,
	}

	before := time.Now()
	if err := NewMaintenanceHandler(deps)(context.Background(), nil); err != nil {
		t.Fatalf("maintenance handler returned error: %v", err)
	}

	if repo.deleteOldCall == nil {
		t.Fatal("DeleteOld was never called — the purge is not wired into the maintenance job")
	}

	// The cutoff must be one retention window in the past.
	wantCutoff := before.Add(-90 * 24 * time.Hour)
	if diff := repo.deleteOldCall.Sub(wantCutoff); diff < -time.Minute || diff > time.Minute {
		t.Errorf("cutoff = %v, want ~%v (retention window before now)", *repo.deleteOldCall, wantCutoff)
	}
}

// A zero retention would put the cutoff at "now", purging every read
// notification. Treat non-positive values as "disabled" instead.
func TestMaintenanceSkipsNotificationPurgeWhenRetentionNonPositive(t *testing.T) {
	for _, retention := range []time.Duration{0, -time.Hour} {
		repo := &mockNotificationRepo{}
		deps := &WorkerDeps{
			NotificationRepo:      repo,
			NotificationRetention: retention,
		}

		if err := NewMaintenanceHandler(deps)(context.Background(), nil); err != nil {
			t.Fatalf("maintenance handler returned error: %v", err)
		}

		if repo.deleteOldCall != nil {
			t.Errorf("retention %v: DeleteOld was called with cutoff %v — a non-positive retention must disable the purge, not delete everything",
				retention, *repo.deleteOldCall)
		}
	}
}

// A purge failure must not abort the maintenance run — the handler logs and
// keeps going, so asynq does not retry the unrelated steps.
func TestMaintenanceToleratesNotificationPurgeError(t *testing.T) {
	repo := &mockNotificationRepo{deleteOldErr: errors.New("db down")}
	deps := &WorkerDeps{
		NotificationRepo:      repo,
		NotificationRetention: 24 * time.Hour,
	}

	if err := NewMaintenanceHandler(deps)(context.Background(), nil); err != nil {
		t.Fatalf("purge error should not fail the maintenance run, got: %v", err)
	}
	if repo.deleteOldCall == nil {
		t.Error("DeleteOld should still have been attempted")
	}
}

// The handler must stay safe when the repo is absent (e.g. a worker built
// without notification wiring).
func TestMaintenanceSkipsNotificationPurgeWhenRepoNil(t *testing.T) {
	deps := &WorkerDeps{NotificationRetention: 24 * time.Hour}

	if err := NewMaintenanceHandler(deps)(context.Background(), nil); err != nil {
		t.Fatalf("maintenance handler returned error: %v", err)
	}
}
