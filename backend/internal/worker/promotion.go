package worker

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/riverqueue/river"
)

// TaskPromotion is the task type for the auto class promotion/demotion run.
const TaskPromotion = "promotion:run"

// PromotionArgs is the job for the promotion engine. The engine itself
// enforces the configurable interval, so this can be scheduled daily and simply
// no-op between cycles.
type PromotionArgs struct{}

func (PromotionArgs) Kind() string { return TaskPromotion }

func (PromotionArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 2} }

// NewPromotionHandler returns the handler that runs the promotion engine.
func NewPromotionHandler(deps *WorkerDeps) func(ctx context.Context, job *river.Job[PromotionArgs]) error {
	return func(ctx context.Context, _ *river.Job[PromotionArgs]) error {
		if deps.PromotionSvc == nil {
			slog.Warn("promotion: missing service, skipping")
			return nil
		}
		summary, err := deps.PromotionSvc.Run(ctx, false)
		if err != nil {
			return fmt.Errorf("promotion run: %w", err)
		}
		if summary.Skipped {
			slog.Debug("promotion: run skipped", "reason", summary.Reason)
		}
		return nil
	}
}
