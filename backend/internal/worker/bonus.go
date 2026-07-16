package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
)

// TaskBonusAward is the task type for the hourly bonus point award cycle.
const TaskBonusAward = "bonus:award"

// NewBonusAwardTask creates a task for one bonus award cycle. The unique window
// prevents duplicate enqueues when multiple scheduler instances run.
func NewBonusAwardTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskBonusAward, nil, asynq.MaxRetry(1), asynq.Unique(50*time.Minute)), nil
}

// NewBonusAwardHandler returns an asynq handler that runs the bonus award cycle.
func NewBonusAwardHandler(deps *WorkerDeps) func(ctx context.Context, t *asynq.Task) error {
	return func(ctx context.Context, _ *asynq.Task) error {
		if deps.BonusSvc == nil {
			slog.Warn("bonus award: missing service, skipping")
			return nil
		}
		summary, err := deps.BonusSvc.RunAward(ctx)
		if err != nil {
			return fmt.Errorf("bonus award run: %w", err)
		}
		if summary.Skipped {
			slog.Debug("bonus award: run skipped", "reason", summary.Reason)
		}
		return nil
	}
}
