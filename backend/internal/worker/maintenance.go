package worker

import (
	"context"
	"encoding/json"
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

		// 2. Clean up expired chat mutes and notify users via WebSocket
		if deps.ChatSvc != nil {
			unmutedUsers, err := deps.ChatSvc.CleanupExpiredMutes(ctx)
			if err != nil {
				slog.Error("maintenance: failed to clean expired chat mutes", "error", err)
			} else if len(unmutedUsers) > 0 {
				slog.Info("maintenance: cleaned expired chat mutes", "count", len(unmutedUsers))

				// Send unmute WS events to affected users
				if deps.SendToUser != nil {
					payload, _ := json.Marshal(map[string]string{"type": "unmute"})
					for _, userID := range unmutedUsers {
						deps.SendToUser(userID, payload)
					}
				}
			}
		}

		return nil
	}
}
