package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
)

// TaskAnnounceLogReindex is the task type for the monthly announce-log rebuild.
const TaskAnnounceLogReindex = "announce_log:reindex"

// announceReindexTimeout bounds one rebuild. Generous on purpose: REINDEX
// CONCURRENTLY cannot be chunked or resumed the way the prune can, so a timeout
// that fires mid-rebuild does not save work — it throws the rebuild away and
// leaves an invalid index for the next run to clear. Two hours is far past what
// the measurement in #259 suggests any plausible deployment needs, and the point
// of the bound is to stop a wedged statement being held forever, not to ration.
const announceReindexTimeout = 2 * time.Hour

// NewAnnounceLogReindexTask creates a task for the monthly announce-log rebuild.
//
// MaxRetry(0): a failed rebuild should wait for next month rather than retry.
// Retrying immediately would re-enter a job that just took a long time and
// failed, most likely for a reason that has not gone away — disk, or a lock it
// could not get — and each attempt leaves another invalid index behind.
func NewAnnounceLogReindexTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskAnnounceLogReindex, nil,
		asynq.MaxRetry(0),
		asynq.Timeout(announceReindexTimeout),
		asynq.Unique(announceReindexTimeout),
	), nil
}

// NewAnnounceLogReindexHandler returns an asynq handler that rebuilds the
// announce log's indexes.
//
// This exists because the nightly prune cannot keep them in shape and no amount
// of tuning will make it. Measured over 45 nights of full steady-state churn
// (#259): the heap does not drift by a single byte, because the pages the delete
// frees are reused exactly by the next night's inserts — but the indexes grew by
// roughly the full entry width of every row removed and had not begun to
// plateau. Two of the three indexes lead on a monotonically increasing value,
// and the prune deletes the oldest rows, so it empties the left edge of those
// B-trees while every announce appends to the right. Autovacuum was keeping up
// throughout and does not address it.
//
// Monthly rather than nightly: the growth is gradual, and a rebuild is the one
// piece of announce-log maintenance whose cost scales with the whole table
// rather than with a day of it.
//
// Deliberately not gated on announce_log_enabled, unlike the capture path. That
// setting stops new announces being recorded; it does not stop the prune, which
// keeps deleting from whatever is already there — and deleting is what causes
// the index bloat. A site that turned capture off still has a shrinking table
// whose indexes need rebuilding, and on an empty one the rebuild costs nothing.
func NewAnnounceLogReindexHandler(deps *WorkerDeps) func(ctx context.Context, t *asynq.Task) error {
	return func(ctx context.Context, _ *asynq.Task) error {
		if deps.AnnounceEventRepo == nil {
			return nil
		}

		start := time.Now()
		result, err := deps.AnnounceEventRepo.Reindex(ctx)

		// Reported before the error is handled, because a run that cleaned up
		// after an earlier failure and then failed again has still done something
		// worth knowing about.
		if result.LeftoversDropped > 0 {
			slog.Warn("announce log: dropped invalid indexes left by a previous failed rebuild",
				"count", result.LeftoversDropped)
		}
		if err != nil {
			// Returned as well as logged so it surfaces as a failed task. An
			// operator who never learns the rebuild stopped working finds out from
			// the disk usage instead, months later.
			slog.Error("announce log: failed to rebuild indexes", "error", err)
			return err
		}
		if result.Skipped {
			// Not an error: a rebuild is already running. Said out loud anyway,
			// because "the job ran and did nothing" and "the job did not run" look
			// identical in a log that stays silent, and if it repeats every month
			// something is holding the lock that should not be.
			slog.Warn("announce log: another rebuild is already running, skipped this one")
			return nil
		}

		slog.Info("announce log: rebuilt indexes",
			"took", time.Since(start).Round(time.Second),
			"bytes_before", result.BytesBefore,
			"bytes_after", result.BytesAfter,
			"bytes_recovered", result.BytesBefore-result.BytesAfter)
		return nil
	}
}
