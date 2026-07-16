package worker

import (
	"context"
	"testing"
)

// TestNewInviteDistributionTask verifies the task descriptor is constructible
// and carries the expected type, mirroring TestNewRatioWarningTask/other task
// constructors in this package.
func TestNewInviteDistributionTask(t *testing.T) {
	task, err := NewInviteDistributionTask()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.Type() != TaskInviteDistribution {
		t.Errorf("expected task type %s, got %s", TaskInviteDistribution, task.Type())
	}
}

// The handler must no-op safely when its dependencies are absent, rather than
// panicking a background worker — mirrors TestRatioWarningHandlerSkipsWhenDepsMissing.
func TestInviteDistributionHandlerSkipsWhenDepsMissing(t *testing.T) {
	handler := NewInviteDistributionHandler(&WorkerDeps{})
	if err := handler(context.Background(), nil); err != nil {
		t.Errorf("handler with no deps returned %v, want a clean skip", err)
	}
}
