package worker

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/riverqueue/river"
)

// TaskBonusAward is the task type for the hourly bonus point award cycle.
const TaskBonusAward = "bonus:award"

// BonusAwardArgs is the job for one bonus award cycle.
type BonusAwardArgs struct{}

func (BonusAwardArgs) Kind() string { return TaskBonusAward }

func (BonusAwardArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 2} }

// NewBonusAwardHandler returns the handler that runs the bonus award cycle.
func NewBonusAwardHandler(deps *WorkerDeps) func(ctx context.Context, job *river.Job[BonusAwardArgs]) error {
	return func(ctx context.Context, _ *river.Job[BonusAwardArgs]) error {
		if deps.BonusSvc == nil {
			slog.Warn("bonus award: missing service, skipping")
			return nil
		}
		summary, err := deps.BonusSvc.RunAward(ctx)
		if err != nil {
			return fmt.Errorf("bonus award run: %w", err)
		}
		if summary.Skipped {
			slog.Debug("bonus award: run skipped", "reason", summary.Reason)
		}
		return nil
	}
}
