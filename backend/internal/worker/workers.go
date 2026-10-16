package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/riverqueue/river"
)

// funcWorker adapts a handler function to river.Worker, so each job's logic
// stays a plain func(ctx, job) that tests can call directly.
type funcWorker[T river.JobArgs] struct {
	river.WorkerDefaults[T]
	fn      func(ctx context.Context, job *river.Job[T]) error
	timeout time.Duration
}

func (w *funcWorker[T]) Work(ctx context.Context, job *river.Job[T]) error {
	return w.fn(ctx, job)
}

// Timeout overrides the client-wide jobTimeout when set. Zero defers to it. It
// is also how long River's rescuer leaves the job alone, so it must cover the
// job's longest legitimate run.
func (w *funcWorker[T]) Timeout(*river.Job[T]) time.Duration {
	return w.timeout
}

// registration is one job kind and how to add its worker.
type registration struct {
	kind string
	// worker is the *funcWorker[T] that add registers, kept so a test can reach
	// the exact worker River would run rather than a handler built separately.
	worker any
	add    func(*river.Workers) error
}

func register[T river.JobArgs](fn func(context.Context, *river.Job[T]) error) registration {
	return registerWithTimeout(fn, 0)
}

func registerWithTimeout[T river.JobArgs](fn func(context.Context, *river.Job[T]) error, timeout time.Duration) registration {
	var args T
	worker := &funcWorker[T]{fn: fn, timeout: timeout}
	return registration{
		kind:   args.Kind(),
		worker: worker,
		add: func(w *river.Workers) error {
			return river.AddWorkerSafely(w, worker)
		},
	}
}

// registrations lists every job kind the worker handles. A kind that is
// enqueued or scheduled but missing here sits in river_job unworked, so
// TestEveryScheduledKindHasAWorker checks the two lists against each other.
func registrations(deps *WorkerDeps) []registration {
	return []registration{
		register(NewSendEmailHandler(deps)),
		register(NewConnectorDrainHandler(deps)),
		registerWithTimeout(NewCleanupHandler(deps), longJobTimeout),
		registerWithTimeout(NewRatioWarningHandler(deps), longJobTimeout),
		registerWithTimeout(NewMaintenanceHandler(deps), longJobTimeout),
		registerWithTimeout(NewBackupHandler(deps), backupJobTimeout(deps)),
		registerWithTimeout(NewPromotionHandler(deps), longJobTimeout),
		registerWithTimeout(NewBonusAwardHandler(deps), longJobTimeout),
		registerWithTimeout(NewInviteDistributionHandler(deps), longJobTimeout),
		registerWithTimeout(NewDigestHandler(deps), longJobTimeout),
		registerWithTimeout(NewAnnounceLogMaintenanceHandler(deps), longJobTimeout),
		registerWithTimeout(NewAnnounceLogReindexHandler(deps), announceReindexTimeout),
		registerWithTimeout(NewHnREvaluateHandler(deps), longJobTimeout),
	}
}

// NewWorkers returns the River worker bundle with every job handler registered.
// deps provides the repositories and services the handlers call.
func NewWorkers(deps *WorkerDeps) (*river.Workers, error) {
	workers := river.NewWorkers()
	for _, r := range registrations(deps) {
		if err := r.add(workers); err != nil {
			return nil, fmt.Errorf("registering worker for %q: %w", r.kind, err)
		}
	}
	return workers, nil
}
