package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// TaskBackupDatabase is the task type for the scheduled database backup job.
const TaskBackupDatabase = "backup:database"

// BackupArgs is the job that runs a pg_dump backup. It is only scheduled when
// BACKUP_SCHEDULE_CRON is set (see periodicSpecs).
type BackupArgs struct{}

func (BackupArgs) Kind() string { return TaskBackupDatabase }

func (BackupArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 2} }

// NewBackupHandler returns the handler that creates a database backup.
// Old backups beyond the configured retention are pruned by the service.
func NewBackupHandler(deps *WorkerDeps) func(ctx context.Context, job *river.Job[BackupArgs]) error {
	return func(ctx context.Context, _ *river.Job[BackupArgs]) error {
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

// backupJobTimeout outlasts the backup's own pg_dump limit (BACKUP_TIMEOUT).
// The dump deliberately ignores cancellation (BackupService.Create detaches its
// context), so a job timeout shorter than the dump would not stop it — it would
// only let River's rescuer decide the job was orphaned and start a second dump
// on another node while the first is still writing.
func backupJobTimeout(deps *WorkerDeps) time.Duration {
	if deps.BackupSvc == nil {
		return longJobTimeout
	}
	return deps.BackupSvc.Timeout() + 5*time.Minute
}
