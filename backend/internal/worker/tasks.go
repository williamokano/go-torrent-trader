package worker

import (
	"encoding/json"
	"time"

	"github.com/hibiken/asynq"
)

// EmailPayload holds the data for an email sending task.
type EmailPayload struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// NewSendEmailTask creates a task to send an email.
func NewSendEmailTask(to, subject, body string) (*asynq.Task, error) {
	payload, err := json.Marshal(EmailPayload{To: to, Subject: subject, Body: body})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskSendEmail, payload, asynq.MaxRetry(3)), nil
}

// NewCleanupPeersTask creates a task to clean stale peers.
// Unique window prevents duplicate enqueues when multiple scheduler instances are running.
func NewCleanupPeersTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskCleanupPeers, nil, asynq.MaxRetry(1), asynq.Unique(14*time.Minute)), nil
}

// NewRecalcStatsTask creates a task to recalculate site statistics.
// Unique window prevents duplicate enqueues when multiple scheduler instances are running.
func NewRecalcStatsTask() (*asynq.Task, error) {
	return asynq.NewTask(TaskRecalcStats, nil, asynq.MaxRetry(1), asynq.Unique(59*time.Minute)), nil
}
