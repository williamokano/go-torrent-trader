package worker

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/hibiken/asynq"
)

// Task type constants for job types.
const (
	TaskSendEmail    = "email:send"
	TaskCleanupPeers = "cleanup:peers"
	TaskRecalcStats  = "stats:recalc"
	// TaskRatioWarning is defined in ratio_warning.go
)

// NewClient creates an asynq client for enqueueing jobs.
func NewClient(redisURL string) (*asynq.Client, error) {
	opt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parsing redis URL: %w", err)
	}
	return asynq.NewClient(opt), nil
}

// NewServer creates an asynq server for processing jobs.
func NewServer(redisURL string, concurrency int) (*asynq.Server, error) {
	opt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parsing redis URL: %w", err)
	}
	srv := asynq.NewServer(opt, asynq.Config{
		Concurrency: concurrency,
		Queues: map[string]int{
			"critical": 6,
			"default":  3,
			"low":      1,
		},
		Logger: newSlogAdapter(slog.Default()),
	})
	return srv, nil
}

// slogAdapter adapts slog.Logger to the asynq.Logger interface.
type slogAdapter struct {
	logger *slog.Logger
}

func newSlogAdapter(logger *slog.Logger) *slogAdapter {
	return &slogAdapter{logger: logger}
}

func (a *slogAdapter) Debug(args ...interface{}) {
	a.logger.Debug(fmt.Sprint(args...))
}

func (a *slogAdapter) Info(args ...interface{}) {
	a.logger.Info(fmt.Sprint(args...))
}

func (a *slogAdapter) Warn(args ...interface{}) {
	a.logger.Warn(fmt.Sprint(args...))
}

func (a *slogAdapter) Error(args ...interface{}) {
	a.logger.Error(fmt.Sprint(args...))
}

func (a *slogAdapter) Fatal(args ...interface{}) {
	a.logger.Log(context.Background(), slog.LevelError+4, fmt.Sprint(args...))
	os.Exit(1)
}
