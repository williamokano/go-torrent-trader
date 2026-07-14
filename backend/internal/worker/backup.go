package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"

	"github.com/williamokano/go-torrent-trader/backend/internal/event"
)

// TaskBackupDatabase is the task type for the scheduled database backup job.
const TaskBackupDatabase = "backup:database"

// backupActor identifies scheduled backups in the activity log — they have no
// human actor behind them.
var backupActor = event.Actor{Username: "system"}

// NewBackupTask creates a task that runs a pg_dump backup.
// The unique window is short (5 minutes) so it only de-duplicates enqueues from
// multiple scheduler instances, without suppressing a legitimately frequent cron.
func NewBackupTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskBackupDatabase, nil, asynq.MaxRetry(1), asynq.Unique(5*time.Minute)), nil
}

// NewBackupHandler returns an asynq handler that creates a database backup.
// Old backups beyond the configured retention are pruned by the service.
func NewBackupHandler(deps *WorkerDeps) func(ctx context.Context, t *asynq.Task) error {
	return func(ctx context.Context, _ *asynq.Task) error {
		if deps.BackupSvc == nil {
			slog.Warn("scheduled backup: backup service not configured, skipping")
			return nil
		}
		backup, err := deps.BackupSvc.Create(ctx, backupActor)
		if err != nil {
			return fmt.Errorf("scheduled backup: %w", err)
		}
		slog.Info("scheduled backup created", "name", backup.Name, "size", backup.Size)
		return nil
	}
}
