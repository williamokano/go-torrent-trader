package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/williamokano/go-torrent-trader/backend/internal/model"
	"github.com/williamokano/go-torrent-trader/backend/internal/repository"
)

// dateLayout is the wire form used for the rollup watermark.
//
// The watermark is read and written as text rather than as a time.Time bound to a
// DATE column on purpose: a date has no timezone, so every conversion between one
// and a Go time.Time picks a zone from somewhere — the driver, or the session's
// TimeZone. This watermark is what stops the prune deleting un-aggregated rows, so
// "probably UTC" is not good enough. Text in, text out, parsed as UTC here.
const dateLayout = "2006-01-02"

// AnnounceRollupRepo implements repository.AnnounceRollupRepository using PostgreSQL.
type AnnounceRollupRepo struct {
	db *sql.DB
}

// NewAnnounceRollupRepo returns a new PostgreSQL-backed AnnounceRollupRepository.
func NewAnnounceRollupRepo(db *sql.DB) repository.AnnounceRollupRepository {
	return &AnnounceRollupRepo{db: db}
}

func (r *AnnounceRollupRepo) RolledThrough(ctx context.Context) (time.Time, error) {
	var text string
	if err := r.db.QueryRowContext(ctx,
		`SELECT rolled_through::text FROM announce_rollup_state WHERE id`).Scan(&text); err != nil {
		// Wrapped, not replaced: callers distinguish "no watermark" (fail closed,
		// prune nothing) from a connection failure, and sql.ErrNoRows is how the
		// rest of this package says the row is absent.
		return time.Time{}, fmt.Errorf("reading announce rollup watermark: %w", err)
	}
	return parseUTCDate(text)
}

func (r *AnnounceRollupRepo) Rollup(ctx context.Context, through time.Time, maxDays int) (repository.RollupResult, error) {
	if maxDays < 1 {
		maxDays = 1
	}
	through = utcMidnight(through)

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return repository.RollupResult{}, fmt.Errorf("begin announce rollup: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// FOR UPDATE serialises two workers racing on the same watermark. Without it
	// both would read the same start date and both would add the same deltas.
	var fromText string
	if err := tx.QueryRowContext(ctx,
		`SELECT rolled_through::text FROM announce_rollup_state WHERE id FOR UPDATE`).Scan(&fromText); err != nil {
		return repository.RollupResult{}, fmt.Errorf("locking announce rollup watermark: %w", err)
	}
	from, err := parseUTCDate(fromText)
	if err != nil {
		return repository.RollupResult{}, err
	}

	if !from.Before(through) {
		// Already caught up. Nothing to aggregate and nothing to advance — and a
		// watermark ahead of `through` (a clock stepped backwards) must not be
		// walked back, or every day between would be counted twice.
		return repository.RollupResult{From: from, To: from, CaughtUp: true}, nil
	}

	to := from.AddDate(0, 0, maxDays)
	if to.After(through) {
		to = through
	}

	// Bounds are passed as time.Time against timestamptz, which is exact. Months
	// are bucketed with an explicit AT TIME ZONE 'UTC' so the boundary does not
	// move with the session's TimeZone setting.
	//
	// A member deleted between this statement's snapshot and its FK check would
	// fail the whole transaction on user_period_stats' foreign key. That leaves the
	// watermark unadvanced and is retried on the next run, which is the behaviour
	// we want: no partial credit, no lost days.
	res, err := tx.ExecContext(ctx, rollupUpsert, from, to)
	if err != nil {
		return repository.RollupResult{}, fmt.Errorf("aggregating announce events: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return repository.RollupResult{}, fmt.Errorf("checking rows affected: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE announce_rollup_state SET rolled_through = $1::date, updated_at = NOW() WHERE id`,
		to.Format(dateLayout)); err != nil {
		return repository.RollupResult{}, fmt.Errorf("advancing announce rollup watermark: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return repository.RollupResult{}, fmt.Errorf("commit announce rollup: %w", err)
	}

	return repository.RollupResult{
		From:     from,
		To:       to,
		Rows:     rows,
		CaughtUp: !to.Before(through),
	}, nil
}

// rollupUpsert adds one window's deltas to the monthly totals. The conflict clause
// adds rather than replaces, because a month is aggregated across as many runs as
// it has days and each window contributes its own slice.
const rollupUpsert = `
INSERT INTO user_period_stats (
	user_id, year_month, uploaded, downloaded, counted_downloaded,
	announces, seed_announces, updated_at
)
SELECT ae.user_id,
       to_char(ae.announced_at AT TIME ZONE 'UTC', 'YYYY-MM'),
       COALESCE(SUM(ae.uploaded_delta), 0),
       COALESCE(SUM(ae.downloaded_delta), 0),
       COALESCE(SUM(ae.counted_downloaded_delta), 0),
       COUNT(*),
       COUNT(*) FILTER (WHERE ae.seeder),
       NOW()
FROM announce_events ae
WHERE ae.announced_at >= $1 AND ae.announced_at < $2
GROUP BY 1, 2
ON CONFLICT (user_id, year_month) DO UPDATE SET
	uploaded           = user_period_stats.uploaded + EXCLUDED.uploaded,
	downloaded         = user_period_stats.downloaded + EXCLUDED.downloaded,
	counted_downloaded = user_period_stats.counted_downloaded + EXCLUDED.counted_downloaded,
	announces          = user_period_stats.announces + EXCLUDED.announces,
	seed_announces     = user_period_stats.seed_announces + EXCLUDED.seed_announces,
	updated_at         = NOW()`

func (r *AnnounceRollupRepo) ListByUser(ctx context.Context, userID int64, limit int) ([]model.UserPeriodStats, error) {
	if limit < 1 {
		limit = 12
	}
	if limit > 240 {
		limit = 240
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT user_id, year_month, uploaded, downloaded, counted_downloaded,
		        announces, seed_announces, updated_at
		 FROM user_period_stats
		 WHERE user_id = $1
		 ORDER BY year_month DESC
		 LIMIT $2`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing user period stats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []model.UserPeriodStats
	for rows.Next() {
		var s model.UserPeriodStats
		if err := rows.Scan(&s.UserID, &s.YearMonth, &s.Uploaded, &s.Downloaded,
			&s.CountedDownloaded, &s.Announces, &s.SeedAnnounces, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning user period stats: %w", err)
		}
		results = append(results, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating user period stats: %w", err)
	}
	return results, nil
}

// parseUTCDate reads a Postgres date literal as midnight UTC.
func parseUTCDate(text string) (time.Time, error) {
	d, err := time.ParseInLocation(dateLayout, text, time.UTC)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing date %q: %w", text, err)
	}
	return d, nil
}

// utcMidnight drops the clock time, in UTC. Date arithmetic elsewhere in this file
// assumes its inputs are midnights; this is what makes that true.
func utcMidnight(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}
