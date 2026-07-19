package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
)

// TaskDigest is the task type for the notification email digest run.
const TaskDigest = "notification_digest:run"

// NewDigestTask creates a task for the notification digest engine. The unique
// window prevents duplicate enqueues from multiple scheduler instances; the
// engine itself gates each recipient on their own cadence (see
// NotificationDigestService.Run), so this can be scheduled daily and simply
// no-op for users not yet due.
func NewDigestTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskDigest, nil, asynq.MaxRetry(1), asynq.Unique(23*time.Hour)), nil
}

// NewDigestHandler returns an asynq handler that runs the notification digest engine.
func NewDigestHandler(deps *WorkerDeps) func(ctx context.Context, t *asynq.Task) error {
	return func(ctx context.Context, _ *asynq.Task) error {
		if deps.DigestSvc == nil {
			slog.Warn("notification digest: missing service, skipping")
			return nil
		}
		summary, err := deps.DigestSvc.Run(ctx, time.Now())
		if err != nil {
			return fmt.Errorf("notification digest run: %w", err)
		}
		slog.Info("notification digest run complete", "recipients", summary.Recipients, "emailed", summary.Emailed)
		return nil
	}
}
