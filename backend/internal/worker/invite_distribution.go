package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
)

// TaskInviteDistribution is the task type for the auto invite distribution run.
const TaskInviteDistribution = "invite_distribution:run"

// NewInviteDistributionTask creates a task for the invite distribution engine.
// The unique window prevents duplicate enqueues; the engine itself enforces
// the configurable interval, so this can be scheduled daily and simply no-op
// between cycles.
func NewInviteDistributionTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskInviteDistribution, nil, asynq.MaxRetry(1), asynq.Unique(23*time.Hour)), nil
}

// NewInviteDistributionHandler returns an asynq handler that runs the invite
// distribution engine.
func NewInviteDistributionHandler(deps *WorkerDeps) func(ctx context.Context, t *asynq.Task) error {
	return func(ctx context.Context, _ *asynq.Task) error {
		if deps.InviteDistributionSvc == nil {
			slog.Warn("invite distribution: missing service, skipping")
			return nil
		}
		summary, err := deps.InviteDistributionSvc.Run(ctx, false)
		if err != nil {
			return fmt.Errorf("invite distribution run: %w", err)
		}
		if summary.Skipped {
			slog.Debug("invite distribution: run skipped", "reason", summary.Reason)
		}
		return nil
	}
}
