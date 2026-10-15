package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
)

// Job kinds for the jobs not defined next to their own handler. River calls
// these "kinds"; the strings are kept from the asynq era so logs read the same.
const (
	TaskSendEmail    = "email:send"
	TaskCleanupPeers = "cleanup:peers"
	TaskRecalcStats  = "stats:recalc"
	// TaskRatioWarning is defined in ratio_warning.go
)

const (
	// DefaultConcurrency is how many jobs one process works at once.
	DefaultConcurrency = 10

	// jobTimeout bounds a job that does not set its own: an email, a connector
	// drain (which bounds itself at connectorDrainDeadline anyway). Short on
	// purpose, because River will not rescue a job whose worker died until its
	// timeout has passed (see rescueStuckJobsAfter).
	jobTimeout = 5 * time.Minute

	// longJobTimeout is for the periodic jobs that walk whole tables — cleanup,
	// promotion, the announce rollup and the like. Thirty minutes is what asynq
	// applied to every job, so nothing that finished under asynq starts timing
	// out here.
	longJobTimeout = 30 * time.Minute

	// rescueStuckJobsAfter is when River starts treating a running job as
	// orphaned by a dead worker. River also waits out each job's own timeout
	// before rescuing it (job_rescuer.go: cmp.Or(workUnit.Timeout(), ...)), so a
	// long job is protected by its worker timeout, not by this. Keeping this
	// short is what bounds recovery after a crash: an email (a reset link lives
	// an hour) or a connector drain — whose uniqueness blocks every dispatch for
	// that instance while it sits in running — comes back within minutes.
	rescueStuckJobsAfter = 10 * time.Minute

	// softStopTimeout is how long shutdown waits for running jobs before
	// cancelling their contexts. Cancelled, the connector drain snoozes rather
	// than ending, so it is picked up again at once instead of staying "running"
	// until the rescuer reaches it. It sits inside the 30s main.go gives Stop.
	softStopTimeout = 20 * time.Second

	// completedJobRetention is how long finished jobs stay in river_job. River's
	// default is a day, but email job args carry the message body, and reset and
	// confirmation links in plaintext with it — the very tokens the database
	// otherwise only stores as hashes. An hour is past a reset link's life and
	// long enough to look at a job that just ran.
	completedJobRetention = time.Hour
	// discardedJobRetention keeps jobs that ran out of attempts — the only trace
	// of an email that never went — for a day rather than River's week, for the
	// same reason. The backup also leaves river_job's rows out (BackupService).
	discardedJobRetention = 24 * time.Hour

	// riverLogLevel is the least severe River message that reaches the log.
	// River narrates its own housekeeping at Info (producer job counts every few
	// seconds, each maintenance pass); its standalone default is Warn too.
	riverLogLevel = slog.LevelWarn
)

// JobInserter is the slice of *river.Client the enqueuers need. An interface so
// the options each enqueue carries can be asserted without a database.
type JobInserter interface {
	Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// NewInsertClient creates a River client that only inserts jobs: no queues, no
// workers, no leader election. The HTTP side needs one long before the worker
// dependencies exist — the same split asynq had between Client and Server.
func NewInsertClient(pool *pgxpool.Pool) (*river.Client[pgx.Tx], error) {
	client, err := river.NewClient(newRiverDriver(pool), &river.Config{Logger: riverLogger()})
	if err != nil {
		return nil, fmt.Errorf("creating river insert client: %w", err)
	}
	return client, nil
}

// NewWorkerClient creates the River client that works jobs and, on whichever
// node River elects leader, runs the periodic schedule. Every node runs one:
// leader election is what stops a periodic job firing once per node, which is
// what the ENABLE_SCHEDULER switch and the per-task uniqueness windows used to
// approximate by hand.
func NewWorkerClient(pool *pgxpool.Pool, deps *WorkerDeps, periodic []*river.PeriodicJob, concurrency int) (*river.Client[pgx.Tx], error) {
	workers, err := NewWorkers(deps)
	if err != nil {
		return nil, err
	}
	client, err := river.NewClient(newRiverDriver(pool), workerConfig(workers, periodic, concurrency))
	if err != nil {
		return nil, fmt.Errorf("creating river worker client: %w", err)
	}
	return client, nil
}

func newRiverDriver(pool *pgxpool.Pool) *riverpgxv5.Driver { return riverpgxv5.New(pool) }

func workerConfig(workers *river.Workers, periodic []*river.PeriodicJob, concurrency int) *river.Config {
	return &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: concurrency},
		},
		Workers:                     workers,
		PeriodicJobs:                periodic,
		JobTimeout:                  jobTimeout,
		RescueStuckJobsAfter:        rescueStuckJobsAfter,
		SoftStopTimeout:             softStopTimeout,
		CompletedJobRetentionPeriod: completedJobRetention,
		DiscardedJobRetentionPeriod: discardedJobRetention,
		Logger:                      riverLogger(),
	}
}

// riverLogger is the process logger with River's chatter below riverLogLevel
// filtered out.
func riverLogger() *slog.Logger {
	return slog.New(minLevelHandler{Handler: slog.Default().Handler(), min: riverLogLevel})
}

// minLevelHandler drops records below min, whatever the wrapped handler would
// accept.
type minLevelHandler struct {
	slog.Handler
	min slog.Level
}

func (h minLevelHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.min && h.Handler.Enabled(ctx, level)
}

func (h minLevelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return minLevelHandler{Handler: h.Handler.WithAttrs(attrs), min: h.min}
}

func (h minLevelHandler) WithGroup(name string) slog.Handler {
	return minLevelHandler{Handler: h.Handler.WithGroup(name), min: h.min}
}
