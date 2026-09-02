package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// TaskHnREvaluate is the task type for the hit-and-run daemon sweep.
const TaskHnREvaluate = "hnr:evaluate"

// NewHnREvaluateTask creates a task for the HnR daemon. The unique window is
// shorter than the schedule interval (hourly), the same relationship
// promotion's daily task has to its 23h window: it only guards against a
// literal duplicate enqueue in the same cycle. The service's own two-stage
// advisory lock (HnRService.RunDaemon) is what actually makes concurrent
// invocations — from asynq retries, a manual "run now", or another worker
// process entirely — safe, not this.
func NewHnREvaluateTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskHnREvaluate, nil, asynq.MaxRetry(1), asynq.Unique(55*time.Minute)), nil
}

// NewHnREvaluateHandler returns an asynq handler that runs the HnR daemon
// sweep. Skipping (another instance already queued) is not an error — it is
// the lock working as intended, so it is logged at Info and swallowed rather
// than surfaced as a task failure that would trigger a retry.
func NewHnREvaluateHandler(deps *WorkerDeps) func(ctx context.Context, t *asynq.Task) error {
	return func(ctx context.Context, _ *asynq.Task) error {
		if deps.HnRSvc == nil {
			slog.Warn("hnr: missing service, skipping")
			return nil
		}
		summary, err := deps.HnRSvc.RunDaemon(ctx, model.HnRRunTriggerSchedule, nil)
		if err != nil {
			return fmt.Errorf("hnr run: %w", err)
		}
		if summary.Skipped {
			slog.Info("hnr: run skipped, another instance already queued")
			return nil
		}
		slog.Info("hnr: run complete",
			"run_id", summary.RunID,
			"scanned", summary.Counts.Scanned,
			"breached", summary.Counts.Breached,
			"satisfied", summary.Counts.Satisfied,
		)
		return nil
	}
}
