package worker

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hibiken/asynq"
)

func TestHandleSendEmailValid(t *testing.T) {
	payload, err := json.Marshal(EmailPayload{
		To:      "user@example.com",
		Subject: "Test",
		Body:    "Hello",
	})
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	task := asynq.NewTask(TaskSendEmail, payload)
	if err := HandleSendEmail(context.Background(), task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleSendEmailInvalidPayload(t *testing.T) {
	task := asynq.NewTask(TaskSendEmail, []byte("invalid json"))
	err := HandleSendEmail(context.Background(), task)
	if err == nil {
		t.Fatal("expected error for invalid payload")
	}
}

func TestHandleCleanupPeers(t *testing.T) {
	task := asynq.NewTask(TaskCleanupPeers, nil)
	if err := HandleCleanupPeers(context.Background(), task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleRecalcStats(t *testing.T) {
	task := asynq.NewTask(TaskRecalcStats, nil)
	if err := HandleRecalcStats(context.Background(), task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
