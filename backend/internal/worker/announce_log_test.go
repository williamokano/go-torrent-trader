package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// --- mocks -----------------------------------------------------------------

type deleteCall struct {
	cutoff  time.Time
	limit   int
	deleted int64
}

type mockAnnounceEventRepo struct {
	// remaining is how many rows are still older than the cutoff. Each call
	// deletes up to limit of them, so the handler's chunk loop behaves the way it
	// would against a real table.
	remaining int
	deleteErr error
	calls     []deleteCall
}

func (m *mockAnnounceEventRepo) Create(context.Context, *model.AnnounceEvent) error { return nil }

func (m *mockAnnounceEventRepo) ListByUser(context.Context, int64, int, int) ([]repository.AnnounceEventWithTorrent, int64, error) {
	return nil, 0, nil
}

func (m *mockAnnounceEventRepo) DeleteOlderThan(_ context.Context, cutoff time.Time, limit int) (int64, error) {
	if m.deleteErr != nil {
		m.calls = append(m.calls, deleteCall{cutoff: cutoff, limit: limit})
		return 0, m.deleteErr
	}
	n := limit
	if m.remaining < n {
		n = m.remaining
	}
	m.remaining -= n
	m.calls = append(m.calls, deleteCall{cutoff: cutoff, limit: limit, deleted: int64(n)})
	return int64(n), nil
}

// deletedRows is the number of rows the prune actually removed, as distinct from
// how many statements it took. Both matter and they are easy to confuse.
func (m *mockAnnounceEventRepo) deletedRows() int64 {
	var total int64
	for _, c := range m.calls {
		total += c.deleted
	}
	return total
}

type rollupCall struct {
	through time.Time
	maxDays int
}

type mockAnnounceRollupRepo struct {
	// watermark is the exclusive upper date bound already aggregated. Rollup
	// advances it by at most maxDays per call, like the real one.
	watermark time.Time
	rowsEach  int64
	rollupErr error
	// rollupErrAfter lets the first N calls succeed before rollupErr kicks in.
	rollupErrAfter   int
	rolledThroughErr error

	calls []rollupCall
}

func (m *mockAnnounceRollupRepo) RolledThrough(context.Context) (time.Time, error) {
	if m.rolledThroughErr != nil {
		return time.Time{}, m.rolledThroughErr
	}
	return m.watermark, nil
}

func (m *mockAnnounceRollupRepo) Rollup(_ context.Context, through time.Time, maxDays int) (repository.RollupResult, error) {
	m.calls = append(m.calls, rollupCall{through: through, maxDays: maxDays})
	if m.rollupErr != nil && len(m.calls) > m.rollupErrAfter {
		return repository.RollupResult{}, m.rollupErr
	}

	from := m.watermark
	if !from.Before(through) {
		return repository.RollupResult{From: from, To: from, CaughtUp: true}, nil
	}
	to := from.AddDate(0, 0, maxDays)
	if to.After(through) {
		to = through
	}
	m.watermark = to
	return repository.RollupResult{From: from, To: to, Rows: m.rowsEach, CaughtUp: !to.Before(through)}, nil
}

func (m *mockAnnounceRollupRepo) ListByUser(context.Context, int64, int) ([]model.UserPeriodStats, error) {
	return nil, nil
}

// utcToday is the boundary the handler aggregates up to.
func utcToday() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

// --- tests -----------------------------------------------------------------

func TestAnnounceLogMaintenance_RollsUpThenPrunes(t *testing.T) {
	// A week behind, so one chunk catches up.
	rollups := &mockAnnounceRollupRepo{watermark: utcToday().AddDate(0, 0, -7), rowsEach: 4}
	events := &mockAnnounceEventRepo{remaining: 12}
	deps := &WorkerDeps{
		AnnounceRollupRepo:   rollups,
		AnnounceEventRepo:    events,
		AnnounceLogRetention: func() time.Duration { return 90 * 24 * time.Hour },
	}

	if err := NewAnnounceLogMaintenanceHandler(deps)(context.Background(), nil); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if len(rollups.calls) != 1 {
		t.Fatalf("expected one rollup call to catch up a week, got %d", len(rollups.calls))
	}
	if !rollups.calls[0].through.Equal(utcToday()) {
		t.Errorf("rollup ran through %v, want this morning's UTC midnight %v",
			rollups.calls[0].through, utcToday())
	}
	if rollups.watermark != utcToday() {
		t.Errorf("watermark = %v, want %v", rollups.watermark, utcToday())
	}

	if len(events.calls) == 0 {
		t.Fatal("nothing was pruned — the retention job is not wired")
	}
	wantCutoff := time.Now().Add(-90 * 24 * time.Hour)
	if diff := events.calls[0].cutoff.Sub(wantCutoff); diff < -time.Minute || diff > time.Minute {
		t.Errorf("cutoff = %v, want ~%v", events.calls[0].cutoff, wantCutoff)
	}
	if events.remaining != 0 {
		t.Errorf("%d rows past retention survived the chunk loop", events.remaining)
	}
}

// The whole point of the ordering: a raw row may only be deleted once its bytes
// are counted. When the rollup is behind retention, the prune cutoff is pulled
// back to the watermark rather than deleting by age.
func TestAnnounceLogMaintenance_PruneNeverPassesTheRollupWatermark(t *testing.T) {
	// Ten years of arrears — more than announceRollupChunkDays *
	// announceRollupMaxChunks can close in one run, so the run ends with the
	// watermark still older than the 90-day retention cutoff. That is the case
	// where the two bounds disagree and the watermark has to win.
	rollups := &mockAnnounceRollupRepo{watermark: utcToday().AddDate(-10, 0, 0)}
	events := &mockAnnounceEventRepo{remaining: 1}
	deps := &WorkerDeps{
		AnnounceRollupRepo:   rollups,
		AnnounceEventRepo:    events,
		AnnounceLogRetention: func() time.Duration { return 90 * 24 * time.Hour },
	}

	if err := NewAnnounceLogMaintenanceHandler(deps)(context.Background(), nil); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if rollups.watermark.After(time.Now().Add(-90 * 24 * time.Hour)) {
		t.Fatal("the mock caught up past the retention cutoff — this test no longer exercises the watermark bound")
	}

	if len(events.calls) == 0 {
		t.Fatal("expected a prune bounded by the watermark, got none")
	}
	cutoff := events.calls[0].cutoff
	retentionCutoff := time.Now().Add(-90 * 24 * time.Hour)
	if cutoff.After(retentionCutoff) {
		t.Errorf("cutoff %v is newer than the retention window %v — un-aggregated rows would be deleted",
			cutoff, retentionCutoff)
	}
	if !cutoff.Equal(rollups.watermark) {
		t.Errorf("cutoff = %v, want the rollup watermark %v", cutoff, rollups.watermark)
	}
}

// A failing rollup must not let the prune fall back to deleting by age: that is
// the one failure mode that loses data permanently.
func TestAnnounceLogMaintenance_RollupFailureLimitsPruneToLastGoodWatermark(t *testing.T) {
	watermark := utcToday().AddDate(0, 0, -400)
	rollups := &mockAnnounceRollupRepo{
		watermark: watermark,
		rollupErr: errors.New("boom"),
	}
	events := &mockAnnounceEventRepo{remaining: 1}
	deps := &WorkerDeps{
		AnnounceRollupRepo:   rollups,
		AnnounceEventRepo:    events,
		AnnounceLogRetention: func() time.Duration { return 24 * time.Hour },
	}

	// The error is reported to asynq (see RollupFailureIsReturnedToAsynq); what
	// matters here is where the prune stopped.
	if err := NewAnnounceLogMaintenanceHandler(deps)(context.Background(), nil); err == nil {
		t.Fatal("expected the rollup failure to be reported")
	}

	if len(events.calls) != 1 {
		t.Fatalf("expected exactly one bounded prune, got %d calls", len(events.calls))
	}
	if !events.calls[0].cutoff.Equal(watermark) {
		t.Errorf("cutoff = %v, want the last good watermark %v", events.calls[0].cutoff, watermark)
	}
}

// Same guard as the notification and connector purges: a zero window would set the
// cutoff to now and delete the entire log.
func TestAnnounceLogMaintenance_NonPositiveRetentionDisablesPrune(t *testing.T) {
	for _, retention := range []time.Duration{0, -time.Hour} {
		rollups := &mockAnnounceRollupRepo{watermark: utcToday()}
		events := &mockAnnounceEventRepo{remaining: 100}
		deps := &WorkerDeps{
			AnnounceRollupRepo:   rollups,
			AnnounceEventRepo:    events,
			AnnounceLogRetention: func() time.Duration { return retention },
		}

		if err := NewAnnounceLogMaintenanceHandler(deps)(context.Background(), nil); err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		if len(events.calls) != 0 {
			t.Errorf("retention %v: pruned anyway (%d calls)", retention, len(events.calls))
		}
	}
}

// An unwired deployment must be inert rather than panicking the worker.
func TestAnnounceLogMaintenance_NilDepsAreInert(t *testing.T) {
	for name, deps := range map[string]*WorkerDeps{
		"nothing wired": {},
		"no rollup repo": {
			AnnounceEventRepo:    &mockAnnounceEventRepo{remaining: 5},
			AnnounceLogRetention: func() time.Duration { return time.Hour },
		},
		"no event repo": {
			AnnounceRollupRepo:   &mockAnnounceRollupRepo{watermark: utcToday()},
			AnnounceLogRetention: func() time.Duration { return time.Hour },
		},
		"no retention func": {
			AnnounceRollupRepo: &mockAnnounceRollupRepo{watermark: utcToday()},
			AnnounceEventRepo:  &mockAnnounceEventRepo{remaining: 5},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := NewAnnounceLogMaintenanceHandler(deps)(context.Background(), nil); err != nil {
				t.Fatalf("handler returned error: %v", err)
			}
		})
	}
}

// A rollup already at the boundary does nothing but must still report a watermark
// the prune can use, or a caught-up deployment would stop pruning.
func TestAnnounceLogMaintenance_CaughtUpRollupStillPrunes(t *testing.T) {
	rollups := &mockAnnounceRollupRepo{watermark: utcToday()}
	events := &mockAnnounceEventRepo{remaining: 3}
	deps := &WorkerDeps{
		AnnounceRollupRepo:   rollups,
		AnnounceEventRepo:    events,
		AnnounceLogRetention: func() time.Duration { return 90 * 24 * time.Hour },
	}

	if err := NewAnnounceLogMaintenanceHandler(deps)(context.Background(), nil); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if events.remaining != 0 {
		t.Errorf("%d rows past retention survived", events.remaining)
	}
}

// Chunking is what keeps one night's prune from becoming one enormous DELETE.
func TestAnnounceLogMaintenance_PrunesInBoundedChunks(t *testing.T) {
	rollups := &mockAnnounceRollupRepo{watermark: utcToday()}
	events := &mockAnnounceEventRepo{remaining: announcePruneChunkRows*2 + 7}
	deps := &WorkerDeps{
		AnnounceRollupRepo:   rollups,
		AnnounceEventRepo:    events,
		AnnounceLogRetention: func() time.Duration { return 90 * 24 * time.Hour },
	}

	if err := NewAnnounceLogMaintenanceHandler(deps)(context.Background(), nil); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if len(events.calls) != 3 {
		t.Errorf("expected 3 chunked deletes, got %d", len(events.calls))
	}
	if want := int64(announcePruneChunkRows*2 + 7); events.deletedRows() != want {
		t.Errorf("deleted %d rows, want %d", events.deletedRows(), want)
	}
	for i, c := range events.calls {
		if c.limit != announcePruneChunkRows {
			t.Errorf("chunk %d asked for %d rows, want %d", i, c.limit, announcePruneChunkRows)
		}
	}
	if events.remaining != 0 {
		t.Errorf("%d rows survived", events.remaining)
	}
}

// A backlog bigger than one run's cap drains over several nights rather than in
// one unbounded loop.
func TestAnnounceLogMaintenance_PruneStopsAtTheRunCap(t *testing.T) {
	rollups := &mockAnnounceRollupRepo{watermark: utcToday()}
	events := &mockAnnounceEventRepo{remaining: announcePruneChunkRows * (announcePruneMaxChunks + 5)}
	deps := &WorkerDeps{
		AnnounceRollupRepo:   rollups,
		AnnounceEventRepo:    events,
		AnnounceLogRetention: func() time.Duration { return 90 * 24 * time.Hour },
	}

	if err := NewAnnounceLogMaintenanceHandler(deps)(context.Background(), nil); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if len(events.calls) != announcePruneMaxChunks {
		t.Errorf("expected the run to stop at %d chunks, got %d", announcePruneMaxChunks, len(events.calls))
	}
	if events.remaining == 0 {
		t.Error("the whole backlog was deleted in one run — the cap is not holding")
	}
}

// A delete failure stops the loop instead of hammering a broken database 200 times.
func TestAnnounceLogMaintenance_PruneStopsOnError(t *testing.T) {
	rollups := &mockAnnounceRollupRepo{watermark: utcToday()}
	events := &mockAnnounceEventRepo{remaining: 100000, deleteErr: errors.New("boom")}
	deps := &WorkerDeps{
		AnnounceRollupRepo:   rollups,
		AnnounceEventRepo:    events,
		AnnounceLogRetention: func() time.Duration { return 90 * 24 * time.Hour },
	}

	if err := NewAnnounceLogMaintenanceHandler(deps)(context.Background(), nil); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if len(events.calls) != 1 {
		t.Errorf("expected the loop to stop after the first failure, got %d calls", len(events.calls))
	}
}

// A cancelled context must abandon the run rather than working through the caps.
func TestAnnounceLogMaintenance_CancelledContextStopsWork(t *testing.T) {
	rollups := &mockAnnounceRollupRepo{watermark: utcToday().AddDate(0, 0, -3650)}
	events := &mockAnnounceEventRepo{remaining: 100000}
	deps := &WorkerDeps{
		AnnounceRollupRepo:   rollups,
		AnnounceEventRepo:    events,
		AnnounceLogRetention: func() time.Duration { return 90 * 24 * time.Hour },
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := NewAnnounceLogMaintenanceHandler(deps)(ctx, nil); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if len(rollups.calls) > 1 {
		t.Errorf("expected the rollup loop to stop after one chunk, got %d", len(rollups.calls))
	}
}

// The catch-up cap bounds a first run over years of history; the watermark it
// leaves behind is what the prune must respect.
func TestAnnounceLogMaintenance_RollupCatchUpIsCapped(t *testing.T) {
	// Ten years of arrears: far more than announceRollupMaxChunks can cover.
	rollups := &mockAnnounceRollupRepo{watermark: utcToday().AddDate(-10, 0, 0), rowsEach: 1}
	events := &mockAnnounceEventRepo{remaining: 1}
	deps := &WorkerDeps{
		AnnounceRollupRepo:   rollups,
		AnnounceEventRepo:    events,
		AnnounceLogRetention: func() time.Duration { return 90 * 24 * time.Hour },
	}

	if err := NewAnnounceLogMaintenanceHandler(deps)(context.Background(), nil); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if len(rollups.calls) != announceRollupMaxChunks {
		t.Errorf("expected the rollup to stop at %d chunks, got %d", announceRollupMaxChunks, len(rollups.calls))
	}
	if rollups.watermark.Equal(utcToday()) {
		t.Error("the mock caught up entirely — the cap is not holding")
	}
	if len(events.calls) != 1 {
		t.Fatalf("expected one prune bounded by the partial watermark, got %d", len(events.calls))
	}
	if !events.calls[0].cutoff.Equal(rollups.watermark) {
		t.Errorf("cutoff = %v, want the partial watermark %v", events.calls[0].cutoff, rollups.watermark)
	}
}

// Retention is the operator's decision about how long to keep personal data. It is
// not a decision to break class promotion, which gap-sums raw announces over its
// own window — so the prune keeps whichever window is longer.
func TestAnnounceLogMaintenance_PruneRespectsWhatOtherFeaturesStillNeed(t *testing.T) {
	rollups := &mockAnnounceRollupRepo{watermark: utcToday()}
	events := &mockAnnounceEventRepo{remaining: 1}
	deps := &WorkerDeps{
		AnnounceRollupRepo: rollups,
		AnnounceEventRepo:  events,
		// An operator shortening retention to a week for privacy reasons...
		AnnounceLogRetention: func() time.Duration { return 7 * 24 * time.Hour },
		// ...while promotion still needs 31 days of raw announces.
		AnnounceLogMinWindow: func() time.Duration { return 31 * 24 * time.Hour },
	}

	if err := NewAnnounceLogMaintenanceHandler(deps)(context.Background(), nil); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if len(events.calls) == 0 {
		t.Fatal("expected a prune")
	}
	cutoff := events.calls[0].cutoff
	if cutoff.After(time.Now().Add(-31 * 24 * time.Hour).Add(time.Minute)) {
		t.Errorf("cutoff %v is inside the 31-day window promotion reads — seed hours would be zeroed", cutoff)
	}
}

// The floor only raises the window, never lowers it: a short promotion window must
// not shorten a long retention setting.
func TestAnnounceLogMaintenance_MinWindowNeverShortensRetention(t *testing.T) {
	rollups := &mockAnnounceRollupRepo{watermark: utcToday()}
	events := &mockAnnounceEventRepo{remaining: 1}
	deps := &WorkerDeps{
		AnnounceRollupRepo:   rollups,
		AnnounceEventRepo:    events,
		AnnounceLogRetention: func() time.Duration { return 90 * 24 * time.Hour },
		AnnounceLogMinWindow: func() time.Duration { return 7 * 24 * time.Hour },
	}

	if err := NewAnnounceLogMaintenanceHandler(deps)(context.Background(), nil); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	wantCutoff := time.Now().Add(-90 * 24 * time.Hour)
	if diff := events.calls[0].cutoff.Sub(wantCutoff); diff < -time.Minute || diff > time.Minute {
		t.Errorf("cutoff = %v, want the 90-day retention cutoff ~%v", events.calls[0].cutoff, wantCutoff)
	}
}

// A zero floor means nothing else reads the raw rows — promotion switched off, say
// — and must not disable pruning.
func TestAnnounceLogMaintenance_ZeroMinWindowStillPrunes(t *testing.T) {
	rollups := &mockAnnounceRollupRepo{watermark: utcToday()}
	events := &mockAnnounceEventRepo{remaining: 3}
	deps := &WorkerDeps{
		AnnounceRollupRepo:   rollups,
		AnnounceEventRepo:    events,
		AnnounceLogRetention: func() time.Duration { return 90 * 24 * time.Hour },
		AnnounceLogMinWindow: func() time.Duration { return 0 },
	}

	if err := NewAnnounceLogMaintenanceHandler(deps)(context.Background(), nil); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if events.remaining != 0 {
		t.Errorf("%d rows past retention survived", events.remaining)
	}
}

// A rollup that cannot make progress has to surface as a failed task. Logging it
// and returning nil is how a job comes to report success every night while the
// table it exists to bound grows without limit — which is the bug this whole job
// was written to fix.
func TestAnnounceLogMaintenance_RollupFailureIsReturnedToAsynq(t *testing.T) {
	rollupErr := errors.New("statement timeout")
	rollups := &mockAnnounceRollupRepo{
		watermark: utcToday().AddDate(0, 0, -400),
		rollupErr: rollupErr,
	}
	events := &mockAnnounceEventRepo{remaining: 1}
	deps := &WorkerDeps{
		AnnounceRollupRepo:   rollups,
		AnnounceEventRepo:    events,
		AnnounceLogRetention: func() time.Duration { return 24 * time.Hour },
	}

	err := NewAnnounceLogMaintenanceHandler(deps)(context.Background(), nil)
	if !errors.Is(err, rollupErr) {
		t.Fatalf("handler returned %v, want the rollup error so the task is marked failed", err)
	}
	// And the prune still ran, bounded by the last good watermark: a failed rollup
	// must not stop housekeeping that is already safe to do.
	if len(events.calls) != 1 {
		t.Errorf("expected the bounded prune to still run, got %d calls", len(events.calls))
	}
}

// A run with no watermark at all is also a failure, and must not prune.
func TestAnnounceLogMaintenance_UnreadableWatermarkIsReturnedToAsynq(t *testing.T) {
	rollups := &mockAnnounceRollupRepo{
		watermark:        utcToday().AddDate(0, 0, -400),
		rollupErr:        errors.New("rollup down"),
		rolledThroughErr: errors.New("watermark unreadable"),
	}
	events := &mockAnnounceEventRepo{remaining: 100}
	deps := &WorkerDeps{
		AnnounceRollupRepo:   rollups,
		AnnounceEventRepo:    events,
		AnnounceLogRetention: func() time.Duration { return 24 * time.Hour },
	}

	if err := NewAnnounceLogMaintenanceHandler(deps)(context.Background(), nil); err == nil {
		t.Fatal("expected an error so the task is marked failed")
	}
	if len(events.calls) != 0 {
		t.Errorf("pruned %d times without a watermark", len(events.calls))
	}
}

// A healthy run reports success, so the failures above are distinguishable.
func TestAnnounceLogMaintenance_HealthyRunReturnsNil(t *testing.T) {
	deps := &WorkerDeps{
		AnnounceRollupRepo:   &mockAnnounceRollupRepo{watermark: utcToday()},
		AnnounceEventRepo:    &mockAnnounceEventRepo{remaining: 2},
		AnnounceLogRetention: func() time.Duration { return 90 * 24 * time.Hour },
	}
	if err := NewAnnounceLogMaintenanceHandler(deps)(context.Background(), nil); err != nil {
		t.Fatalf("healthy run returned %v, want nil", err)
	}
}

func TestAnnounceLogMaintenanceTask_IsWellFormed(t *testing.T) {
	task, err := NewAnnounceLogMaintenanceTask()
	if err != nil {
		t.Fatalf("NewAnnounceLogMaintenanceTask: %v", err)
	}
	if task.Type() != TaskAnnounceLogMaintenance {
		t.Errorf("task type = %q, want %q", task.Type(), TaskAnnounceLogMaintenance)
	}
}

// Every other test here calls the handler directly, which proves nothing about
// whether the worker will ever call it. Migration 079's own header records what
// that costs: announce_log_retention_days was seeded and validated for months, and
// nothing read it. A dropped HandleFunc line in a rebase would reproduce exactly
// that, with every test in this package still green.
func TestAnnounceLogMaintenanceIsRegisteredInTheMux(t *testing.T) {
	task, err := NewAnnounceLogMaintenanceTask()
	if err != nil {
		t.Fatalf("NewAnnounceLogMaintenanceTask: %v", err)
	}

	handler, pattern := NewMux(&WorkerDeps{}).Handler(task)
	if pattern != TaskAnnounceLogMaintenance {
		t.Fatalf("mux has no handler for %q (matched pattern %q)", TaskAnnounceLogMaintenance, pattern)
	}
	// And the registered handler is inert against empty deps rather than panicking
	// the worker process, since that is what a partially wired deployment hands it.
	if err := handler.ProcessTask(context.Background(), task); err != nil {
		t.Errorf("registered handler returned %v with nothing wired, want nil", err)
	}
}
