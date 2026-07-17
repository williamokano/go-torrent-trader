package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository/postgres"
	"github.com/williamokano/go-torrent-trader/backend/internal/service"
)

// mentionableTables lists every table with a mentioned_usernames JSONB
// column. Hardcoded here (never derived from user input), so interpolating
// the name directly into SQL below is safe.
var mentionableTables = []string{"torrent_comments", "forum_posts", "messages"}

// Options configures a backfill run.
type Options struct {
	DryRun    bool
	BatchSize int
}

// TableStats summarizes what a backfill run did to one table.
type TableStats struct {
	Scanned int
	Changed int
}

// Stats summarizes an entire backfill run.
type Stats struct {
	Tables map[string]TableStats
}

// checkSchema verifies every mentionable table already has the
// mentioned_usernames column, so a missing migration fails loudly up front
// instead of as a confusing mid-run SQL error.
func checkSchema(ctx context.Context, db *sql.DB) error {
	for _, table := range mentionableTables {
		//nolint:gosec // table is one of the hardcoded constants above, never user input
		rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT mentioned_usernames FROM %s LIMIT 0", table))
		if err != nil {
			return fmt.Errorf("mentioned_usernames column missing on %s (has the migration been applied?): %w", table, err)
		}
		_ = rows.Close()
	}
	return nil
}

// runBackfill recomputes mentioned_usernames for every mentionable row across
// torrent_comments, forum_posts, and messages, reusing the exact same
// resolution logic (service.ResolveMentionedUsernames) that CreateComment /
// CreateTopic / CreatePost / EditPost / SendMessage already use — so this
// tool can never drift from what a fresh write would produce.
func runBackfill(ctx context.Context, db *sql.DB, opts Options) (Stats, error) {
	users := postgres.NewUserRepo(db)
	stats := Stats{Tables: make(map[string]TableStats, len(mentionableTables))}

	for _, table := range mentionableTables {
		ts, err := backfillTable(ctx, db, users, table, opts)
		if err != nil {
			return stats, fmt.Errorf("backfill %s: %w", table, err)
		}
		stats.Tables[table] = ts
		slog.Info("backfill complete for table", "table", table, "scanned", ts.Scanned, "changed", ts.Changed)
	}

	return stats, nil
}

type mentionRow struct {
	id        int64
	body      string
	mentioned []string
}

// backfillTable keyset-paginates a single table by id, prefiltering on
// `body LIKE '%@%'` (a pure optimization — Extract would return nothing for
// a row without '@' anyway, so this never changes behavior, only skips
// wasted resolution round trips). Every matching row is reprocessed
// unconditionally: ResolveMentionedUsernames is a pure function of body, so
// re-running is always safe, and "mentioned_usernames already looks
// populated" is not a reliable skip signal (an empty array is indistinguishable
// from "never processed").
//
// Writes go through a narrow `UPDATE <table> SET mentioned_usernames = $1
// WHERE id = $2` only — never CommentRepo.Update / ForumPostRepo.Update,
// which also stamp updated_at / edited_at+edited_by and would make every
// backfilled row incorrectly show up as "(edited)" in the UI.
func backfillTable(ctx context.Context, db *sql.DB, users repository.UserRepository, table string, opts Options) (TableStats, error) {
	var stats TableStats
	var lastID int64

	//nolint:gosec // table is one of the hardcoded constants above, never user input
	selectQuery := fmt.Sprintf(
		`SELECT id, body, mentioned_usernames FROM %s WHERE id > $1 AND body LIKE '%%@%%' ORDER BY id ASC LIMIT $2`,
		table,
	)
	//nolint:gosec // table is one of the hardcoded constants above, never user input
	updateQuery := fmt.Sprintf(`UPDATE %s SET mentioned_usernames = $1 WHERE id = $2`, table)

	for {
		batch, err := fetchBatch(ctx, db, selectQuery, lastID, opts.BatchSize)
		if err != nil {
			return stats, fmt.Errorf("fetch batch from %s: %w", table, err)
		}
		if len(batch) == 0 {
			break
		}
		lastID = batch[len(batch)-1].id
		stats.Scanned += len(batch)

		type resolvedUpdate struct {
			id        int64
			mentioned []string
		}
		var updates []resolvedUpdate

		for i := range batch {
			resolved, err := service.ResolveMentionedUsernames(ctx, users, batch[i].body)
			if err != nil {
				return stats, fmt.Errorf("resolve mentions for %s id=%d: %w", table, batch[i].id, err)
			}

			changed := !stringSlicesEqual(batch[i].mentioned, resolved)
			if opts.DryRun {
				if changed {
					slog.Info("would update mentioned_usernames", "table", table, "id", batch[i].id,
						"old", batch[i].mentioned, "new", resolved)
					stats.Changed++
				}
				continue
			}
			if changed {
				updates = append(updates, resolvedUpdate{id: batch[i].id, mentioned: resolved})
			}
		}

		if !opts.DryRun && len(updates) > 0 {
			err := repository.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
				for _, u := range updates {
					encoded, err := postgres.MarshalMentionedUsernames(u.mentioned)
					if err != nil {
						return fmt.Errorf("marshal mentioned usernames for %s id=%d: %w", table, u.id, err)
					}
					if _, err := tx.ExecContext(ctx, updateQuery, encoded, u.id); err != nil {
						return fmt.Errorf("update %s id=%d: %w", table, u.id, err)
					}
				}
				return nil
			})
			if err != nil {
				return stats, err
			}
			stats.Changed += len(updates)
		}

		slog.Info("backfill progress", "table", table, "scanned", stats.Scanned, "changed", stats.Changed, "last_id", lastID)

		if len(batch) < opts.BatchSize {
			break
		}
	}

	return stats, nil
}

func fetchBatch(ctx context.Context, db *sql.DB, query string, afterID int64, limit int) ([]mentionRow, error) {
	rows, err := db.QueryContext(ctx, query, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var batch []mentionRow
	for rows.Next() {
		var r mentionRow
		var mentioned []byte
		if err := rows.Scan(&r.id, &r.body, &mentioned); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		r.mentioned = postgres.ScanMentionedUsernames(mentioned)
		batch = append(batch, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return batch, nil
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
