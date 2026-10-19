package worker

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// StalePeerCutoff defines how long after the last announce a peer is
// considered stale: two and a half announce intervals. A removed peer comes
// back with no baseline and its traffic since the last announce goes
// uncredited, so one missed announce (a sleeping laptop, a network blip)
// must not be enough to remove it — at 1.5 intervals it was.
const StalePeerCutoff = 5 * service.DefaultInterval * time.Second / 2

// NewSendEmailHandler returns a handler that sends emails using the provided EmailSender.
func NewSendEmailHandler(deps *WorkerDeps) func(ctx context.Context, job *river.Job[SendEmailArgs]) error {
	return func(ctx context.Context, job *river.Job[SendEmailArgs]) error {
		payload := job.Args
		slog.Info("sending email", "to", payload.To, "subject", payload.Subject)
		if deps.EmailSender == nil {
			slog.Warn("email sender not configured, skipping email send")
			return nil
		}
		if err := deps.EmailSender.Send(ctx, payload.To, payload.Subject, payload.Body); err != nil {
			return fmt.Errorf("send email: %w", err)
		}
		return nil
	}
}

// NewCleanupHandler returns the handler that runs all scheduled
// maintenance tasks: stale peer removal, count recalculation, dead
// torrent hiding, expired invite/registration cleanup, and warning
// deactivation.
func NewCleanupHandler(deps *WorkerDeps) func(ctx context.Context, job *river.Job[CleanupPeersArgs]) error {
	return func(ctx context.Context, _ *river.Job[CleanupPeersArgs]) error {
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

		// 4. Delete expired pending registrations (never activated, older than 7 days)
		//
		// enabled=false alone is NOT a safe signal here: banning a user also sets
		// enabled=false, so a banned user whose *registration* happens to be
		// older than 7 days would otherwise be hard-deleted, cascading away their
		// torrents/notes/warnings and erasing the ban record itself.
		// awaiting_confirmation is the pending signal — every disable clears it
		// (model.UserPatch.SetEnabled), so this also spares a member banned
		// before they ever confirmed.
		//
		// activated_at IS NULL is kept as a backstop "never activated" signal (BE-8.19):
		// model.User.ActivatedAt is stamped exactly once — at registration when
		// no email confirmation is required, at email confirmation, or at first
		// login, whichever happens first (see AuthService.Register/Login/
		// ConfirmEmail) — and is never touched by a ban. Deliberately not
		// last_access: that column only updates on a *subsequent* authenticated
		// request (ActivityTracker middleware, debounced), so a user banned
		// immediately after registering or logging in once would still read as
		// "never activated" under last_access and get caught by this same bug.
		// activated_at requires no follow-up request, so it only stays NULL for
		// an account that was created and genuinely never activated at all.
		// Users who still hold a pending (unclaimed, unexpired) email
		// confirmation are excluded; see purgeExpiredRegistrations.
		purgeExpiredRegistrations(ctx, deps.DB)

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

		// 8. Clean up expired/confirmed email confirmation tokens (older than 7 days)
		res, err = deps.DB.ExecContext(ctx, `
			DELETE FROM email_confirmations
			WHERE confirmed_at IS NOT NULL OR expires_at < NOW() - INTERVAL '7 days'
		`)
		if err != nil {
			slog.Error("cleanup: failed to delete expired email confirmations", "error", err)
		} else if n, _ := res.RowsAffected(); n > 0 {
			slog.Info("cleanup: expired email confirmations deleted", "count", n)
		}

		return nil
	}
}

// expiredRegistrationsSQL deletes registrations that were never activated and
// whose confirmation has lapsed — all of them, or only the one whose id is $1.
//
// Two things go in the same statement rather than by cascade. Messages sent
// to the account: a cascade on messages.receiver_id would also silently
// delete a real member's mail should members ever be deleted (the no-action
// key is checked at the end of the statement, after both deletes). And the
// invite the account registered with goes by cascade, but its slot is given
// back to the inviter, who should not lose it to someone who never confirmed.
const expiredRegistrationsSQL = `
	WITH expired AS (
	  SELECT id FROM users
	  WHERE ($1::bigint IS NULL OR id = $1)
	    AND awaiting_confirmation
	    AND activated_at IS NULL
	    AND created_at < NOW() - INTERVAL '7 days'
	    AND NOT EXISTS (
	      SELECT 1 FROM email_confirmations ec
	      WHERE ec.user_id = users.id
	        AND ec.confirmed_at IS NULL
	        AND ec.expires_at > NOW()
	    )
	), mail AS (
	  DELETE FROM messages WHERE receiver_id IN (SELECT id FROM expired)
	), refund AS (
	  UPDATE users u SET invites = u.invites + r.n
	  FROM (
	    SELECT inviter_id, COUNT(*) AS n FROM invites
	    WHERE used_by_id IN (SELECT id FROM expired)
	    GROUP BY inviter_id
	  ) r
	  WHERE u.id = r.inviter_id
	)
	DELETE FROM users WHERE id IN (SELECT id FROM expired)`

// purgeExpiredRegistrations removes expired registrations in one statement.
// If anything blocks it — one account holding a row nobody planned for — it
// falls back to one account at a time, so the rest still go and the account
// that would not is named in the log, instead of the whole purge failing
// silently on every run.
func purgeExpiredRegistrations(ctx context.Context, db *sql.DB) {
	res, err := db.ExecContext(ctx, expiredRegistrationsSQL, nil)
	if err == nil {
		if n, _ := res.RowsAffected(); n > 0 {
			slog.Info("cleanup: expired registrations deleted", "count", n)
		}
		return
	}
	slog.Warn("cleanup: expired registrations could not be deleted together, retrying one by one", "error", err)

	rows, err := db.QueryContext(ctx, `SELECT id FROM users
		WHERE awaiting_confirmation AND activated_at IS NULL AND created_at < NOW() - INTERVAL '7 days'`)
	if err != nil {
		slog.Error("cleanup: failed to list expired registrations", "error", err)
		return
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			slog.Error("cleanup: failed to read expired registration", "error", err)
			_ = rows.Close()
			return
		}
		ids = append(ids, id)
	}
	_ = rows.Close()

	deleted, failed := 0, 0
	for _, id := range ids {
		res, err := db.ExecContext(ctx, expiredRegistrationsSQL, id)
		if err != nil {
			failed++
			slog.Error("cleanup: expired registration could not be deleted", "user_id", id, "error", err)
			continue
		}
		if n, _ := res.RowsAffected(); n > 0 {
			deleted++
		}
	}
	slog.Info("cleanup: expired registrations deleted one by one", "count", deleted, "failed", failed)
}
