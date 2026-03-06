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

	return nil
}
