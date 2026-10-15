package worker

import (
	"context"
	"testing"
)

// The handler must no-op safely when its dependencies are absent, rather than
// panicking a background worker — mirrors TestInviteDistributionHandlerSkipsWhenDepsMissing.
func TestDigestHandlerSkipsWhenDepsMissing(t *testing.T) {
	handler := NewDigestHandler(&WorkerDeps{})
	if err := handler(context.Background(), nil); err != nil {
		t.Errorf("handler with no deps returned %v, want a clean skip", err)
	}
}
