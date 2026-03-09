package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
)

// TaskMaintenance is the task type for the general maintenance job.
const TaskMaintenance = "maintenance:run"

// NewMaintenanceTask creates a task for the general maintenance job.
func NewMaintenanceTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskMaintenance, nil, asynq.MaxRetry(1), asynq.Unique(4*time.Minute)), nil
}

// NewMaintenanceHandler returns an asynq handler that performs lightweight
// housekeeping tasks on a frequent interval (every 5 minutes).
func NewMaintenanceHandler(deps *WorkerDeps) func(ctx context.Context, t *asynq.Task) error {
	return func(ctx context.Context, _ *asynq.Task) error {
		// 1. Resolve expired manual warnings
		if deps.WarningSvc != nil {
			if resolved, err := deps.WarningSvc.ResolveExpiredManualWarnings(ctx); err != nil {
				slog.Error("maintenance: failed to resolve expired manual warnings", "error", err)
			} else if resolved > 0 {
				slog.Info("maintenance: resolved expired manual warnings", "count", resolved)
			}
		}

		return nil
	}
}
