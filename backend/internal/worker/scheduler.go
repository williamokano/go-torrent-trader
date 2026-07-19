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

	// Evaluate class promotions daily. The engine gates on the configurable
	// interval and enable flag, so a fixed daily schedule is fine.
	promotionTask, err := NewPromotionTask()
	if err != nil {
		return fmt.Errorf("create promotion task: %w", err)
	}
	if _, err := scheduler.Register("0 5 * * *", promotionTask); err != nil {
		return fmt.Errorf("register promotion: %w", err)
	}

	// Award bonus points hourly (offset from the stats job at minute 0). The
	// service no-ops while bonus_enabled is off.
	bonusTask, err := NewBonusAwardTask()
	if err != nil {
		return fmt.Errorf("create bonus award task: %w", err)
	}
	if _, err := scheduler.Register("30 * * * *", bonusTask); err != nil {
		return fmt.Errorf("register bonus award: %w", err)
	}

	// Evaluate invite distribution daily, offset from the promotion job so
	// they don't fire at the identical instant. The engine gates on its own
	// configurable interval and enable flag, so a fixed daily schedule is fine.
	inviteDistributionTask, err := NewInviteDistributionTask()
	if err != nil {
		return fmt.Errorf("create invite distribution task: %w", err)
	}
	if _, err := scheduler.Register("30 5 * * *", inviteDistributionTask); err != nil {
		return fmt.Errorf("register invite distribution: %w", err)
	}

	// Evaluate the notification email digest daily, offset an hour past
	// promotion/invite distribution so it doesn't contend with them. The
	// engine gates each recipient independently on their own daily/weekly
	// cadence (see NotificationDigestService.Run), so one fixed daily
	// schedule covers both frequencies.
	digestTask, err := NewDigestTask()
	if err != nil {
		return fmt.Errorf("create notification digest task: %w", err)
	}
	if _, err := scheduler.Register("0 6 * * *", digestTask); err != nil {
		return fmt.Errorf("register notification digest: %w", err)
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
