package worker

import (
	"context"
	"testing"
)

// TestNewDigestTask verifies the task descriptor is constructible and carries
// the expected type, mirroring TestNewInviteDistributionTask/other task
// constructors in this package.
func TestNewDigestTask(t *testing.T) {
	task, err := NewDigestTask()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.Type() != TaskDigest {
		t.Errorf("expected task type %s, got %s", TaskDigest, task.Type())
	}
}

// The handler must no-op safely when its dependencies are absent, rather than
// panicking a background worker — mirrors TestInviteDistributionHandlerSkipsWhenDepsMissing.
func TestDigestHandlerSkipsWhenDepsMissing(t *testing.T) {
	handler := NewDigestHandler(&WorkerDeps{})
	if err := handler(context.Background(), nil); err != nil {
		t.Errorf("handler with no deps returned %v, want a clean skip", err)
	}
}
