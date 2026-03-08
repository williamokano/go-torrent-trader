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
const StalePeerCutoff = 45 * time.Minute

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

// NewCleanupHandler returns an asynq handler that runs all scheduled
// maintenance tasks: stale peer removal, count recalculation, dead
// torrent hiding, expired invite/registration cleanup, and warning
// deactivation.
func NewCleanupHandler(deps *WorkerDeps) func(ctx context.Context, t *asynq.Task) error {
	return func(ctx context.Context, _ *asynq.Task) error {
		// 1. Remove stale peers
		cutoff := time.Now().Add(-StalePeerCutoff)
		removed, err := deps.PeerRepo.DeleteStale(ctx, cutoff)
		if err != nil {
			return fmt.Errorf("deleting stale peers: %w", err)
		}
		slog.Info("cleanup: stale peers removed", "count", removed)

		// 2. Recalculate seeder/leecher counts from actual peers
		if removed > 0 {
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

			// Zero out counts for torrents with no remaining peers.
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
		}

		// Remaining tasks require a DB connection (skipped in unit tests with nil DB)
		if deps.DB == nil {
			return nil
		}

		// 3. Hide dead torrents (no seeders for over 28 days)
		res, err := deps.DB.ExecContext(ctx, `
			UPDATE torrents SET visible = false, updated_at = NOW()
			WHERE visible = true
			  AND seeders = 0
			  AND updated_at < NOW() - INTERVAL '28 days'
		`)
		if err != nil {
			slog.Error("cleanup: failed to hide dead torrents", "error", err)
		} else if n, _ := res.RowsAffected(); n > 0 {
			slog.Info("cleanup: dead torrents hidden", "count", n)
		}

		// 4. Delete expired pending registrations (enabled=false, older than 7 days)
		res, err = deps.DB.ExecContext(ctx, `
			DELETE FROM users
			WHERE enabled = false
			  AND created_at < NOW() - INTERVAL '7 days'
		`)
		if err != nil {
			slog.Error("cleanup: failed to delete expired registrations", "error", err)
		} else if n, _ := res.RowsAffected(); n > 0 {
			slog.Info("cleanup: expired registrations deleted", "count", n)
		}

		// 5. Remove expired invite tokens
		res, err = deps.DB.ExecContext(ctx, `
			DELETE FROM invites
			WHERE used_by_id IS NULL
			  AND expires_at < NOW()
		`)
		if err != nil {
			slog.Error("cleanup: failed to delete expired invites", "error", err)
		} else if n, _ := res.RowsAffected(); n > 0 {
			slog.Info("cleanup: expired invites deleted", "count", n)
		}

		// 6. Deactivate expired warnings
		res, err = deps.DB.ExecContext(ctx, `
			UPDATE users SET
				warned = false,
				warn_until = NULL,
				updated_at = NOW()
			WHERE warned = true
			  AND warn_until IS NOT NULL
			  AND warn_until < NOW()
		`)
		if err != nil {
			slog.Error("cleanup: failed to deactivate expired warnings", "error", err)
		} else if n, _ := res.RowsAffected(); n > 0 {
			slog.Info("cleanup: expired warnings deactivated", "count", n)
		}

		// 7. Clean up expired/used password reset tokens (older than 7 days)
		res, err = deps.DB.ExecContext(ctx, `
			DELETE FROM password_resets
			WHERE used = true OR expires_at < NOW() - INTERVAL '7 days'
		`)
		if err != nil {
			slog.Error("cleanup: failed to delete expired password resets", "error", err)
		} else if n, _ := res.RowsAffected(); n > 0 {
			slog.Info("cleanup: expired password resets deleted", "count", n)
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
