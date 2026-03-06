package worker

import (
	"encoding/json"
	"testing"
)

func TestNewSendEmailTask(t *testing.T) {
	task, err := NewSendEmailTask("user@example.com", "Welcome", "Hello!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.Type() != TaskSendEmail {
		t.Errorf("expected task type %s, got %s", TaskSendEmail, task.Type())
	}
	if task.Payload() == nil {
		t.Fatal("expected non-nil payload")
	}
}

func TestNewSendEmailTaskPayloadRoundtrip(t *testing.T) {
	to := "user@example.com"
	subject := "Test Subject"
	body := "Test Body"

	task, err := NewSendEmailTask(to, subject, body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload EmailPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}

	if payload.To != to {
		t.Errorf("expected To %q, got %q", to, payload.To)
	}
	if payload.Subject != subject {
		t.Errorf("expected Subject %q, got %q", subject, payload.Subject)
	}
	if payload.Body != body {
		t.Errorf("expected Body %q, got %q", body, payload.Body)
	}
}

func TestNewCleanupPeersTask(t *testing.T) {
	task, err := NewCleanupPeersTask()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.Type() != TaskCleanupPeers {
		t.Errorf("expected task type %s, got %s", TaskCleanupPeers, task.Type())
	}
}

func TestNewRecalcStatsTask(t *testing.T) {
	task, err := NewRecalcStatsTask()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.Type() != TaskRecalcStats {
		t.Errorf("expected task type %s, got %s", TaskRecalcStats, task.Type())
	}
}
