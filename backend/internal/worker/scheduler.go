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

// TaskScheduler is the slice of *asynq.Scheduler that RegisterPeriodicTasks
// uses. It is an interface so the registrations themselves can be asserted:
// asynq only publishes its entries to Redis once the scheduler is running, so
// with the concrete type there is no way to check that a job is scheduled at all
// short of standing the whole thing up. A recurring job nobody schedules is as
// dead as a handler nobody registers, and quieter — every handler test still
// passes.
type TaskScheduler interface {
	Register(cronspec string, task *asynq.Task, opts ...asynq.Option) (string, error)
}

// RegisterPeriodicTasks registers all recurring tasks with the scheduler.
func RegisterPeriodicTasks(scheduler TaskScheduler) error {
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

	// Roll up and prune the announce log nightly, before the promotion job at 05:00
	// so the two bulk-write jobs do not overlap. Hours past midnight UTC on
	// purpose: the rollup only aggregates days that have closed, and the gap makes
	// certain that every announce stamped just before midnight has committed.
	announceLogTask, err := NewAnnounceLogMaintenanceTask()
	if err != nil {
		return fmt.Errorf("create announce log maintenance task: %w", err)
	}
	if _, err := scheduler.Register("15 4 * * *", announceLogTask); err != nil {
		return fmt.Errorf("register announce log maintenance: %w", err)
	}

	// Evaluate hit-and-run obligations hourly, at :45 — clear of the other
	// jobs, which sit at :00, :15, :30, and the nightly/monthly window between
	// 04:15 and 06:00. hnr_records only ever gains rows when the announce
	// path's own cached HnREnabled() check passes, so on a site that hasn't
	// turned the feature on this is a cheap scan of an empty table, not a
	// second place the master switch is read. The service's two-stage
	// advisory lock (not asynq.Unique) is what keeps concurrent invocations
	// across multiple worker processes safe.
	hnrTask, err := NewHnREvaluateTask()
	if err != nil {
		return fmt.Errorf("create hnr evaluate task: %w", err)
	}
	if _, err := scheduler.Register("45 * * * *", hnrTask); err != nil {
		return fmt.Errorf("register hnr evaluate: %w", err)
	}

	// Rebuild the announce log's indexes on the first of the month. The nightly
	// prune keeps the heap at a fixed size but cannot keep the indexes there, so
	// without this they grow indefinitely (#259).
	//
	// 01:00 puts it clear of the heavy jobs, which all sit between 04:15 and
	// 06:00. That gap matters more here than for the other jobs: a rebuild is the
	// only one whose runtime scales with the entire table, so it is the only one
	// likely to still be going when the next job starts.
	//
	// It is only a gap, not a guarantee. stack.env.example suggests 03:00 for the
	// backup, so a rebuild that runs past two hours meets it; and the task's own
	// two-hour timeout is measured from when a worker picks the job up, not from
	// 01:00, so a worker restarted at 03:30 shifts the whole window. What makes
	// those survivable is the advisory lock in AnnounceEventRepo.Reindex rather
	// than the schedule: overlapping runs skip instead of colliding.
	announceReindexTask, err := NewAnnounceLogReindexTask()
	if err != nil {
		return fmt.Errorf("create announce log reindex task: %w", err)
	}
	if _, err := scheduler.Register("0 1 1 * *", announceReindexTask); err != nil {
		return fmt.Errorf("register announce log reindex: %w", err)
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
