package worker

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/riverqueue/river"
)

// TaskInviteDistribution is the task type for the auto invite distribution run.
const TaskInviteDistribution = "invite_distribution:run"

// InviteDistributionArgs is the job for the invite distribution engine.
// The engine itself enforces
// the configurable interval, so this can be scheduled daily and simply no-op
// between cycles.
type InviteDistributionArgs struct{}

func (InviteDistributionArgs) Kind() string { return TaskInviteDistribution }

func (InviteDistributionArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 2} }

// NewInviteDistributionHandler returns the handler that runs the invite
// distribution engine.
func NewInviteDistributionHandler(deps *WorkerDeps) func(ctx context.Context, job *river.Job[InviteDistributionArgs]) error {
	return func(ctx context.Context, _ *river.Job[InviteDistributionArgs]) error {
		if deps.InviteDistributionSvc == nil {
			slog.Warn("invite distribution: missing service, skipping")
			return nil
		}
		summary, err := deps.InviteDistributionSvc.Run(ctx, false)
		if err != nil {
			return fmt.Errorf("invite distribution run: %w", err)
		}
		if summary.Skipped {
			slog.Debug("invite distribution: run skipped", "reason", summary.Reason)
		}
		return nil
	}
}
