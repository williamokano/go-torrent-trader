package worker

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"
)

// AsynqEmailEnqueuer implements service.TaskEnqueuer using asynq.
type AsynqEmailEnqueuer struct {
	client *asynq.Client
}

// NewAsynqEmailEnqueuer creates an enqueuer backed by the given asynq client.
func NewAsynqEmailEnqueuer(client *asynq.Client) *AsynqEmailEnqueuer {
	return &AsynqEmailEnqueuer{client: client}
}

// EnqueueSendEmail enqueues an email sending task for background processing.
func (e *AsynqEmailEnqueuer) EnqueueSendEmail(_ context.Context, to, subject, body string) error {
	task, err := NewSendEmailTask(to, subject, body)
	if err != nil {
		return fmt.Errorf("create email task: %w", err)
	}
	_, err = e.client.Enqueue(task)
	if err != nil {
		return fmt.Errorf("enqueue email task: %w", err)
	}
	return nil
}
