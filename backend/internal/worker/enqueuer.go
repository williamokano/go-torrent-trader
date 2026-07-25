package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

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

// AsynqConnectorEnqueuer implements listener.DrainEnqueuer using asynq.
type AsynqConnectorEnqueuer struct {
	client *asynq.Client
}

// NewAsynqConnectorEnqueuer creates an enqueuer backed by the given asynq client.
func NewAsynqConnectorEnqueuer(client *asynq.Client) *AsynqConnectorEnqueuer {
	return &AsynqConnectorEnqueuer{client: client}
}

// EnqueueConnectorDrain queues a drain for one instance, optionally deferred.
//
// A duplicate inside the task's uniqueness window is reported as success: that
// is the burst-collapse doing its job (25 approvals in a second produce one
// drain, which then delivers all 25 rows), not a failure to schedule work.
func (e *AsynqConnectorEnqueuer) EnqueueConnectorDrain(_ context.Context, instanceID int64, delay time.Duration) error {
	task, err := NewConnectorDrainTask(instanceID, delay)
	if err != nil {
		return fmt.Errorf("create connector drain task: %w", err)
	}
	if _, err := e.client.Enqueue(task); err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) {
			return nil
		}
		return fmt.Errorf("enqueue connector drain task: %w", err)
	}
	return nil
}
