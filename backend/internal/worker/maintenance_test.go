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
func (m *mockNotificationRepo) MarkRead(_ context.Context, _, _ int64) error { return nil }
func (m *mockNotificationRepo) MarkAllRead(_ context.Context, _ int64) error { return nil }
func (m *mockNotificationRepo) MarkTopicReplyGroupRead(_ context.Context, _, _ int64) (int64, error) {
	return 0, nil
}
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

// --- connector delivery log (BE-10.1) --------------------------------------

// dueDeliveryRepo reports a fixed set of instances with due work and records the
// prune cutoffs it was asked for.
type dueDeliveryRepo struct {
	fakeDeliveryRepo
	due    []int64
	dueErr error
}

func (r *dueDeliveryRepo) InstancesWithDue(context.Context, time.Time) ([]int64, error) {
	return r.due, r.dueErr
}

func TestMaintenancePrunesConnectorDeliveriesPastRetention(t *testing.T) {
	deliveries := &dueDeliveryRepo{fakeDeliveryRepo: *newFakeDeliveryRepo()}
	deps := &WorkerDeps{
		ConnectorDeliveryRepo:      deliveries,
		ConnectorDeliveryRetention: 30 * 24 * time.Hour,
	}

	before := time.Now()
	if err := NewMaintenanceHandler(deps)(context.Background(), nil); err != nil {
		t.Fatalf("maintenance: %v", err)
	}

	if len(deliveries.deleted) != 1 {
		t.Fatalf("pruned %d times, want 1", len(deliveries.deleted))
	}
	cutoff := deliveries.deleted[0]
	want := before.Add(-30 * 24 * time.Hour)
	if cutoff.Sub(want) > time.Minute || want.Sub(cutoff) > time.Minute {
		t.Fatalf("cutoff = %v, want about %v", cutoff, want)
	}
}

// A misconfigured zero would set the cutoff to now and wipe the whole log.
func TestMaintenanceSkipsConnectorPruneWhenRetentionNonPositive(t *testing.T) {
	deliveries := &dueDeliveryRepo{fakeDeliveryRepo: *newFakeDeliveryRepo()}
	deps := &WorkerDeps{ConnectorDeliveryRepo: deliveries, ConnectorDeliveryRetention: 0}

	if err := NewMaintenanceHandler(deps)(context.Background(), nil); err != nil {
		t.Fatalf("maintenance: %v", err)
	}
	if len(deliveries.deleted) != 0 {
		t.Fatal("a non-positive retention must disable pruning entirely")
	}
}

// The safety net for work stranded between the delivery row being written and
// its drain being queued — a crash, or Redis being briefly unavailable.
func TestMaintenanceReEnqueuesInstancesWithDueDeliveries(t *testing.T) {
	deliveries := &dueDeliveryRepo{fakeDeliveryRepo: *newFakeDeliveryRepo(), due: []int64{3, 7}}
	enqueuer := &fakeEnqueuer{}
	deps := &WorkerDeps{ConnectorDeliveryRepo: deliveries, ConnectorEnqueuer: enqueuer}

	if err := NewMaintenanceHandler(deps)(context.Background(), nil); err != nil {
		t.Fatalf("maintenance: %v", err)
	}

	if len(enqueuer.calls) != 2 {
		t.Fatalf("queued %d drains, want 2", len(enqueuer.calls))
	}
	if enqueuer.calls[0].instanceID != 3 || enqueuer.calls[1].instanceID != 7 {
		t.Fatalf("queued %+v, want instances 3 and 7", enqueuer.calls)
	}
}

func TestMaintenanceToleratesDueLookupFailure(t *testing.T) {
	deliveries := &dueDeliveryRepo{fakeDeliveryRepo: *newFakeDeliveryRepo(), dueErr: errors.New("database down")}
	enqueuer := &fakeEnqueuer{}
	deps := &WorkerDeps{ConnectorDeliveryRepo: deliveries, ConnectorEnqueuer: enqueuer}

	if err := NewMaintenanceHandler(deps)(context.Background(), nil); err != nil {
		t.Fatalf("maintenance must survive a failed sweep: %v", err)
	}
	if len(enqueuer.calls) != 0 {
		t.Fatal("nothing should be queued when the sweep query failed")
	}
}

func TestMaintenanceSkipsConnectorStepsWhenUnwired(t *testing.T) {
	if err := NewMaintenanceHandler(&WorkerDeps{ConnectorDeliveryRetention: time.Hour})(context.Background(), nil); err != nil {
		t.Fatalf("maintenance: %v", err)
	}
}
