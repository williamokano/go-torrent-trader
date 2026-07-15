package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
)

// TaskPromotion is the task type for the auto class promotion/demotion run.
const TaskPromotion = "promotion:run"

// NewPromotionTask creates a task for the promotion engine. The unique window
// prevents duplicate enqueues; the engine itself enforces the configurable
// interval, so this can be scheduled daily and simply no-op between cycles.
func NewPromotionTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskPromotion, nil, asynq.MaxRetry(1), asynq.Unique(23*time.Hour)), nil
}

// NewPromotionHandler returns an asynq handler that runs the promotion engine.
func NewPromotionHandler(deps *WorkerDeps) func(ctx context.Context, t *asynq.Task) error {
	return func(ctx context.Context, _ *asynq.Task) error {
		if deps.PromotionSvc == nil {
			slog.Warn("promotion: missing service, skipping")
			return nil
		}
		summary, err := deps.PromotionSvc.Run(ctx, false)
		if err != nil {
			return fmt.Errorf("promotion run: %w", err)
		}
		if summary.Skipped {
			slog.Debug("promotion: run skipped", "reason", summary.Reason)
		}
		return nil
	}
}
