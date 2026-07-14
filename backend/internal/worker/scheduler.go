package worker

import (
	"fmt"

	"github.com/hibiken/asynq"
)

// NewScheduler creates an asynq scheduler for periodic tasks.
func NewScheduler(redisURL string) (*asynq.Scheduler, error) {
	opt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parsing redis URL: %w", err)
	}
	return asynq.NewScheduler(opt, nil), nil
}

// RegisterPeriodicTasks registers all recurring tasks with the scheduler.
func RegisterPeriodicTasks(scheduler *asynq.Scheduler) error {
	// Clean stale peers every 15 minutes.
	cleanupTask, err := NewCleanupPeersTask()
	if err != nil {
		return fmt.Errorf("create cleanup peers task: %w", err)
	}
	if _, err := scheduler.Register("*/15 * * * *", cleanupTask); err != nil {
		return fmt.Errorf("register cleanup peers: %w", err)
	}

	// Recalculate stats every hour.
	statsTask, err := NewRecalcStatsTask()
	if err != nil {
		return fmt.Errorf("create recalc stats task: %w", err)
	}
	if _, err := scheduler.Register("0 * * * *", statsTask); err != nil {
		return fmt.Errorf("register recalc stats: %w", err)
	}

	// Check ratio warnings every 6 hours.
	ratioTask, err := NewRatioWarningTask()
	if err != nil {
		return fmt.Errorf("create ratio warning task: %w", err)
	}
	if _, err := scheduler.Register("0 */6 * * *", ratioTask); err != nil {
		return fmt.Errorf("register ratio warning: %w", err)
	}

	// General maintenance every 5 minutes (expired warnings, flag cleanup, etc.).
	maintenanceTask, err := NewMaintenanceTask()
	if err != nil {
		return fmt.Errorf("create maintenance task: %w", err)
	}
	if _, err := scheduler.Register("*/5 * * * *", maintenanceTask); err != nil {
		return fmt.Errorf("register maintenance: %w", err)
	}

	return nil
}

// RegisterBackupTask registers the periodic database backup on the given cron
// spec (e.g. "0 3 * * *"). It is separate from RegisterPeriodicTasks because
// scheduled backups are opt-in: main only calls this when BACKUP_SCHEDULE_CRON
// is set.
func RegisterBackupTask(scheduler *asynq.Scheduler, cronSpec string) error {
	if cronSpec == "" {
		return nil
	}
	backupTask, err := NewBackupTask()
	if err != nil {
		return fmt.Errorf("create backup task: %w", err)
	}
	if _, err := scheduler.Register(cronSpec, backupTask); err != nil {
		return fmt.Errorf("register backup (cron %q): %w", cronSpec, err)
	}
	return nil
}
