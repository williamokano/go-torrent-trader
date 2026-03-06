package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
)

// StalePeerCutoff defines how long after the last announce a peer is
// considered stale. This is typically announce_interval * 1.5.
const StalePeerCutoff = 30 * time.Minute

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

// NewCleanupHandler returns an asynq handler that deletes stale peers and
// recalculates seeder/leecher counts for all torrents.
func NewCleanupHandler(deps *WorkerDeps) func(ctx context.Context, t *asynq.Task) error {
	return func(ctx context.Context, _ *asynq.Task) error {
		cutoff := time.Now().Add(-StalePeerCutoff)

		removed, err := deps.PeerRepo.DeleteStale(ctx, cutoff)
		if err != nil {
			return fmt.Errorf("deleting stale peers: %w", err)
		}
		slog.Info("cleaned stale peers", "removed", removed)

		if removed == 0 {
			return nil
		}

		// Recalculate seeder and leecher counts for all torrents that still
		// have peers, and zero out counts for torrents with no remaining peers.
		_, err = deps.DB.ExecContext(ctx, `
			WITH peer_counts AS (
				SELECT
					torrent_id,
					COUNT(*) FILTER (WHERE seeder = true)  AS seeder_cnt,
					COUNT(*) FILTER (WHERE seeder = false) AS leecher_cnt
				FROM peers
				GROUP BY torrent_id
			)
			UPDATE torrents t SET
				seeders    = COALESCE(pc.seeder_cnt, 0),
				leechers   = COALESCE(pc.leecher_cnt, 0),
				updated_at = NOW()
			FROM peer_counts pc
			WHERE t.id = pc.torrent_id
		`)
		if err != nil {
			return fmt.Errorf("recalculating torrent peer counts: %w", err)
		}

		// Zero out counts for torrents that no longer have any peers.
		_, err = deps.DB.ExecContext(ctx, `
			UPDATE torrents SET
				seeders    = 0,
				leechers   = 0,
				updated_at = NOW()
			WHERE id NOT IN (SELECT DISTINCT torrent_id FROM peers)
			  AND (seeders > 0 OR leechers > 0)
		`)
		if err != nil {
			return fmt.Errorf("zeroing orphaned torrent peer counts: %w", err)
		}

		return nil
	}
}

// HandleRecalcStats recalculates site-wide statistics.
func HandleRecalcStats(_ context.Context, _ *asynq.Task) error {
	slog.Info("recalculating site statistics")
	// TODO: implement stats recalculation
	return nil
}
