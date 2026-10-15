package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/riverqueue/river"
)

// TaskDigest is the task type for the notification email digest run.
const TaskDigest = "notification_digest:run"

// DigestArgs is the job for the notification digest engine. The engine
// itself gates each recipient on their own cadence (see
// NotificationDigestService.Run), so this can be scheduled daily and simply
// no-op for users not yet due.
type DigestArgs struct{}

func (DigestArgs) Kind() string { return TaskDigest }

func (DigestArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 2} }

// NewDigestHandler returns the handler that runs the notification digest engine.
func NewDigestHandler(deps *WorkerDeps) func(ctx context.Context, job *river.Job[DigestArgs]) error {
	return func(ctx context.Context, _ *river.Job[DigestArgs]) error {
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
