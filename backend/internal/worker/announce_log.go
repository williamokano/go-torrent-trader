package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
)

// TaskAnnounceLogMaintenance is the task type for the nightly announce-log job.
const TaskAnnounceLogMaintenance = "announce_log:maintain"

// Bounds on one run. The job is nightly and self-resuming: whatever it does not
// finish tonight it picks up tomorrow, so the bounds trade catch-up speed for a
// predictable amount of work per run rather than risking a job that never ends.
//
// Both halves are bounded by *work*, not by row count alone. A fixed row budget
// would be a steady-state ceiling as well as a catch-up one: announces accrue in
// proportion to the size of the swarm, so any constant is a number some tracker
// exceeds every day, at which point the job stops keeping up and the table grows
// again with a job that reports success every night.
const (
	// announceRollupChunkDays is how many days one aggregating statement covers.
	// A week, not a month: the GROUP BY produces one or two buckets either way, but
	// the scan underneath it is a week of traffic instead of a month of it. That
	// matters because the statement runs inside the transaction holding the
	// watermark lock — if it trips a statement_timeout the chunk rolls back, the
	// watermark never moves, and a prune clamped to the watermark deletes nothing,
	// every night, forever.
	announceRollupChunkDays = 7
	// announceRollupMaxChunks caps catch-up at roughly two years per run, which is
	// more history than a deployment adopting this is likely to be carrying.
	announceRollupMaxChunks = 110

	// announcePruneChunkRows is how many rows one DELETE removes. Small enough that
	// the row locks and the WAL it generates stay unremarkable.
	announcePruneChunkRows = 5000
	// announcePruneBudget is how long one run spends deleting. This is the real
	// bound: it scales with the machine instead of with a guess about swarm size,
	// so a busy tracker simply deletes more rows in the same ten minutes.
	announcePruneBudget = 10 * time.Minute
	// announcePruneMaxChunks is a runaway backstop, not the working limit — ten
	// million rows is far past anything a night should produce, and reaching it
	// means something is wrong rather than merely busy.
	announcePruneMaxChunks = 2000
)

// NewAnnounceLogMaintenanceTask creates a task for the nightly announce-log job.
func NewAnnounceLogMaintenanceTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskAnnounceLogMaintenance, nil, asynq.MaxRetry(1), asynq.Unique(time.Hour)), nil
}

// NewAnnounceLogMaintenanceHandler returns an asynq handler that keeps the
// announce log bounded: it aggregates closed days into monthly per-user totals,
// then deletes raw rows past the configured retention window.
//
// The order is the safety property. Pruning is capped at the rollup watermark, so
// a raw row can only be deleted once its bytes are counted in user_period_stats.
// If aggregation fails, the watermark does not move and the prune deletes nothing
// — the log grows for another day, which is the recoverable failure. The other
// order loses data permanently.
//
// It runs nightly rather than in the five-minute maintenance sweep because it is
// not lightweight: a first run on an existing deployment aggregates the whole
// history and deletes everything past the window.
func NewAnnounceLogMaintenanceHandler(deps *WorkerDeps) func(ctx context.Context, t *asynq.Task) error {
	return func(ctx context.Context, _ *asynq.Task) error {
		rolledThrough, ok, rollupErr := rollUpAnnounces(ctx, deps)
		if ok {
			pruneAnnounces(ctx, deps, rolledThrough)
		}
		// Returned, not just logged, so a rollup that cannot make progress shows up
		// as a failed task rather than as a nightly success that quietly deletes
		// nothing. A retry is safe: the rollup is additive per closed day and the
		// watermark only moves on commit.
		return rollupErr
	}
}

// rollUpAnnounces aggregates every closed day up to the previous UTC midnight,
// returning the watermark the prune may not cross. ok is false when there is no
// trustworthy watermark, which must stop the prune rather than let it default to
// deleting by age alone. A non-nil error means the run did not finish; the
// watermark it returns may still be usable, so the two are independent.
func rollUpAnnounces(ctx context.Context, deps *WorkerDeps) (time.Time, bool, error) {
	if deps.AnnounceRollupRepo == nil {
		return time.Time{}, false, nil
	}

	// Today is still open — an announce can land in it after we look. Aggregating
	// only through this morning's midnight means every window counted is closed,
	// which is what lets the rollup be additive instead of recomputed.
	now := time.Now().UTC()
	through := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	for i := 0; i < announceRollupMaxChunks; i++ {
		result, err := deps.AnnounceRollupRepo.Rollup(ctx, through, announceRollupChunkDays)
		if err != nil {
			slog.Error("announce log: failed to roll up announces into monthly stats", "error", err)
			// Report the last known-good watermark so the prune stays behind it.
			// Reading it again rather than trusting a partial loop: Rollup is
			// atomic per chunk, so whatever committed is what the row says.
			rolled, rerr := deps.AnnounceRollupRepo.RolledThrough(ctx)
			if rerr != nil {
				// Both halves failed. Logged explicitly, because "pruned nothing
				// because there is no watermark" and "pruned up to the last good
				// watermark" are the same single error line otherwise — and the
				// operator's question is why the table stopped shrinking.
				slog.Error("announce log: rollup failed and the watermark is unreadable, skipping the prune",
					"error", rerr)
				return time.Time{}, false, err
			}
			return rolled, true, err
		}
		if result.Rows > 0 {
			slog.Info("announce log: rolled up announces into monthly stats",
				"from", result.From.Format(time.DateOnly),
				"to", result.To.Format(time.DateOnly),
				"period_rows", result.Rows)
		}
		if result.CaughtUp {
			return result.To, true, nil
		}
		if err := ctx.Err(); err != nil {
			slog.Warn("announce log: rollup interrupted", "error", err)
			return result.To, true, nil
		}
	}

	rolled, err := deps.AnnounceRollupRepo.RolledThrough(ctx)
	if err != nil {
		slog.Error("announce log: failed to read rollup watermark", "error", err)
		return time.Time{}, false, err
	}
	slog.Info("announce log: rollup capped for this run, resuming tomorrow",
		"rolled_through", rolled.Format(time.DateOnly))
	return rolled, true, nil
}

// pruneAnnounces deletes raw announce rows past the retention window, never past
// rolledThrough.
func pruneAnnounces(ctx context.Context, deps *WorkerDeps, rolledThrough time.Time) {
	if deps.AnnounceEventRepo == nil || deps.AnnounceRetention == nil {
		return
	}

	// The resolved window, not the configured one. Other features still read the
	// raw log and the retention setting does not know about them — class promotion
	// gap-sums raw announces over its own seeding window, so an operator
	// shortening retention to a week for privacy reasons would zero every member's
	// seed hours and silently stop promotions. Resolving that is not this
	// function's job: it happens once, in service.ResolveAnnounceRetention, and
	// every surface that reports a window reads the same answer.
	window := deps.AnnounceRetention()

	// A non-positive window disables pruning, the same guard the notification and
	// connector-delivery purges use: without it a misconfigured zero sets the
	// cutoff to now and deletes the entire log. EffectiveDays is zero exactly when
	// the operator disabled pruning — a floor cannot raise a disabled window,
	// because there is nothing to delete for it to protect.
	if window.EffectiveDays <= 0 {
		return
	}
	if window.Overridden() {
		slog.Warn("announce log: retention is shorter than another feature needs, keeping the longer window",
			"configured_days", window.ConfiguredDays,
			"effective_days", window.EffectiveDays,
			"required_by", window.FloorReason)
	}

	cutoff := time.Now().Add(-time.Duration(window.EffectiveDays) * 24 * time.Hour)
	if rolledThrough.Before(cutoff) {
		// Aggregation is behind retention — a first run on a large history, or a
		// run of failures. Delete only what has been counted and say so, because
		// otherwise the log appears to stop shrinking for no visible reason.
		slog.Info("announce log: prune limited by rollup progress",
			"retention_cutoff", cutoff.Format(time.RFC3339),
			"rolled_through", rolledThrough.Format(time.DateOnly))
		cutoff = rolledThrough
	}

	var (
		total         int64
		deadline      = time.Now().Add(announcePruneBudget)
		cutoffCleared bool
	)
	for i := 0; i < announcePruneMaxChunks; i++ {
		deleted, err := deps.AnnounceEventRepo.DeleteOlderThan(ctx, cutoff, announcePruneChunkRows)
		if err != nil {
			slog.Error("announce log: failed to prune old announce events", "error", err)
			break
		}
		total += deleted
		if deleted < announcePruneChunkRows {
			// A short chunk means nothing older than the cutoff is left.
			cutoffCleared = true
			break
		}
		if err := ctx.Err(); err != nil {
			slog.Warn("announce log: prune interrupted", "error", err)
			break
		}
		if !time.Now().Before(deadline) {
			break
		}
	}

	if total > 0 {
		slog.Info("announce log: pruned old announce events",
			"count", total, "cutoff", cutoff.Format(time.RFC3339))
	}
	if !cutoffCleared {
		// Said out loud because this is the shape of the bug this job exists to fix:
		// a run that looks successful while the table keeps growing. If it repeats
		// every night, the site is accruing announces faster than one budgeted run
		// can delete them and the window or the budget needs raising.
		slog.Warn("announce log: prune ended with rows still past the cutoff",
			"deleted", total, "cutoff", cutoff.Format(time.RFC3339))
	}
}
