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

		// 2. Re-enable users whose temporary bans have expired
		if deps.AdminSvc != nil {
			if reEnabled, err := deps.AdminSvc.ReEnableExpiredBans(ctx); err != nil {
				slog.Error("maintenance: failed to re-enable expired bans", "error", err)
			} else if reEnabled > 0 {
				slog.Info("maintenance: re-enabled expired bans", "count", reEnabled)
			}
		}

		// 3. Clean up expired chat mutes and notify users via WebSocket
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

		// 4. Resolve expired privilege restrictions
		if deps.RestrictionSvc != nil {
			if resolved, err := deps.RestrictionSvc.ResolveExpired(ctx); err != nil {
				slog.Error("maintenance: failed to resolve expired restrictions", "error", err)
			} else if resolved > 0 {
				slog.Info("maintenance: resolved expired restrictions", "count", resolved)
			}
		}

		// 5. Purge read notifications past the retention window. A non-positive
		// retention disables the purge — without that guard a misconfigured zero
		// would set the cutoff to now and delete every read notification.
		if deps.NotificationRepo != nil && deps.NotificationRetention > 0 {
			cutoff := time.Now().Add(-deps.NotificationRetention)
			if purged, err := deps.NotificationRepo.DeleteOld(ctx, cutoff); err != nil {
				slog.Error("maintenance: failed to purge old notifications", "error", err)
			} else if purged > 0 {
				slog.Info("maintenance: purged old notifications", "count", purged, "cutoff", cutoff)
			}
		}

		// 6. Prune the connector delivery log. Same non-positive guard as above:
		// a misconfigured zero would set the cutoff to now and wipe the log.
		if deps.ConnectorDeliveryRepo != nil && deps.ConnectorDeliveryRetention != nil {
			retention := deps.ConnectorDeliveryRetention()
			if retention > 0 {
				cutoff := time.Now().Add(-retention)
				if purged, err := deps.ConnectorDeliveryRepo.DeleteOld(ctx, cutoff); err != nil {
					slog.Error("maintenance: failed to purge old connector deliveries", "error", err)
				} else if purged > 0 {
					slog.Info("maintenance: purged old connector deliveries", "count", purged, "cutoff", cutoff)
				}
			}
		}

		// 7. Re-enqueue drains for instances with work that is due. This is the
		// safety net for anything stranded between the delivery row being
		// written and its drain being queued — a crash, or a Redis blip while
		// the dispatcher was enqueuing.
		if deps.ConnectorDeliveryRepo != nil && deps.ConnectorEnqueuer != nil {
			ids, err := deps.ConnectorDeliveryRepo.InstancesWithDue(ctx, time.Now())
			if err != nil {
				slog.Error("maintenance: failed to list connector instances with due deliveries", "error", err)
			} else {
				for _, id := range ids {
					if err := deps.ConnectorEnqueuer.EnqueueConnectorDrain(ctx, id, 0); err != nil {
						slog.Error("maintenance: failed to enqueue connector drain",
							"instance_id", id, "error", err)
					}
				}
			}
		}

		return nil
	}
}
