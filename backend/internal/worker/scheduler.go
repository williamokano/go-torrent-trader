package worker

import (
	"fmt"
	"strings"
	"time"

	"github.com/riverqueue/river"
	"github.com/robfig/cron/v3"
)

// periodicSpec is one recurring job: a standard five-field cron spec,
// evaluated in UTC, and the args it inserts each time it fires.
type periodicSpec struct {
	cron string
	args river.JobArgs
}

// periodicSpecs lists every recurring job. Kept as plain data, separate from
// the river.PeriodicJob values built from it, because River's are opaque: this
// is what lets a test check that each job is actually scheduled, and when. A
// recurring job nobody schedules is as dead as a handler nobody registers, and
// quieter — every handler test still passes.
//
// Under asynq every node that ran a scheduler enqueued its own copy, and
// Unique() collapsed them. River runs the schedule only on its elected leader,
// so each tick normally inserts exactly one job. PeriodicJobs still makes each
// insert unique within its own period, for the ticks that can fire twice
// anyway: a leader handover between nodes whose clocks disagree, a crash
// mid-insert. That matters most for the bonus award, which is not idempotent —
// a second run in the same hour pays members twice.
//
// That uniqueness is by period, never by state alone: a run still going (or
// left "running" by a crashed node) does not stop the next period's tick.
// Overlap between a slow run and the next tick is guarded where it matters, by
// the advisory locks in HnRService.RunDaemon and AnnounceEventRepo.Reindex, and
// BackupService's in-progress check.
//
// backupCron is BACKUP_SCHEDULE_CRON: scheduled backups are opt-in, so the
// backup job is only added when it is set.
func periodicSpecs(backupCron string) []periodicSpec {
	specs := []periodicSpec{
		// Clean stale peers every 15 minutes.
		{"*/15 * * * *", CleanupPeersArgs{}},

		// Check ratio warnings every 6 hours.
		{"0 */6 * * *", RatioWarningArgs{}},

		// General maintenance every 5 minutes (expired warnings, flag cleanup, etc.).
		{"*/5 * * * *", MaintenanceArgs{}},

		// Evaluate class promotions daily. The engine gates on the configurable
		// interval and enable flag, so a fixed daily schedule is fine.
		{"0 5 * * *", PromotionArgs{}},

		// Award bonus points hourly (offset from the stats job at minute 0). The
		// service no-ops while bonus_enabled is off.
		{"30 * * * *", BonusAwardArgs{}},

		// Evaluate invite distribution daily, offset from the promotion job so
		// they don't fire at the identical instant. The engine gates on its own
		// configurable interval and enable flag, so a fixed daily schedule is fine.
		{"30 5 * * *", InviteDistributionArgs{}},

		// Evaluate the notification email digest daily, offset an hour past
		// promotion/invite distribution so it doesn't contend with them. The
		// engine gates each recipient independently on their own daily/weekly
		// cadence (see NotificationDigestService.Run), so one fixed daily
		// schedule covers both frequencies.
		{"0 6 * * *", DigestArgs{}},

		// Roll up and prune the announce log nightly, before the promotion job at 05:00
		// so the two bulk-write jobs do not overlap. Hours past midnight UTC on
		// purpose: the rollup only aggregates days that have closed, and the gap makes
		// certain that every announce stamped just before midnight has committed.
		{"15 4 * * *", AnnounceLogMaintenanceArgs{}},

		// Evaluate hit-and-run obligations hourly, at :45 — clear of the other
		// jobs, which sit at :00, :15, :30, and the nightly/monthly window between
		// 04:15 and 06:00. hnr_records only ever gains rows when the announce
		// path's own cached HnREnabled() check passes, so on a site that hasn't
		// turned the feature on this is a cheap scan of an empty table, not a
		// second place the master switch is read. The service's two-stage
		// advisory lock is what keeps a slow run and the next tick from
		// overlapping.
		{"45 * * * *", HnREvaluateArgs{}},

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
		// backup, so a rebuild that runs past two hours meets it; and the job's own
		// two-hour timeout is measured from when a worker picks the job up, not from
		// 01:00, so a worker restarted at 03:30 shifts the whole window. What makes
		// those survivable is the advisory lock in AnnounceEventRepo.Reindex rather
		// than the schedule: overlapping runs skip instead of colliding.
		{"0 1 1 * *", AnnounceLogReindexArgs{}},
	}
	if backupCron != "" {
		specs = append(specs, periodicSpec{backupCron, BackupArgs{}})
	}
	return specs
}

// PeriodicJobs builds River's periodic job list. It fails on a cron spec that
// does not parse, which in practice means a malformed BACKUP_SCHEDULE_CRON:
// better to refuse to start than to boot with backups silently off.
func PeriodicJobs(backupCron string) ([]*river.PeriodicJob, error) {
	specs := periodicSpecs(backupCron)
	jobs := make([]*river.PeriodicJob, 0, len(specs))
	for _, spec := range specs {
		schedule, err := parseCron(spec.cron)
		if err != nil {
			return nil, fmt.Errorf("schedule for %q (cron %q): %w", spec.args.Kind(), spec.cron, err)
		}
		args := spec.args
		opts := &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: uniquePeriod(schedule)}}
		jobs = append(jobs, river.NewPeriodicJob(
			schedule,
			func() (river.JobArgs, *river.InsertOpts) { return args, opts },
			&river.PeriodicJobOpts{ID: args.Kind()},
		))
	}
	return jobs, nil
}

// maxUniquePeriod caps the dedupe window. Completed jobs are pruned after
// completedJobRetention, so a longer window would not hold anyway; a day covers
// every daily job and is still far shorter than the monthly reindex's gap.
const maxUniquePeriod = 24 * time.Hour

// uniquePeriod is the gap between a schedule's runs, capped at maxUniquePeriod:
// the window inside which a second insert of the same job is a duplicate.
// River buckets by truncating to multiples of it, so a "*/15" job dedupes per
// quarter hour and a daily one per UTC day.
func uniquePeriod(schedule cron.Schedule) time.Duration {
	first := schedule.Next(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	gap := schedule.Next(first).Sub(first)
	return min(max(gap, time.Second), maxUniquePeriod)
}

// parseCron parses a standard five-field spec pinned to UTC. asynq's scheduler
// ran in UTC, and the comments above reason in UTC ("hours past midnight UTC on
// purpose"); robfig/cron would otherwise use the process's local zone.
func parseCron(spec string) (cron.Schedule, error) {
	schedule, err := cron.ParseStandard(spec)
	if err != nil {
		return nil, err
	}
	// An explicit CRON_TZ=/TZ= prefix (allowed in BACKUP_SCHEDULE_CRON) wins.
	explicitZone := strings.HasPrefix(spec, "CRON_TZ=") || strings.HasPrefix(spec, "TZ=")
	if s, ok := schedule.(*cron.SpecSchedule); ok && !explicitZone {
		s.Location = time.UTC
	}
	return schedule, nil
}
