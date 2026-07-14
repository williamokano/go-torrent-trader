package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"

	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// TaskBackupDatabase is the task type for the scheduled database backup job.
const TaskBackupDatabase = "backup:database"

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
		backup, err := deps.BackupSvc.Create(ctx, service.SystemActor)
		if err != nil {
			// An admin kicked off a backup by hand at the same moment. That is
			// not a failure: retrying would just queue a redundant dump.
			if errors.Is(err, service.ErrBackupInProgress) {
				slog.Info("scheduled backup skipped: a backup is already running")
				return nil
			}
			return fmt.Errorf("scheduled backup: %w", err)
		}
		slog.Info("scheduled backup created", "name", backup.Name, "size", backup.Size)
		return nil
	}
}
