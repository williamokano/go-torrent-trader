package worker

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/riverqueue/river"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
)

// TaskHnREvaluate is the task type for the hit-and-run daemon sweep.
const TaskHnREvaluate = "hnr:evaluate"

// HnREvaluateArgs is the job for the HnR daemon. The service's own two-stage
// advisory lock (HnRService.RunDaemon) is what makes concurrent invocations —
// a retry, a manual "run now", or a slow run meeting the next tick — safe.
type HnREvaluateArgs struct{}

func (HnREvaluateArgs) Kind() string { return TaskHnREvaluate }

func (HnREvaluateArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 2} }

// NewHnREvaluateHandler returns the handler that runs the HnR daemon
// sweep. Skipping (another instance already queued) is not an error — it is
// the lock working as intended, so it is logged at Info and swallowed rather
// than surfaced as a task failure that would trigger a retry.
func NewHnREvaluateHandler(deps *WorkerDeps) func(ctx context.Context, job *river.Job[HnREvaluateArgs]) error {
	return func(ctx context.Context, _ *river.Job[HnREvaluateArgs]) error {
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
