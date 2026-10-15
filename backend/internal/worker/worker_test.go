package worker

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// unreachablePool returns a pool that is never dialled: pgxpool.New connects
// lazily, and neither client below touches the database until it is started or
// asked to insert. That is enough to run River's own config validation, which
// is where a bad combination (a rescue window shorter than the job timeout, a
// duplicate worker) is caught.
func unreachablePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), "postgres://nobody@127.0.0.1:1/none")
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestNewInsertClient(t *testing.T) {
	client, err := NewInsertClient(unreachablePool(t))
	if err != nil {
		t.Fatalf("NewInsertClient: %v", err)
	}
	if client == nil {
		t.Fatal("expected a client")
	}
}

func TestNewWorkerClient(t *testing.T) {
	periodic, err := PeriodicJobs("0 3 * * *")
	if err != nil {
		t.Fatalf("PeriodicJobs: %v", err)
	}
	client, err := NewWorkerClient(unreachablePool(t), &WorkerDeps{}, periodic, DefaultConcurrency)
	if err != nil {
		t.Fatalf("NewWorkerClient: %v", err)
	}
	if client == nil {
		t.Fatal("expected a client")
	}
}

// River's defaults would each break something quietly (tasks/lessons.md): a
// one-minute job timeout, a day's retention of email args that carry reset
// links, no soft stop. Pinned here so a refactor of workerConfig cannot drop
// one without a test going red.
func TestWorkerConfigOverridesRiversDefaults(t *testing.T) {
	cfg := workerConfig(nil, nil, DefaultConcurrency)
	checks := map[string][2]time.Duration{
		"JobTimeout":                  {cfg.JobTimeout, jobTimeout},
		"RescueStuckJobsAfter":        {cfg.RescueStuckJobsAfter, rescueStuckJobsAfter},
		"SoftStopTimeout":             {cfg.SoftStopTimeout, softStopTimeout},
		"CompletedJobRetentionPeriod": {cfg.CompletedJobRetentionPeriod, completedJobRetention},
		"DiscardedJobRetentionPeriod": {cfg.DiscardedJobRetentionPeriod, discardedJobRetention},
	}
	for name, c := range checks {
		if c[0] != c[1] {
			t.Errorf("%s = %s, want %s", name, c[0], c[1])
		}
	}
	// Shutdown gives Stop 30s; the soft stop must cancel jobs inside that, or
	// the process exits with them still marked running.
	if softStopTimeout >= 30*time.Second {
		t.Errorf("softStopTimeout %s does not fit inside main.go's 30s stop budget", softStopTimeout)
	}
	// A reset link lives an hour; a crashed email job must be retried well
	// inside it.
	if rescueStuckJobsAfter+jobTimeout >= time.Hour {
		t.Errorf("a crashed email waits %s for its retry, past a reset link's life", rescueStuckJobsAfter+jobTimeout)
	}
}

// River's rescuer waits out each job's own Timeout before rescuing it, so that
// timeout is what keeps a slow job from being run twice. Each has to cover its
// longest legitimate run — the reindex its two hours, the backup its own
// pg_dump limit, which it does not let River cancel.
func TestLongJobsCarryTimeoutsThatCoverThem(t *testing.T) {
	if got := registeredWorker[AnnounceLogReindexArgs](t, &WorkerDeps{}).Timeout(nil); got != announceReindexTimeout {
		t.Errorf("reindex timeout = %s, want %s", got, announceReindexTimeout)
	}
	if got := registeredWorker[MaintenanceArgs](t, &WorkerDeps{}).Timeout(nil); got != longJobTimeout {
		t.Errorf("maintenance timeout = %s, want %s", got, longJobTimeout)
	}
	if got := registeredWorker[AnnounceLogMaintenanceArgs](t, &WorkerDeps{}).Timeout(nil); got != longJobTimeout {
		t.Errorf("announce log maintenance timeout = %s, want %s", got, longJobTimeout)
	}

	backups, err := service.NewBackupService(service.BackupServiceConfig{
		DatabaseURL: "postgres://u@localhost/db", Dir: t.TempDir(), Timeout: 3 * time.Hour,
	}, nil)
	if err != nil {
		t.Fatalf("NewBackupService: %v", err)
	}
	got := registeredWorker[BackupArgs](t, &WorkerDeps{BackupSvc: backups}).Timeout(nil)
	if got <= 3*time.Hour {
		t.Errorf("backup job timeout %s does not outlast BACKUP_TIMEOUT (3h)", got)
	}
}

func TestRiverLoggerDropsInfo(t *testing.T) {
	logger := riverLogger()
	ctx := context.Background()
	if logger.Enabled(ctx, slog.LevelInfo) {
		t.Error("River's Info messages reach the log")
	}
	if !logger.Enabled(ctx, slog.LevelWarn) {
		t.Error("River's warnings are dropped")
	}
	// The filter survives the With/WithGroup River uses for its components.
	if logger.With("component", "x").WithGroup("g").Enabled(ctx, slog.LevelInfo) {
		t.Error("a derived River logger lets Info through")
	}
}

// registeredWorker returns the worker NewWorkers registers for T's kind: the
// exact one River would run, not a handler built separately for the test.
// Calling a handler directly proves nothing about whether the worker ever
// reaches it.
func registeredWorker[T river.JobArgs](t *testing.T, deps *WorkerDeps) *funcWorker[T] {
	t.Helper()
	var args T
	for _, r := range registrations(deps) {
		if r.kind != args.Kind() {
			continue
		}
		w, ok := r.worker.(*funcWorker[T])
		if !ok {
			t.Fatalf("worker for %q is %T, want *funcWorker[%T]", args.Kind(), r.worker, args)
		}
		return w
	}
	t.Fatalf("no worker registered for %q", args.Kind())
	return nil
}
