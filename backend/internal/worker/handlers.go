package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
)

// HandleSendEmail processes email sending tasks.
func HandleSendEmail(_ context.Context, t *asynq.Task) error {
	var payload EmailPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal email payload: %w", err)
	}
	slog.Info("sending email", "to", payload.To, "subject", payload.Subject)
	// TODO: implement actual SMTP sending
	return nil
}

// HandleCleanupPeers removes stale peers from the database.
func HandleCleanupPeers(_ context.Context, _ *asynq.Task) error {
	slog.Info("cleaning up stale peers")
	// TODO: call PeerRepository.DeleteStale()
	return nil
}

// HandleRecalcStats recalculates site-wide statistics.
func HandleRecalcStats(_ context.Context, _ *asynq.Task) error {
	slog.Info("recalculating site statistics")
	// TODO: implement stats recalculation
	return nil
}
